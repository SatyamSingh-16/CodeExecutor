package sse

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
	"github.com/SatyamSingh-16/code_executor/internal/submission"
)

type mockSubscription struct {
	msgCh    chan string
	closed   atomic.Bool
	onClose  func()
}

func (m *mockSubscription) Channel() <-chan string {
	return m.msgCh
}

func (m *mockSubscription) Close() error {
	if m.closed.CompareAndSwap(false, true) {
		close(m.msgCh)
		if m.onClose != nil {
			m.onClose()
		}
	}
	return nil
}

type mockSubscriber struct {
	mu           sync.Mutex
	subscriptions map[string]*mockSubscription
	subscribeErr error
	subscribeHook func(submissionID string)
}

func newMockSubscriber() *mockSubscriber {
	return &mockSubscriber{
		subscriptions: make(map[string]*mockSubscription),
	}
}

func (m *mockSubscriber) Subscribe(ctx context.Context, submissionID string) (Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}

	if m.subscribeHook != nil {
		m.subscribeHook(submissionID)
	}

	sub := &mockSubscription{
		msgCh: make(chan string, 10),
	}
	m.subscriptions[submissionID] = sub
	return sub, nil
}

func (m *mockSubscriber) Publish(submissionID, payload string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sub, ok := m.subscriptions[submissionID]; ok && !sub.closed.Load() {
		sub.msgCh <- payload
	}
}

type mockRepo struct {
	mu          sync.Mutex
	submissions map[string]*submission.SubmissionEntity
	getHook     func(id, userID string)
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		submissions: make(map[string]*submission.SubmissionEntity),
	}
}

func (m *mockRepo) GetByIDAndUserID(ctx context.Context, id, userID string) (*submission.SubmissionEntity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getHook != nil {
		m.getHook(id, userID)
	}

	sub, ok := m.submissions[id]
	if !ok || sub.UserID != userID {
		return nil, submission.ErrSubmissionNotFound
	}
	return sub, nil
}

func (m *mockRepo) set(sub *submission.SubmissionEntity) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submissions[sub.ID] = sub
}

// flushRecorder implements http.ResponseWriter and http.Flusher
type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flushRecorder) Flush() {
	f.flushed = true
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{
		ResponseRecorder: httptest.NewRecorder(),
	}
}

type nonFlusherWriter struct {
	header http.Header
	code   int
	body   strings.Builder
}

func newNonFlusherWriter() *nonFlusherWriter {
	return &nonFlusherWriter{
		header: make(http.Header),
		code:   http.StatusOK,
	}
}

func (n *nonFlusherWriter) Header() http.Header {
	return n.header
}

func (n *nonFlusherWriter) Write(b []byte) (int, error) {
	return n.body.Write(b)
}

func (n *nonFlusherWriter) WriteHeader(statusCode int) {
	n.code = statusCode
}

func TestSSEHandler_NonFlusherResponseWriter(t *testing.T) {
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	handler := NewHandler(repo, subscriber)

	req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-1/stream", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	req.SetPathValue("id", "sub-1")

	w := newNonFlusherWriter()
	handler.Stream(w, req)

	if w.code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 for non-flusher, got %d", w.code)
	}
	if !strings.Contains(w.body.String(), "streaming unsupported") {
		t.Fatalf("expected error message about unsupported streaming, got %s", w.body.String())
	}
}

func TestSSEHandler_AuthenticationRequired(t *testing.T) {
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	handler := NewHandler(repo, subscriber)

	req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-1/stream", nil)
	req.SetPathValue("id", "sub-1")

	w := newFlushRecorder()
	handler.Stream(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for unauthenticated request, got %d", w.Code)
	}
}

