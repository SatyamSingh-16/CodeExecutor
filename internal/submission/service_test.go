package submission

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockSubmissionRepo struct {
	mu          sync.Mutex
	submissions map[string]*SubmissionEntity
	createErr   error
	getErr      error
}

func newMockSubmissionRepo() *mockSubmissionRepo {
	return &mockSubmissionRepo{
		submissions: make(map[string]*SubmissionEntity),
	}
}

func (m *mockSubmissionRepo) Create(ctx context.Context, sub *SubmissionEntity) (*SubmissionEntity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return nil, m.createErr
	}

	id := fmt.Sprintf("sub-uuid-%d", len(m.submissions)+1)
	entity := &SubmissionEntity{
		ID:         id,
		UserID:     sub.UserID,
		Language:   sub.Language,
		Code:       sub.Code,
		Stdin:      sub.Stdin,
		Status:     "QUEUED",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		RetryCount: 0,
	}
	m.submissions[id] = entity
	return entity, nil
}

func (m *mockSubmissionRepo) GetByIDAndUserID(ctx context.Context, id, userID string) (*SubmissionEntity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getErr != nil {
		return nil, m.getErr
	}

	sub, ok := m.submissions[id]
	if !ok || sub.UserID != userID {
		return nil, ErrSubmissionNotFound
	}
	return sub, nil
}

func (m *mockSubmissionRepo) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*SubmissionEntity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*SubmissionEntity
	for _, sub := range m.submissions {
		if sub.UserID == userID {
			result = append(result, sub)
		}
	}
	return result, nil
}

type mockQueue struct {
	mu         sync.Mutex
	enqueued   []string
	enqueueErr error
}

func (q *mockQueue) Enqueue(ctx context.Context, submissionID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.enqueueErr != nil {
		return q.enqueueErr
	}
	q.enqueued = append(q.enqueued, submissionID)
	return nil
}

func (q *mockQueue) InitConsumerGroup(ctx context.Context) error {
	return nil
}

func (q *mockQueue) Close() error {
	return nil
}

func TestSubmissionService_Create_Success_Python(t *testing.T) {
	repo := newMockSubmissionRepo()
	q := &mockQueue{}
	svc := NewService(repo, q)

	req := CreateSubmissionRequest{
		Language:   "Python", // Case-insensitive
		SourceCode: "print('hello world')",
		Stdin:      "input_data",
	}

	res, err := svc.CreateSubmission(context.Background(), "user-1", req)
	if err != nil {
		t.Fatalf("unexpected error creating python submission: %v", err)
	}

	if res.ID == "" {
		t.Error("expected non-empty submission ID")
	}
	if res.Status != "QUEUED" {
		t.Errorf("expected status QUEUED, got %s", res.Status)
	}

	// Verify enqueued into Redis stream
	q.mu.Lock()
	if len(q.enqueued) != 1 || q.enqueued[0] != res.ID {
		t.Errorf("expected submission ID %s to be enqueued, got %v", res.ID, q.enqueued)
	}
	q.mu.Unlock()

	// Verify database record
	repo.mu.Lock()
	stored := repo.submissions[res.ID]
	repo.mu.Unlock()
	if stored == nil || stored.Language != "python" || stored.Code != "print('hello world')" {
		t.Errorf("database record mismatch: %+v", stored)
	}
}

func TestSubmissionService_Create_Success_Go(t *testing.T) {
	repo := newMockSubmissionRepo()
	q := &mockQueue{}
	svc := NewService(repo, q)

	req := CreateSubmissionRequest{
		Language: "go",
		Code:     "package main\nfunc main() {}",
	}

	res, err := svc.CreateSubmission(context.Background(), "user-1", req)
	if err != nil {
		t.Fatalf("unexpected error creating go submission: %v", err)
	}
	if res.Status != "QUEUED" {
		t.Errorf("expected status QUEUED, got %s", res.Status)
	}
}

