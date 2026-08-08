package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OpsNexusHQ/opsnexus-backend/internal/config"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/database"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/server"
)

func main() {
	cfg := config.Load()

	if cfg.DatabaseURL == "" {
		log.Fatal("OPSNEXUS_DATABASE_URL is not set")
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	log.Println("database connection established")

	srv := server.New(cfg, db)

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf("OpsNexus backend listening on %s", srv.Addr)

		if err := srv.ListenAndServe(); err != nil {
			serverErrors <- err
		}
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, os.ErrClosed) {
			log.Fatalf("server error: %v", err)
		}

	case sig := <-shutdownSignal:
		log.Printf("received signal: %s", sig)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		return
	}

	log.Println("OpsNexus backend stopped")
}