func TestSSEHandler_NonOwnerReturns404(t *testing.T) {
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	handler := NewHandler(repo, subscriber)

	repo.set(&submission.SubmissionEntity{
		ID:     "sub-1",
		UserID: "owner-user",
		Status: "QUEUED",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-1/stream", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "other-user"))
	req.SetPathValue("id", "sub-1")

	w := newFlushRecorder()
	handler.Stream(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-owner, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "owner-user") {
		t.Fatalf("response leaked owner data")
	}
}

func TestSSEHandler_NonexistentSubmissionReturns404(t *testing.T) {
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	handler := NewHandler(repo, subscriber)

	req := httptest.NewRequest(http.MethodGet, "/api/submissions/nonexistent/stream", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	req.SetPathValue("id", "nonexistent")

	w := newFlushRecorder()
	handler.Stream(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent submission, got %d", w.Code)
	}
}

func TestSSEHandler_HeadersAndFlusher(t *testing.T) {
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	handler := NewHandler(repo, subscriber)

	exitCode := 0
	execTime := int64(15)
	memUsage := int64(5000)
	repo.set(&submission.SubmissionEntity{
		ID:              "sub-1",
		UserID:          "user-1",
		Status:          "SUCCESS",
		Stdout:          "hello world\n",
		ExitCode:        &exitCode,
		ExecutionTimeMs: &execTime,
		MemoryUsageKb:   &memUsage,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-1/stream", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	req.SetPathValue("id", "sub-1")

	w := newFlushRecorder()
	handler.Stream(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %q", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("expected Connection keep-alive, got %q", conn)
	}
	if !w.flushed {
		t.Errorf("expected flusher to be called")
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: submission\n") {
		t.Errorf("expected event framing in response body: %s", body)
	}
	if !strings.Contains(body, `"status":"SUCCESS"`) {
		t.Errorf("expected status SUCCESS in event data: %s", body)
	}
	if !strings.Contains(body, `"stdout":"hello world\n"`) {
		t.Errorf("expected stdout in event data: %s", body)
	}
}

func TestSSEHandler_AlreadyTerminalClosesImmediately(t *testing.T) {
	terminalStatuses := []string{
		"SUCCESS",
		"COMPILATION_ERROR",
		"RUNTIME_ERROR",
		"TIME_LIMIT_EXCEEDED",
		"MEMORY_LIMIT_EXCEEDED",
		"SYSTEM_ERROR",
	}

	for _, status := range terminalStatuses {
		t.Run(status, func(t *testing.T) {
			repo := newMockRepo()
			subscriber := newMockSubscriber()
			handler := NewHandler(repo, subscriber)

			repo.set(&submission.SubmissionEntity{
				ID:     "sub-term",
				UserID: "user-1",
				Status: status,
			})

			req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-term/stream", nil)
			req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
			req.SetPathValue("id", "sub-term")

			w := newFlushRecorder()
			done := make(chan struct{})
			go func() {
				handler.Stream(w, req)
				close(done)
			}()

			select {
			case <-done:
				// Succeeded in exiting immediately without hanging
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("handler did not close immediately for terminal status %s", status)
			}

			body := w.Body.String()
			if !strings.Contains(body, fmt.Sprintf(`"status":"%s"`, status)) {
				t.Fatalf("expected payload to have status %s, got: %s", status, body)
			}
		})
	}
}

func TestSSEHandler_LiveStreamingToTerminal(t *testing.T) {
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	handler := NewHandler(repo, subscriber, WithHeartbeatInterval(1*time.Hour))

	repo.set(&submission.SubmissionEntity{
		ID:     "sub-live",
		UserID: "user-1",
		Status: "QUEUED",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-live/stream", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	req.SetPathValue("id", "sub-live")

	w := newFlushRecorder()
	done := make(chan struct{})

	go func() {
		handler.Stream(w, req)
		close(done)
	}()

	// Wait for initial event
	time.Sleep(50 * time.Millisecond)

	// Update DB to PROCESSING and publish event
	repo.set(&submission.SubmissionEntity{
		ID:     "sub-live",
		UserID: "user-1",
		Status: "PROCESSING",
	})
	subscriber.Publish("sub-live", `{"status":"PROCESSING"}`)

	time.Sleep(50 * time.Millisecond)

	// Stream should still be open
	select {
	case <-done:
		t.Fatalf("handler closed prematurely on PROCESSING status")
	default:
	}

	// Update DB to SUCCESS and publish terminal event
	repo.set(&submission.SubmissionEntity{
		ID:     "sub-live",
		UserID: "user-1",
		Status: "SUCCESS",
		Stdout: "42\n",
	})
	subscriber.Publish("sub-live", `{"status":"SUCCESS"}`)

	// Now handler must exit cleanly
	select {
	case <-done:
		// Clean exit
	case <-time.After(1 * time.Second):
		t.Fatalf("handler failed to terminate upon receiving terminal event")
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"QUEUED"`) {
		t.Errorf("expected initial QUEUED event in body")
	}
	if !strings.Contains(body, `"status":"PROCESSING"`) {
		t.Errorf("expected PROCESSING event in body")
	}
	if !strings.Contains(body, `"status":"SUCCESS"`) {
		t.Errorf("expected terminal SUCCESS event in body")
	}
	if !strings.Contains(body, `"stdout":"42\n"`) {
		t.Errorf("expected stdout in body: %s", body)
	}
}

func TestSSEHandler_ClientDisconnectCleansUp(t *testing.T) {
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	handler := NewHandler(repo, subscriber)

	repo.set(&submission.SubmissionEntity{
		ID:     "sub-dc",
		UserID: "user-1",
		Status: "QUEUED",
	})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-dc/stream", nil).WithContext(ctx)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	req.SetPathValue("id", "sub-dc")

	w := newFlushRecorder()
	done := make(chan struct{})

	go func() {
		handler.Stream(w, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	// Cancel context to simulate client disconnect
	cancel()

	select {
	case <-done:
		// Exited promptly upon disconnect
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handler did not exit promptly upon client disconnect")
	}
}

func TestSSEHandler_RaceCondition_WorkerCompletesDuringConnection(t *testing.T) {
	// Simulate:
	// Worker updates DB to SUCCESS right as SSE client subscribes.
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	handler := NewHandler(repo, subscriber)

	// DB begins at QUEUED
	repo.set(&submission.SubmissionEntity{
		ID:     "sub-race",
		UserID: "user-1",
		Status: "QUEUED",
	})

	// Hook: as soon as Subscribe is called, worker finishes in DB and publishes event!
	subscriber.subscribeHook = func(submissionID string) {
		repo.set(&submission.SubmissionEntity{
			ID:     submissionID,
			UserID: "user-1",
			Status: "SUCCESS",
			Stdout: "race resolved\n",
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-race/stream", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	req.SetPathValue("id", "sub-race")

	w := newFlushRecorder()
	done := make(chan struct{})

	go func() {
		handler.Stream(w, req)
		close(done)
	}()

	select {
	case <-done:
		// Handler immediately detected that DB was SUCCESS when it read authoritative state!
	case <-time.After(1 * time.Second):
		t.Fatalf("handler failed to resolve race condition")
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"SUCCESS"`) {
		t.Fatalf("expected SUCCESS status in response, got: %s", body)
	}
	if !strings.Contains(body, `"stdout":"race resolved\n"`) {
		t.Fatalf("expected authoritative stdout, got: %s", body)
	}
}

func TestSSEHandler_PubSubFailureDoesNotCorruptDB(t *testing.T) {
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	subscriber.subscribeErr = errors.New("redis pubsub connection refused")
	handler := NewHandler(repo, subscriber)

	repo.set(&submission.SubmissionEntity{
		ID:     "sub-err",
		UserID: "user-1",
		Status: "QUEUED",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-err/stream", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	req.SetPathValue("id", "sub-err")

	w := newFlushRecorder()
	handler.Stream(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when Redis subscribe fails, got %d", w.Code)
	}

	// Verify DB is untouched
	entity, err := repo.GetByIDAndUserID(context.Background(), "sub-err", "user-1")
	if err != nil || entity.Status != "QUEUED" {
		t.Fatalf("expected DB status to remain QUEUED, got %+v (err: %v)", entity, err)
	}
}

func TestSSEHandler_HeartbeatFlushes(t *testing.T) {
	repo := newMockRepo()
	subscriber := newMockSubscriber()
	// Configure ultra-short heartbeat for testing
	handler := NewHandler(repo, subscriber, WithHeartbeatInterval(20*time.Millisecond))

	repo.set(&submission.SubmissionEntity{
		ID:     "sub-hb",
		UserID: "user-1",
		Status: "QUEUED",
	})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/submissions/sub-hb/stream", nil).WithContext(ctx)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	req.SetPathValue("id", "sub-hb")

	w := newFlushRecorder()
	done := make(chan struct{})

	go func() {
		handler.Stream(w, req)
		close(done)
	}()

	// Wait for at least one heartbeat
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, ": keepalive\n\n") {
		t.Fatalf("expected keepalive heartbeat in body, got: %s", body)
	}
}
