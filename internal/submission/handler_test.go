package submission

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
)

func setupTestSubmissionHandler() (*Handler, *Service, *mockSubmissionRepo, *mockQueue) {
	repo := newMockSubmissionRepo()
	q := &mockQueue{}
	svc := NewService(repo, q)
	h := NewHandler(svc)
	return h, svc, repo, q
}

func TestHandler_Create_Success_202(t *testing.T) {
	h, _, _, q := setupTestSubmissionHandler()

	body, _ := json.Marshal(CreateSubmissionRequest{
		Language:   "python",
		SourceCode: "print('success')",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader(body))
	req = req.WithContext(auth.WithUserID(req.Context(), "user-123"))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202 Accepted, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp CreateSubmissionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID == "" {
		t.Error("expected non-empty submission ID")
	}
	if resp.Status != "QUEUED" {
		t.Errorf("expected status QUEUED, got %s", resp.Status)
	}

	// Verify enqueued
	q.mu.Lock()
	if len(q.enqueued) != 1 || q.enqueued[0] != resp.ID {
		t.Errorf("expected %s enqueued, got %v", resp.ID, q.enqueued)
	}
	q.mu.Unlock()
}

func TestHandler_Create_DisallowUnknownFields_ClientSuppliedUserIDRejected(t *testing.T) {
	h, _, _, _ := setupTestSubmissionHandler()

	// Attacker tries to inject user_id to spoof another user
	payload := `{"language":"python","source_code":"print(1)","user_id":"victim-user-id"}`

	req := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader([]byte(payload)))
	req = req.WithContext(auth.WithUserID(req.Context(), "attacker-user-id"))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when unknown field 'user_id' is supplied, got %d", rec.Code)
	}
}

func TestHandler_Create_Unauthenticated_401(t *testing.T) {
	h, _, _, _ := setupTestSubmissionHandler()

	body, _ := json.Marshal(CreateSubmissionRequest{Language: "python", SourceCode: "print(1)"})
	// No user ID in context
	req := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated request, got %d", rec.Code)
	}
}

func TestHandler_Create_InvalidInputs_400(t *testing.T) {
	h, _, _, _ := setupTestSubmissionHandler()

	testCases := []struct {
		name string
		json string
	}{
		{"invalid json", "{bad-json"},
		{"unsupported language", `{"language":"csharp","source_code":"print(1)"}`},
		{"empty code", `{"language":"python","source_code":""}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader([]byte(tc.json)))
			req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
			rec := httptest.NewRecorder()

			h.Create(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 Bad Request for %s, got %d", tc.name, rec.Code)
			}
		})
	}
}

func TestHandler_Get_Success_200(t *testing.T) {
	h, svc, _, _ := setupTestSubmissionHandler()

	res, _ := svc.CreateSubmission(t.Context(), "user-owner", CreateSubmissionRequest{
		Language:   "python",
		SourceCode: "print('my code')",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/submissions/"+res.ID, nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-owner"))
	rec := httptest.NewRecorder()

	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var detail SubmissionDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if detail.ID != res.ID {
		t.Errorf("expected ID %s, got %s", res.ID, detail.ID)
	}
	if detail.SourceCode != "print('my code')" {
		t.Errorf("expected source code, got %s", detail.SourceCode)
	}
}

func TestHandler_Get_OwnershipEnforced_404(t *testing.T) {
	h, svc, _, _ := setupTestSubmissionHandler()

	// User A creates submission
	resA, _ := svc.CreateSubmission(t.Context(), "user-A", CreateSubmissionRequest{
		Language:   "python",
		SourceCode: "print('classified A')",
	})

	// User B attempts to access User A's submission
	reqB := httptest.NewRequest(http.MethodGet, "/api/submissions/"+resA.ID, nil)
	reqB = reqB.WithContext(auth.WithUserID(reqB.Context(), "user-B"))
	recB := httptest.NewRecorder()

	h.Get(recB, reqB)

	// STRICT SPEC REQUIREMENT: return 404 to avoid leaking existence of other users' submissions
	if recB.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for non-owner access, got %d. Body: %s", recB.Code, recB.Body.String())
	}

	var errResp map[string]string
	_ = json.Unmarshal(recB.Body.Bytes(), &errResp)
	if errResp["error"] != "submission not found" {
		t.Errorf("expected 'submission not found', got %q", errResp["error"])
	}
}

func TestHandler_List_Success_200(t *testing.T) {
	h, svc, _, _ := setupTestSubmissionHandler()

	_, _ = svc.CreateSubmission(t.Context(), "user-list", CreateSubmissionRequest{Language: "python", SourceCode: "1"})
	_, _ = svc.CreateSubmission(t.Context(), "user-list", CreateSubmissionRequest{Language: "python", SourceCode: "2"})

	req := httptest.NewRequest(http.MethodGet, "/api/submissions", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-list"))
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var list []SubmissionDetailResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Errorf("expected 2 submissions, got %d", len(list))
	}
}
