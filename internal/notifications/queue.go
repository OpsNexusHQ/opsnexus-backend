package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	DeliveryID   string
	Channel      Channel
	Notification Notification
	Attempt      int
}

type Queue struct {
	repo        *Repository
	webhookProv Provider
	slackProv   Provider
	jobs        chan Job
	maxRetries  int
	workerCount int
	logger      *slog.Logger
	wg          sync.WaitGroup
}

func NewQueue(repo *Repository, workerCount, queueSize, maxRetries int, timeout time.Duration, logger *slog.Logger) *Queue {
	if workerCount <= 0 {
		workerCount = 4
	}
	if queueSize <= 0 {
		queueSize = 256
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Queue{
		repo:        repo,
		webhookProv: NewWebhookProvider(timeout),
		slackProv:   NewSlackWebhookProvider(timeout),
		jobs:        make(chan Job, queueSize),
		maxRetries:  maxRetries,
		workerCount: workerCount,
		logger:      logger,
	}
}

func (q *Queue) Start(ctx context.Context) {
	for i := 0; i < q.workerCount; i++ {
		q.wg.Add(1)
		go q.worker(ctx, i)
	}
	q.logger.Info("notification worker queue started", slog.Int("workers", q.workerCount))
}

func (q *Queue) Stop() {
	close(q.jobs)
	q.wg.Wait()
	q.logger.Info("notification worker queue stopped")
}

func (q *Queue) Enqueue(ctx context.Context, event NotificationEvent, alertID string, alertData any) {
	channels, err := q.repo.ListChannels(ctx)
	if err != nil {
		q.logger.Error("failed to list notification channels", slog.Any("error", err))
		return
	}

	notification := Notification{
		ID:        fmt.Sprintf("notif-%d", time.Now().UnixNano()),
		Event:     event,
		Timestamp: time.Now(),
		Alert:     alertData,
	}

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}

		delivery := Delivery{
			ID:           fmt.Sprintf("del-%d", time.Now().UnixNano()),
			ChannelID:    ch.ID,
			AlertID:      alertID,
			EventType:    string(event),
			Status:       "pending",
			Attempts:     0,
			ErrorMessage: "",
		}

		createdDelivery, err := q.repo.CreateDelivery(ctx, delivery)
		if err != nil {
			q.logger.Error("failed to create delivery log", slog.Any("error", err))
			continue
		}

		job := Job{
			DeliveryID:   createdDelivery.ID,
			Channel:      ch,
			Notification: notification,
			Attempt:      1,
		}

		select {
		case q.jobs <- job:
		default:
			q.logger.Warn("notification queue full, dropping job", slog.String("channel_id", ch.ID))
			_ = q.repo.UpdateDelivery(ctx, createdDelivery.ID, "failed", 0, 0, "queue full")
		}
	}
}

func (q *Queue) worker(ctx context.Context, id int) {
	defer q.wg.Done()

	for job := range q.jobs {
		q.processJob(ctx, job)
	}
}

func (q *Queue) processJob(ctx context.Context, job Job) {
	var provider Provider
	if job.Channel.Type == "slack_webhook" {
		provider = q.slackProv
	} else {
		provider = q.webhookProv
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	respStatus, err := provider.Send(reqCtx, job.Channel.URL, job.Channel.Secret, job.Notification)
	cancel()

	dbCtx := context.Background()

	if err == nil {
		q.logger.Info("notification delivered successfully",
			slog.String("channel", job.Channel.Name),
			slog.String("delivery_id", job.DeliveryID),
			slog.Int("attempts", job.Attempt),
		)
		_ = q.repo.UpdateDelivery(dbCtx, job.DeliveryID, "delivered", job.Attempt, respStatus, "")
		return
	}

	q.logger.Warn("notification delivery failed",
		slog.String("channel", job.Channel.Name),
		slog.Int("attempt", job.Attempt),
		slog.Any("error", err),
	)

	if job.Attempt < q.maxRetries {
		// Retry with exponential backoff
		backoff := time.Duration(job.Attempt*job.Attempt) * time.Second
		time.Sleep(backoff)

		job.Attempt++
		select {
		case q.jobs <- job:
		default:
			_ = q.repo.UpdateDelivery(dbCtx, job.DeliveryID, "failed", job.Attempt, respStatus, err.Error())
		}
	} else {
		_ = q.repo.UpdateDelivery(dbCtx, job.DeliveryID, "failed", job.Attempt, respStatus, err.Error())
	}
}
