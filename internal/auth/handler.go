package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

// Handler handles HTTP requests for user authentication.
type Handler struct {
	service AuthService
}

// NewHandler constructs a new auth Handler.
func NewHandler(service AuthService) *Handler {
	return &Handler{service: service}
}

// Register handles user registration: POST /api/auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req RegisterRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	res, err := h.service.Register(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrDuplicateEmail) {
			writeJSONError(w, http.StatusConflict, "email already registered")
			return
		}
		if errors.Is(err, ErrInvalidInput) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("[auth] unexpected registration error: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}

// Login handles user login: POST /api/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req LoginRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	res, err := h.service.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			writeJSONError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		log.Printf("[auth] unexpected login error: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

// Me handles fetching the authenticated user's profile: GET /api/auth/me.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.service.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		log.Printf("[auth] unexpected get user error: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"user": user,
	})
}
