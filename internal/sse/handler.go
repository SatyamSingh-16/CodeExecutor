package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
	"github.com/SatyamSingh-16/code_executor/internal/submission"
)

// Handler manages SSE streaming connections for submission events.
type Handler struct {
	repo              SubmissionRepository
	subscriber        EventSubscriber
	heartbeatInterval time.Duration
}

// NewHandler constructs a new SSE handler with the specified repository and subscriber.
func NewHandler(repo SubmissionRepository, subscriber EventSubscriber, opts ...Option) *Handler {
	h := &Handler{
		repo:              repo,
		subscriber:        subscriber,
		heartbeatInterval: DefaultHeartbeatInterval,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Option configures the Handler.
type Option func(*Handler)

// WithHeartbeatInterval overrides the default keepalive heartbeat interval.
func WithHeartbeatInterval(d time.Duration) Option {
	return func(h *Handler) {
		if d > 0 {
			h.heartbeatInterval = d
		}
	}
}

// Stream handles GET /api/submissions/:id/stream.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// 1. Verify http.Flusher support.
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "streaming unsupported: ResponseWriter does not implement http.Flusher",
		})
		return
	}

	// 2. Authenticate user from context.
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "unauthorized",
		})
		return
	}

	// 3. Extract submission ID.
	submissionID := extractSubmissionID(r)
	if submissionID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "missing submission id",
		})
		return
	}

	// 4. Safe Race Sequence:
	// Step A: Subscribe to Redis Pub/Sub FIRST.
	// This ensures that any event published while querying PostgreSQL will not be lost.
	sub, err := h.subscriber.Subscribe(r.Context(), submissionID)
	if err != nil {
		log.Printf("[sse] failed to subscribe to pubsub for submission %s: %v", submissionID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "internal server error",
		})
		return
	}
	defer sub.Close()

	// Step B: Query authoritative PostgreSQL state scoped strictly to the authenticated user.
	entity, err := h.repo.GetByIDAndUserID(r.Context(), submissionID, userID)
	if err != nil {
		if errors.Is(err, submission.ErrSubmissionNotFound) {
			// Do not leak whether another user's submission exists
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "submission not found",
			})
			return
		}
		log.Printf("[sse] database lookup error for submission %s: %v", submissionID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "internal server error",
		})
		return
	}

	// 5. Send SSE headers now that authentication, ownership, and subscription succeed.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable proxy buffering for nginx/proxies
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// 6. If already terminal in PostgreSQL, immediately emit final event and exit.
	if IsTerminalStatus(entity.Status) {
		_ = writeSSEEvent(w, "submission", PayloadFromEntity(entity))
		flusher.Flush()
		return
	}

	// Emit initial non-terminal state (e.g. QUEUED or PROCESSING)
	_ = writeSSEEvent(w, "submission", PayloadFromEntity(entity))
	flusher.Flush()

	// 7. Event loop: listen for Pub/Sub events, keepalive ticker, or client disconnect.
	ticker := time.NewTicker(h.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			// Client disconnected: clean exit without leaking goroutines or modifying state.
			return

		case <-ticker.C:
			// Heartbeat comment
			_, err := fmt.Fprintf(w, ": keepalive\n\n")
			if err != nil {
				return
			}
			flusher.Flush()

		case _, ok := <-sub.Channel():
			if !ok {
				// Subscription channel closed
				return
			}

			// A notification was received. Treat Redis Pub/Sub only as a wake-up signal;
			// query PostgreSQL for authoritative submission state.
			authEntity, err := h.repo.GetByIDAndUserID(r.Context(), submissionID, userID)
			if err != nil {
				log.Printf("[sse] authoritative fetch error: %v", err)
				return
			}

			if err := writeSSEEvent(w, "submission", PayloadFromEntity(authEntity)); err != nil {
				return
			}
			flusher.Flush()

			// If state reached terminal, close the stream cleanly.
			if IsTerminalStatus(authEntity.Status) {
				return
			}
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	return err
}

func extractSubmissionID(r *http.Request) string {
	if id := r.PathValue("id"); id != "" {
		return id
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/submissions/")
	path = strings.TrimSuffix(path, "/stream")
	parts := strings.Split(path, "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}
