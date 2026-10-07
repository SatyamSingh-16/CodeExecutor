//go:build integration

package api_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
	"github.com/SatyamSingh-16/code_executor/internal/queue"
	"github.com/SatyamSingh-16/code_executor/internal/ratelimit"
	"github.com/SatyamSingh-16/code_executor/internal/sse"
	"github.com/SatyamSingh-16/code_executor/internal/submission"
	"github.com/SatyamSingh-16/code_executor/internal/worker"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/redis/go-redis/v9"
)

// nginxHarness manages the test Nginx container proxying to a live Go API test server.
type nginxHarness struct {
	nginxURL    string
	containerID string
	cli         *client.Client
	apiServer   *httptest.Server
	authSvc     *auth.Service
	db          *sql.DB
	rdb         *redis.Client
	subRepo     *submission.PostgresSubmissionRepository
	publisher   *worker.RedisEventPublisher
}

func getOutboundHostIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "host.docker.internal"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func setupNginxHarness(t *testing.T) *nginxHarness {
	t.Helper()

	db := getIntegrationDB(t)
	rdb := getIntegrationRedis(t)

	// Auth components
	tokenMgr, err := auth.NewJWTTokenManager(auth.JWTConfig{
		Secret:     "nginx-integration-test-secret-32b!",
		Expiration: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	userRepo := auth.NewPostgresUserRepository(db)
	authSvc := auth.NewService(userRepo, tokenMgr)
	authHandler := auth.NewHandler(authSvc)
	authMiddleware := auth.NewMiddleware(tokenMgr)

	streamName := fmt.Sprintf("nginx_sub_stream_%d", time.Now().UnixNano())
	testQueue := queue.NewRedisQueue(rdb, streamName, "test_group")
	_ = testQueue.InitConsumerGroup(context.Background())

	limiterKeyPrefix := fmt.Sprintf("nginx_rate_%d:", time.Now().UnixNano())
	limiter := ratelimit.NewRedisSlidingWindowLimiter(rdb, ratelimit.Config{
		Limit:     100,
		Window:    10 * time.Second,
		KeyPrefix: limiterKeyPrefix,
	})
	rateMiddleware := ratelimit.NewMiddleware(limiter)

	subRepo := submission.NewPostgresSubmissionRepository(db)
	subSvc := submission.NewService(subRepo, testQueue)
	subHandler := submission.NewHandler(subSvc)

	sseSubscriber := sse.NewRedisSubscriber(rdb)
	sseHandler := sse.NewHandler(subRepo, sseSubscriber, sse.WithHeartbeatInterval(1*time.Second))
	publisher := worker.NewRedisEventPublisher(rdb)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.Handle("/api/auth/me", authMiddleware.RequireAuth(http.HandlerFunc(authHandler.Me)))

	// Health check endpoints matching cmd/api/main.go
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/health", healthHandler)

	// SSE Stream route
	mux.Handle("GET /api/submissions/{id}/stream", authMiddleware.RequireAuth(http.HandlerFunc(sseHandler.Stream)))

	// Submission routes
	mux.Handle("POST /api/submissions", authMiddleware.RequireAuth(rateMiddleware.RequireRateLimit(http.HandlerFunc(subHandler.Create))))
	mux.Handle("GET /api/submissions", authMiddleware.RequireAuth(http.HandlerFunc(subHandler.List)))
	mux.Handle("GET /api/submissions/{id}", authMiddleware.RequireAuth(http.HandlerFunc(subHandler.Get)))
	mux.Handle("/api/submissions/", authMiddleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stream") {
			sseHandler.Stream(w, r)
			return
		}
		subHandler.RouteSubmissions(w, r)
	})))

	// Start Go backend on all interfaces so Docker container can reach it
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("failed to listen on random port: %v", err)
	}
	apiServer := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: mux},
	}
	apiServer.Start()

	apiPort := listener.Addr().(*net.TCPAddr).Port
	hostIP := getOutboundHostIP()

	// Connect to Docker daemon
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("failed to connect to Docker daemon: %v", err)
	}

	// Prepare temporary Nginx configuration for container
	tmpDir, err := os.MkdirTemp("", "nginx-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	nginxConfPath := filepath.Join(tmpDir, "nginx.conf")
	defaultConfPath := filepath.Join(tmpDir, "default.conf")

	repoRoot := getRepoRoot(t)
	origNginxConf, err := os.ReadFile(filepath.Join(repoRoot, "docker/nginx/nginx.conf"))
	if err != nil {
		t.Fatalf("failed to read docker/nginx/nginx.conf: %v", err)
	}
	_ = os.WriteFile(nginxConfPath, origNginxConf, 0644)

	// Read default.conf and substitute upstream address to target our test Go API
	origDefaultConf, err := os.ReadFile(filepath.Join(repoRoot, "docker/nginx/default.conf"))
	if err != nil {
		t.Fatalf("failed to read docker/nginx/default.conf: %v", err)
	}
	upstreamTarget := fmt.Sprintf("%s:%d", hostIP, apiPort)
	customDefaultConf := strings.Replace(string(origDefaultConf), "server api:8080;", fmt.Sprintf("server %s;", upstreamTarget), 1)
	_ = os.WriteFile(defaultConfPath, []byte(customDefaultConf), 0644)

	// Create and start Nginx container
	containerConfig := &container.Config{
		Image: "nginx:alpine",
		ExposedPorts: nat.PortSet{
			"80/tcp": struct{}{},
		},
	}
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"80/tcp": []nat.PortBinding{
				{HostIP: "127.0.0.1", HostPort: "0"},
			},
		},
		Binds: []string{
			fmt.Sprintf("%s:/etc/nginx/nginx.conf:ro", nginxConfPath),
			fmt.Sprintf("%s:/etc/nginx/conf.d/default.conf:ro", defaultConfPath),
		},
		ExtraHosts: []string{
			fmt.Sprintf("host.docker.internal:%s", hostIP),
		},
	}

	createResp, err := cli.ContainerCreate(context.Background(), containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		t.Fatalf("failed to create Nginx container: %v", err)
	}
	containerID := createResp.ID

	if err := cli.ContainerStart(context.Background(), containerID, container.StartOptions{}); err != nil {
		_ = cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: true})
		t.Fatalf("failed to start Nginx container: %v", err)
	}

	// Inspect assigned port
	inspect, err := cli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		_ = cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: true})
		t.Fatalf("failed to inspect Nginx container: %v", err)
	}

	bindings := inspect.NetworkSettings.Ports["80/tcp"]
	if len(bindings) == 0 {
		_ = cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: true})
		t.Fatalf("no port bindings found for Nginx container")
	}
	nginxPort := bindings[0].HostPort
	nginxURL := fmt.Sprintf("http://127.0.0.1:%s", nginxPort)

	// Wait for Nginx to be ready
	ready := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(nginxURL + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		_ = cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: true})
		t.Fatalf("Nginx container failed to become ready at %s", nginxURL)
	}

	harness := &nginxHarness{
		nginxURL:    nginxURL,
		containerID: containerID,
		cli:         cli,
		apiServer:   apiServer,
		authSvc:     authSvc,
		db:          db,
		rdb:         rdb,
		subRepo:     subRepo,
		publisher:   publisher,
	}

	t.Cleanup(func() {
		_ = cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: true})
		_ = cli.Close()
		apiServer.Close()
		_ = os.RemoveAll(tmpDir)
		_ = rdb.Del(context.Background(), streamName)
		_ = db.Close()
		_ = rdb.Close()
	})

	return harness
}

