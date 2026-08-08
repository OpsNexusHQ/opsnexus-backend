package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Response struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

func Handler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		databaseStatus := "ok"
		status := "ok"

		if err := db.Ping(ctx); err != nil {
			databaseStatus = "error"
			status = "degraded"
		}

		w.Header().Set("Content-Type", "application/json")

		response := Response{
			Status:   status,
			Database: databaseStatus,
		}

		if status != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		_ = json.NewEncoder(w).Encode(response)
	}
}
