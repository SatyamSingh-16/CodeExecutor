package submission

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
)

// Handler handles HTTP endpoints for code execution submissions.
type Handler struct {
	service SubmissionService
}

// NewHandler constructs a new submission Handler.
func NewHandler(service SubmissionService) *Handler {
	return &Handler{service: service}
}

// Create handles POST /api/submissions.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreateSubmissionRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, MaxRequestBodyBytes))
	dec.DisallowUnknownFields() // Strictly disallows unknown fields including client-supplied user_id
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	res, err := h.service.CreateSubmission(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrUnsupportedLanguage) ||
			errors.Is(err, ErrEmptySourceCode) ||
			errors.Is(err, ErrPayloadTooLarge) ||
			errors.Is(err, ErrInvalidInput) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("[submission] creation error: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(res)
}

// Get handles GET /api/submissions/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := extractSubmissionID(r)
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing submission id")
		return
	}

	sub, err := h.service.GetSubmission(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, ErrSubmissionNotFound) {
			// Strictly return 404 for nonexistent or non-owner submissions to prevent enumeration
			writeJSONError(w, http.StatusNotFound, "submission not found")
			return
		}
		log.Printf("[submission] get error: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(sub)
}

// List handles GET /api/submissions.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := 20
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 1 {
			offset = (page - 1) * limit
		}
	} else if offStr := r.URL.Query().Get("offset"); offStr != "" {
		if parsed, err := strconv.Atoi(offStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	submissions, err := h.service.ListSubmissions(r.Context(), userID, limit, offset)
	if err != nil {
		log.Printf("[submission] list error: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(submissions)
}

// RouteSubmissions is a multiplexer helper supporting both /api/submissions and /api/submissions/{id}.
func (h *Handler) RouteSubmissions(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/stream") {
		// Handled by SSE stream handler
		http.NotFound(w, r)
		return
	}

	id := extractSubmissionID(r)
	if id != "" {
		h.Get(w, r)
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.Create(w, r)
	case http.MethodGet:
		h.List(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func extractSubmissionID(r *http.Request) string {
	// Standard library Go 1.22+ path value
	if id := r.PathValue("id"); id != "" {
		return id
	}

	// Suffix path parsing fallback
	path := strings.TrimPrefix(r.URL.Path, "/api/submissions")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return ""
	}
	// Return first segment
	parts := strings.Split(path, "/")
	return parts[0]
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