// 1. NGINX CONFIGURATION SYNTAX CHECK (nginx -t)
func TestNginx_ConfigSyntaxValidation(t *testing.T) {
	cmd := exec.Command("docker", "run", "--rm",
		"-v", fmt.Sprintf("%s/docker/nginx/nginx.conf:/etc/nginx/nginx.conf:ro", getRepoRoot(t)),
		"-v", fmt.Sprintf("%s/docker/nginx/default.conf:/etc/nginx/conf.d/default.conf:ro", getRepoRoot(t)),
		"--add-host=api:127.0.0.1",
		"nginx:alpine", "nginx", "-t",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nginx -t failed: %v\nOutput: %s", err, string(out))
	}
	if !strings.Contains(string(out), "syntax is ok") || !strings.Contains(string(out), "test is successful") {
		t.Fatalf("expected successful nginx -t output, got: %s", string(out))
	}
}

func getRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	// If running inside tests/api, ascend to repo root
	for !strings.HasSuffix(dir, "code_executor") && dir != "/" {
		dir = filepath.Dir(dir)
	}
	return dir
}

// 2. HEALTH ENDPOINTS PROXIED VIA NGINX
func TestNginx_HealthEndpoints(t *testing.T) {
	harness := setupNginxHarness(t)

	for _, path := range []string{"/health", "/healthz"} {
		resp, err := http.Get(harness.nginxURL + path)
		if err != nil {
			t.Fatalf("failed to get %s: %v", path, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", path, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if strings.TrimSpace(string(body)) != "ok" {
			t.Errorf("expected 'ok' for %s, got %q", path, string(body))
		}
	}
}

// 3. API WORKFLOW THROUGH NGINX (Register -> Login -> POST -> GET -> Ownership)
func TestNginx_APIWorkflow_AuthAndSubmissions(t *testing.T) {
	harness := setupNginxHarness(t)

	// User registration through Nginx
	userAEmail := fmt.Sprintf("nginx_user_a_%d@example.com", time.Now().UnixNano())
	regPayload, _ := json.Marshal(map[string]string{
		"email":    userAEmail,
		"password": "Password123!",
	})
	regResp, err := http.Post(harness.nginxURL+"/api/auth/register", "application/json", bytes.NewReader(regPayload))
	if err != nil {
		t.Fatalf("registration failed through Nginx: %v", err)
	}
	defer regResp.Body.Close()
	if regResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created from Nginx, got %d", regResp.StatusCode)
	}

	var regData struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	_ = json.NewDecoder(regResp.Body).Decode(&regData)
	tokenA := regData.Token

	// Verify /api/auth/me through Nginx
	meReq, _ := http.NewRequest(http.MethodGet, harness.nginxURL+"/api/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+tokenA)
	meResp, err := http.DefaultClient.Do(meReq)
	if err != nil || meResp.StatusCode != http.StatusOK {
		t.Fatalf("failed /api/auth/me through Nginx: %v, status: %d", err, meResp.StatusCode)
	}
	_ = meResp.Body.Close()

	// User B registers
	userBEmail := fmt.Sprintf("nginx_user_b_%d@example.com", time.Now().UnixNano())
	regBPayload, _ := json.Marshal(map[string]string{
		"email":    userBEmail,
		"password": "Password123!",
	})
	regBResp, _ := http.Post(harness.nginxURL+"/api/auth/register", "application/json", bytes.NewReader(regBPayload))
	var regBData struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(regBResp.Body).Decode(&regBData)
	_ = regBResp.Body.Close()
	tokenB := regBData.Token

	// POST /api/submissions through Nginx
	subPayload, _ := json.Marshal(map[string]string{
		"language":    "python",
		"source_code": "print('hello from nginx proxy')",
	})
	postReq, _ := http.NewRequest(http.MethodPost, harness.nginxURL+"/api/submissions", bytes.NewReader(subPayload))
	postReq.Header.Set("Authorization", "Bearer "+tokenA)
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil || postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("failed POST /api/submissions via Nginx: status=%d, err=%v", postResp.StatusCode, err)
	}
	defer postResp.Body.Close()

	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = json.NewDecoder(postResp.Body).Decode(&created)
	if created.ID == "" || created.Status != "QUEUED" {
		t.Fatalf("unexpected submission creation response: %+v", created)
	}

	// GET /api/submissions/:id through Nginx
	getReq, _ := http.NewRequest(http.MethodGet, harness.nginxURL+"/api/submissions/"+created.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+tokenA)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil || getResp.StatusCode != http.StatusOK {
		t.Fatalf("failed GET /api/submissions/:id via Nginx: status=%d, err=%v", getResp.StatusCode, err)
	}
	_ = getResp.Body.Close()

	// Ownership isolation through Nginx: User B gets 404
	getReqB, _ := http.NewRequest(http.MethodGet, harness.nginxURL+"/api/submissions/"+created.ID, nil)
	getReqB.Header.Set("Authorization", "Bearer "+tokenB)
	getRespB, _ := http.DefaultClient.Do(getReqB)
	defer getRespB.Body.Close()
	if getRespB.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for User B accessing User A submission through Nginx, got %d", getRespB.StatusCode)
	}
}

