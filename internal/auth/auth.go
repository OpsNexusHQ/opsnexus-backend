package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"-"`
	RawToken   string     `json:"raw_token,omitempty"` // Only returned once on creation
	Role       Role       `json:"role"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type contextKey string

const (
	UserRoleContextKey contextKey = "user_role"
	TokenContextKey    contextKey = "api_token"
)

type Authenticator struct {
	db          *pgxpool.Pool
	authEnabled bool
}

func NewAuthenticator(db *pgxpool.Pool, authEnabled bool) *Authenticator {
	return &Authenticator{
		db:          db,
		authEnabled: authEnabled,
	}
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.authEnabled {
			ctx := context.WithValue(r.Context(), UserRoleContextKey, RoleAdmin)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Allow public endpoints
		if r.URL.Path == "/health" || r.URL.Path == "/api/v1/events" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeUnauthorized(w, "unauthorized")
			return
		}

		rawToken := strings.TrimPrefix(authHeader, "Bearer ")
		hash := sha256.Sum256([]byte(rawToken))
		tokenHash := hex.EncodeToString(hash[:])

		const query = `
			SELECT id, name, role, enabled, created_at, last_used_at, expires_at
			FROM api_tokens
			WHERE token_hash = $1 AND enabled = TRUE
			AND (expires_at IS NULL OR expires_at > NOW())
		`
		var tok APIToken
		err := a.db.QueryRow(r.Context(), query, tokenHash).Scan(
			&tok.ID, &tok.Name, &tok.Role, &tok.Enabled, &tok.CreatedAt, &tok.LastUsedAt, &tok.ExpiresAt,
		)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeUnauthorized(w, "invalid token")
				return
			}
			http.Error(w, `{"error":{"message":"authentication error"}}`, http.StatusInternalServerError)
			return
		}

		// Async update last_used_at
		go func(tokenID string) {
			_, _ = a.db.Exec(context.Background(), "UPDATE api_tokens SET last_used_at = NOW() WHERE id = $1", tokenID)
		}(tok.ID)

		ctx := context.WithValue(r.Context(), UserRoleContextKey, tok.Role)
		ctx = context.WithValue(ctx, TokenContextKey, tok)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequireRole(required Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := r.Context().Value(UserRoleContextKey).(Role)
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":{"message":"forbidden"}}`, http.StatusForbidden)
				return
			}

			if !hasPermission(role, required) {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":{"message":"insufficient permissions"}}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func hasPermission(userRole, required Role) bool {
	if userRole == RoleAdmin {
		return true
	}
	if userRole == RoleOperator && (required == RoleOperator || required == RoleViewer) {
		return true
	}
	if userRole == RoleViewer && required == RoleViewer {
		return true
	}
	return false
}

func (r Role) IsValid() bool {
	switch r {
	case RoleViewer, RoleOperator, RoleAdmin:
		return true
	default:
		return false
	}
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="OpsNexus"`)
	w.Header().Set("Content-Type", "application/json")
	http.Error(w, fmt.Sprintf(`{"error":{"message":"%s"}}`, message), http.StatusUnauthorized)
}

type TokenHandler struct {
	db *pgxpool.Pool
}

func NewTokenHandler(db *pgxpool.Pool) *TokenHandler {
	return &TokenHandler{db: db}
}

func (h *TokenHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":{"message":"method not allowed"}}`, http.StatusMethodNotAllowed)
		return
	}

	const query = `SELECT id, name, role, enabled, created_at, last_used_at, expires_at FROM api_tokens ORDER BY created_at DESC`
	rows, err := h.db.Query(r.Context(), query)
	if err != nil {
		http.Error(w, `{"error":{"message":"failed to list tokens"}}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var tok APIToken
		if err := rows.Scan(&tok.ID, &tok.Name, &tok.Role, &tok.Enabled, &tok.CreatedAt, &tok.LastUsedAt, &tok.ExpiresAt); err != nil {
			http.Error(w, `{"error":{"message":"failed to scan token"}}`, http.StatusInternalServerError)
			return
		}
		tokens = append(tokens, tok)
	}
	if tokens == nil {
		tokens = []APIToken{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"tokens": tokens})
}

type createTokenReq struct {
	Name string `json:"name"`
	Role Role   `json:"role"`
}

func (h *TokenHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":{"message":"method not allowed"}}`, http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()
	var req createTokenReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		http.Error(w, `{"error":{"message":"token name is required"}}`, http.StatusBadRequest)
		return
	}

	if req.Role == "" {
		req.Role = RoleOperator
	}
	if !req.Role.IsValid() {
		http.Error(w, `{"error":{"message":"invalid token role"}}`, http.StatusBadRequest)
		return
	}

	rawBytes := make([]byte, 24)
	_, _ = rand.Read(rawBytes)
	rawToken := fmt.Sprintf("opsnexus_%s", hex.EncodeToString(rawBytes))

	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	tokenID := fmt.Sprintf("tok-%d", time.Now().UnixNano())

	const query = `
		INSERT INTO api_tokens (id, name, token_hash, role, enabled, created_at)
		VALUES ($1, $2, $3, $4, true, NOW())
		RETURNING id, name, role, enabled, created_at
	`
	var created APIToken
	err := h.db.QueryRow(r.Context(), query, tokenID, req.Name, tokenHash, req.Role).
		Scan(&created.ID, &created.Name, &created.Role, &created.Enabled, &created.CreatedAt)

	if err != nil {
		http.Error(w, `{"error":{"message":"failed to create token"}}`, http.StatusInternalServerError)
		return
	}

	created.RawToken = rawToken // Show only once

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func (h *TokenHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":{"message":"method not allowed"}}`, http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":{"message":"token ID is required"}}`, http.StatusBadRequest)
		return
	}

	_, err := h.db.Exec(r.Context(), "DELETE FROM api_tokens WHERE id = $1", id)
	if err != nil {
		http.Error(w, `{"error":{"message":"failed to delete token"}}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