func TestSubmissionService_Create_ValidationFailures(t *testing.T) {
	repo := newMockSubmissionRepo()
	q := &mockQueue{}
	svc := NewService(repo, q)

	testCases := []struct {
		name        string
		userID      string
		lang        string
		code        string
		stdin       string
		expectedErr error
	}{
		{"missing user id", "", "python", "print(1)", "", ErrInvalidInput},
		{"unsupported language", "u1", "rust", "fn main() {}", "", ErrUnsupportedLanguage},
		{"empty language", "u1", "", "print(1)", "", ErrUnsupportedLanguage},
		{"empty source code", "u1", "python", "   ", "", ErrEmptySourceCode},
		{"code exceeds 64KB", "u1", "python", strings.Repeat("a", 65*1024), "", ErrPayloadTooLarge},
		{"stdin exceeds 64KB", "u1", "python", "print(1)", strings.Repeat("b", 65*1024), ErrPayloadTooLarge},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := CreateSubmissionRequest{
				Language:   tc.lang,
				SourceCode: tc.code,
				Stdin:      tc.stdin,
			}
			_, err := svc.CreateSubmission(context.Background(), tc.userID, req)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("expected %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

func TestSubmissionService_Create_DualWrite_PostgresFails_NoQueue(t *testing.T) {
	repo := newMockSubmissionRepo()
	repo.createErr = errors.New("db disk full")
	q := &mockQueue{}
	svc := NewService(repo, q)

	req := CreateSubmissionRequest{Language: "python", SourceCode: "print(1)"}
	_, err := svc.CreateSubmission(context.Background(), "user-1", req)
	if err == nil {
		t.Fatal("expected error on DB failure, got nil")
	}

	// Verify nothing was enqueued to Redis
	q.mu.Lock()
	if len(q.enqueued) != 0 {
		t.Errorf("expected 0 messages enqueued when DB fails, got %d", len(q.enqueued))
	}
	q.mu.Unlock()
}

func TestSubmissionService_Create_DualWrite_RedisFails_RecordPreserved(t *testing.T) {
	repo := newMockSubmissionRepo()
	q := &mockQueue{enqueueErr: errors.New("redis connection reset")}
	svc := NewService(repo, q)

	req := CreateSubmissionRequest{Language: "python", SourceCode: "print(1)"}
	_, err := svc.CreateSubmission(context.Background(), "user-1", req)
	if err == nil {
		t.Fatal("expected error on Redis enqueue failure, got nil")
	}

	// CRITICAL DUAL-WRITE INVARIANT:
	// Record must be preserved in PostgreSQL with status QUEUED for sweeper reconciliation
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.submissions) != 1 {
		t.Fatalf("expected 1 record preserved in DB, got %d", len(repo.submissions))
	}
	for _, sub := range repo.submissions {
		if sub.Status != "QUEUED" {
			t.Errorf("expected preserved record status QUEUED, got %s", sub.Status)
		}
	}
}

func TestSubmissionService_Get_OwnershipEnforced(t *testing.T) {
	repo := newMockSubmissionRepo()
	q := &mockQueue{}
	svc := NewService(repo, q)

	// User A creates submission
	resA, _ := svc.CreateSubmission(context.Background(), "user-A", CreateSubmissionRequest{
		Language:   "python",
		SourceCode: "print('user A secret')",
	})

	// User A retrieves submission -> OK
	detailA, err := svc.GetSubmission(context.Background(), "user-A", resA.ID)
	if err != nil {
		t.Fatalf("owner failed to retrieve submission: %v", err)
	}
	if detailA.ID != resA.ID {
		t.Errorf("expected ID %s, got %s", resA.ID, detailA.ID)
	}

	// User B retrieves User A submission -> ErrSubmissionNotFound (404)
	_, err = svc.GetSubmission(context.Background(), "user-B", resA.ID)
	if err == nil {
		t.Fatal("expected non-owner to be rejected, got nil error")
	}
	if !errors.Is(err, ErrSubmissionNotFound) {
		t.Errorf("expected ErrSubmissionNotFound, got: %v", err)
	}

	// Nonexistent ID -> ErrSubmissionNotFound
	_, err = svc.GetSubmission(context.Background(), "user-A", "non-existent-uuid")
	if !errors.Is(err, ErrSubmissionNotFound) {
		t.Errorf("expected ErrSubmissionNotFound for nonexistent, got: %v", err)
	}
}

func TestSubmissionService_List_UserScoped(t *testing.T) {
	repo := newMockSubmissionRepo()
	q := &mockQueue{}
	svc := NewService(repo, q)

	_, _ = svc.CreateSubmission(context.Background(), "user-X", CreateSubmissionRequest{Language: "python", SourceCode: "x1"})
	_, _ = svc.CreateSubmission(context.Background(), "user-X", CreateSubmissionRequest{Language: "python", SourceCode: "x2"})
	_, _ = svc.CreateSubmission(context.Background(), "user-Y", CreateSubmissionRequest{Language: "python", SourceCode: "y1"})

	listX, err := svc.ListSubmissions(context.Background(), "user-X", 20, 0)
	if err != nil {
		t.Fatalf("failed to list user X submissions: %v", err)
	}
	if len(listX) != 2 {
		t.Errorf("expected 2 submissions for user X, got %d", len(listX))
	}

	listY, err := svc.ListSubmissions(context.Background(), "user-Y", 20, 0)
	if err != nil {
		t.Fatalf("failed to list user Y submissions: %v", err)
	}
	if len(listY) != 1 {
		t.Errorf("expected 1 submission for user Y, got %d", len(listY))
	}
}