// 4. INCREMENTAL SSE STREAMING THROUGH NGINX (Validating proxy_buffering off)
// Specifically verifies that intermediate PROCESSING event arrives before terminal SUCCESS.
func TestNginx_SSE_IncrementalStreamingAndUnbuffered(t *testing.T) {
	harness := setupNginxHarness(t)

	// Register user
	userEmail := fmt.Sprintf("nginx_sse_user_%d@example.com", time.Now().UnixNano())
	regResp, _ := harness.authSvc.Register(context.Background(), auth.RegisterRequest{
		Email:    userEmail,
		Password: "Password123!",
	})
	token := regResp.Token
	userID := regResp.User.ID

	// Create submission
	created, err := harness.subRepo.Create(context.Background(), &submission.SubmissionEntity{
		UserID:   userID,
		Language: "python",
		Code:     "print('stream test')",
	})
	if err != nil {
		t.Fatalf("failed to create submission: %v", err)
	}

	// Connect SSE client through Nginx
	sseReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/%s/stream", harness.nginxURL, created.ID), nil)
	sseReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("SSE connection through Nginx failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected Content-Type text/event-stream, got %q", ct)
	}

	reader := bufio.NewReader(resp.Body)

	// Step 1: Read initial event (QUEUED)
	initEvent := readOneSSEEvent(t, reader, 3*time.Second)
	if initEvent["status"] != "QUEUED" {
		t.Fatalf("expected initial event QUEUED, got %v", initEvent["status"])
	}

	// Step 2: Simulate worker transition to PROCESSING in DB and emit Pub/Sub event
	_, _ = harness.db.Exec(`UPDATE submissions SET status = 'PROCESSING', updated_at = NOW() WHERE id = $1`, created.ID)
	_ = harness.publisher.PublishStatusEvent(context.Background(), created.ID, "PROCESSING")

	// Step 3: Crucial Unbuffered Check:
	// Verify that client receives PROCESSING event *immediately* while the connection remains open!
	procEvent := readOneSSEEvent(t, reader, 3*time.Second)
	if procEvent["status"] != "PROCESSING" {
		t.Fatalf("expected incremental PROCESSING event via unbuffered Nginx proxy, got %v", procEvent["status"])
	}

	// Step 4: Simulate worker transition to terminal SUCCESS in DB and emit Pub/Sub event
	_, _ = harness.db.Exec(`
		UPDATE submissions
		SET status = 'SUCCESS', stdout = $2, exit_code = 0, execution_time_ms = 42, updated_at = NOW()
		WHERE id = $1
	`, created.ID, "stream output\n")
	_ = harness.publisher.PublishStatusEvent(context.Background(), created.ID, "SUCCESS")

	// Step 5: Verify terminal SUCCESS event arrives and stream closes
	termEvent := readOneSSEEvent(t, reader, 3*time.Second)
	if termEvent["status"] != "SUCCESS" {
		t.Fatalf("expected terminal event SUCCESS, got %v", termEvent["status"])
	}
	if stdout, _ := termEvent["stdout"].(string); strings.TrimSpace(stdout) != "stream output" {
		t.Errorf("expected stdout 'stream output', got %q", stdout)
	}

	// Verify stream is closed by server following terminal event
	doneCh := make(chan error, 1)
	go func() {
		_, err := reader.ReadString('\n')
		doneCh <- err
	}()

	select {
	case err := <-doneCh:
		if err != io.EOF && !strings.Contains(fmt.Sprintf("%v", err), "closed") {
			t.Logf("stream finished with: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected SSE stream to close promptly after terminal event")
	}
}

