package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	db            *pgxpool.Pool
	retentionDays int
	interval      time.Duration
	logger        *slog.Logger
}

func NewWorker(db *pgxpool.Pool, retentionDays int, interval time.Duration, logger *slog.Logger) *Worker {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		db:            db,
		retentionDays: retentionDays,
		interval:      interval,
		logger:        logger,
	}
}

func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial run on startup
	w.runRetentionTask(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("telemetry retention worker stopped")
			return
		case <-ticker.C:
			w.runRetentionTask(ctx)
		}
	}
}

func (w *Worker) runRetentionTask(ctx context.Context) {
	w.logger.Info("running telemetry retention and archival task", slog.Int("retention_days", w.retentionDays))

	// 1. Perform hourly rollup aggregation for records older than 1 hour
	if err := w.rollupHourly(ctx); err != nil {
		w.logger.Error("failed to rollup hourly telemetry", slog.Any("error", err))
	}

	// 2. Delete raw telemetry older than configured retention days
	if err := w.purgeOldTelemetry(ctx); err != nil {
		w.logger.Error("failed to purge old telemetry", slog.Any("error", err))
	}
}

func (w *Worker) rollupHourly(ctx context.Context) error {
	_, err := w.db.Exec(ctx, hourlyRollupQuery)
	if err != nil {
		return fmt.Errorf("execute hourly rollup: %w", err)
	}
	return nil
}

const hourlyRollupQuery = `
		INSERT INTO telemetry_hourly (
			agent_id, bucket_start, cpu_avg, cpu_min, cpu_max,
			memory_avg, memory_min, memory_max, disk_avg,
			network_received, network_sent, sample_count
		)
		WITH samples AS (
			SELECT
				t.agent_id,
				date_trunc('hour', t.timestamp) AS bucket_start,
				(t.metrics->'system'->'cpu'->>'usage_percent')::double precision AS cpu_usage,
				(t.metrics->'system'->'memory'->>'used_percent')::double precision AS memory_usage,
				root_disk.used_percent AS disk_usage,
				COALESCE(network.bytes_recv, 0) AS bytes_recv,
				COALESCE(network.bytes_sent, 0) AS bytes_sent
			FROM telemetry t
			LEFT JOIN LATERAL (
				SELECT (partition->>'used_percent')::double precision AS used_percent
				FROM jsonb_array_elements(
					CASE
						WHEN jsonb_typeof(t.metrics->'system'->'disk'->'partitions') = 'array'
						THEN t.metrics->'system'->'disk'->'partitions'
						ELSE '[]'::jsonb
					END
				) AS partition
				WHERE partition->>'mountpoint' = '/'
				LIMIT 1
			) AS root_disk ON TRUE
				LEFT JOIN LATERAL (
					SELECT
						COALESCE(SUM((iface->>'bytes_recv')::bigint), 0) AS bytes_recv,
						COALESCE(SUM((iface->>'bytes_sent')::bigint), 0) AS bytes_sent
					FROM jsonb_array_elements(
						CASE
							WHEN jsonb_typeof(t.metrics->'system'->'network'->'interfaces') = 'array'
						THEN t.metrics->'system'->'network'->'interfaces'
							ELSE '[]'::jsonb
						END
					) AS iface
				) AS network ON TRUE
			WHERE t.timestamp < date_trunc('hour', NOW())
		)
		SELECT
			agent_id,
			bucket_start,
			COALESCE(AVG(cpu_usage), 0) AS cpu_avg,
			COALESCE(MIN(cpu_usage), 0) AS cpu_min,
			COALESCE(MAX(cpu_usage), 0) AS cpu_max,
			COALESCE(AVG(memory_usage), 0) AS memory_avg,
			COALESCE(MIN(memory_usage), 0) AS memory_min,
			COALESCE(MAX(memory_usage), 0) AS memory_max,
			COALESCE(AVG(disk_usage), 0) AS disk_avg,
			COALESCE(SUM(bytes_recv), 0)::bigint AS network_received,
			COALESCE(SUM(bytes_sent), 0)::bigint AS network_sent,
			COUNT(*) AS sample_count
		FROM samples
		GROUP BY agent_id, bucket_start
		ON CONFLICT (agent_id, bucket_start) DO UPDATE SET
			cpu_avg = EXCLUDED.cpu_avg,
			cpu_min = EXCLUDED.cpu_min,
			cpu_max = EXCLUDED.cpu_max,
			memory_avg = EXCLUDED.memory_avg,
			memory_min = EXCLUDED.memory_min,
			memory_max = EXCLUDED.memory_max,
			disk_avg = EXCLUDED.disk_avg,
			network_received = EXCLUDED.network_received,
			network_sent = EXCLUDED.network_sent,
			sample_count = EXCLUDED.sample_count;
	`

func (w *Worker) purgeOldTelemetry(ctx context.Context) error {
	cutoff := time.Now().AddDate(0, 0, -w.retentionDays)
	const query = `DELETE FROM telemetry WHERE timestamp < $1`

	tag, err := w.db.Exec(ctx, query, cutoff)
	if err != nil {
		return fmt.Errorf("purge query: %w", err)
	}

	w.logger.Info("purged old raw telemetry", slog.Int64("rows_deleted", tag.RowsAffected()), slog.Time("cutoff", cutoff))
	return nil
}