// 5. ALREADY-TERMINAL SSE FALLBACK THROUGH NGINX
func TestNginx_SSE_AlreadyTerminalImmediateFallback(t *testing.T) {
	harness := setupNginxHarness(t)

	userEmail := fmt.Sprintf("nginx_late_user_%d@example.com", time.Now().UnixNano())
	regResp, _ := harness.authSvc.Register(context.Background(), auth.RegisterRequest{
		Email:    userEmail,
		Password: "Password123!",
	})
	token := regResp.Token
	userID := regResp.User.ID

	// Insert already terminal submission directly in DB
	exitCode := 0
	execTime := 35
	memKB := 8192
	var subID string
	err := harness.db.QueryRow(`
		INSERT INTO submissions (
			user_id, language, code, status, stdout, stderr, exit_code, execution_time_ms, memory_usage_kb, created_at, updated_at
		) VALUES (
			$1, 'python', 'print("late")', 'SUCCESS', $2, '', $3, $4, $5, NOW(), NOW()
		) RETURNING id;
	`, userID, "late output\n", exitCode, execTime, memKB).Scan(&subID)
	if err != nil {
		t.Fatalf("failed to insert terminal submission: %v", err)
	}

	// Connect late SSE client through Nginx
	start := time.Now()
	sseReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/%s/stream", harness.nginxURL, subID), nil)
	sseReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("SSE connection through Nginx failed: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	events, err := parseSSEEvents(reader)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("failed to parse SSE events: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("expected already-terminal stream to terminate almost immediately through Nginx, took %v", elapsed)
	}

	if len(events) != 1 {
		t.Fatalf("expected exactly 1 terminal event, got %d: %+v", len(events), events)
	}
	if events[0]["status"] != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %v", events[0]["status"])
	}
	if stdout, _ := events[0]["stdout"].(string); strings.TrimSpace(stdout) != "late output" {
		t.Errorf("expected stdout 'late output', got %q", stdout)
	}
}

// 6. SECURITY BOUNDARY: NGINX MUST NOT EXPOSE INTERNAL SERVICES
func TestNginx_Security_InternalServicesNotExposed(t *testing.T) {
	harness := setupNginxHarness(t)

	// Attempt to request arbitrary paths
	for _, unmappedPath := range []string{"/postgres", "/redis", "/docker.sock", "/metrics", "/admin", "/"} {
		resp, err := http.Get(harness.nginxURL + unmappedPath)
		if err != nil {
			t.Fatalf("request to %s failed: %v", unmappedPath, err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for unmapped internal path %s, got %d", unmappedPath, resp.StatusCode)
		}
	}
}

func readOneSSEEvent(t *testing.T, reader *bufio.Reader, timeout time.Duration) map[string]any {
	t.Helper()
	type res struct {
		evt map[string]any
		err error
	}
	ch := make(chan res, 1)

	go func() {
		var currentData string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				ch <- res{err: err}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(line, "data: ") {
				currentData = strings.TrimPrefix(line, "data: ")
			} else if line == "" && currentData != "" {
				var parsed map[string]any
				if err := json.Unmarshal([]byte(currentData), &parsed); err == nil {
					ch <- res{evt: parsed}
					return
				}
				currentData = ""
			}
		}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("error reading SSE event: %v", r.err)
		}
		return r.evt
	case <-time.After(timeout):
		t.Fatalf("timed out after %v waiting for SSE event", timeout)
		return nil
	}
}
