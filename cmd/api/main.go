package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
	"github.com/SatyamSingh-16/code_executor/internal/database"
	"github.com/SatyamSingh-16/code_executor/internal/queue"
	"github.com/SatyamSingh-16/code_executor/internal/ratelimit"
	"github.com/SatyamSingh-16/code_executor/internal/submission"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	JWTExpiry   time.Duration
	RateLimit   int
	RateWindow  time.Duration
}

func loadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://satyamsingh2730@localhost:5432/code_execution_test_db?sslmode=disable"
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "default-dev-secret-do-not-use-in-production-change-me"
	}

	expiryHours := 24
	if hStr := os.Getenv("JWT_EXPIRY_HOURS"); hStr != "" {
		if parsed, err := strconv.Atoi(hStr); err == nil && parsed > 0 {
			expiryHours = parsed
		}
	}

	rateLimit := ratelimit.DefaultRateLimit
	if rlStr := os.Getenv("RATE_LIMIT"); rlStr != "" {
		if parsed, err := strconv.Atoi(rlStr); err == nil && parsed > 0 {
			rateLimit = parsed
		}
	}

	rateWindow := ratelimit.DefaultWindow
	if rwStr := os.Getenv("RATE_WINDOW"); rwStr != "" {
		if d, err := time.ParseDuration(rwStr); err == nil && d > 0 {
			rateWindow = d
		}
	} else if rwSec := os.Getenv("RATE_WINDOW_SECONDS"); rwSec != "" {
		if s, err := strconv.Atoi(rwSec); err == nil && s > 0 {
			rateWindow = time.Duration(s) * time.Second
		}
	}

	return Config{
		Port:        port,
		DatabaseURL: dbURL,
		JWTSecret:   secret,
		JWTExpiry:   time.Duration(expiryHours) * time.Hour,
		RateLimit:   rateLimit,
		RateWindow:  rateWindow,
	}
}

func main() {
	log.Println("[api] starting Distributed Code Execution Platform API server...")

	cfg := loadConfig()

	// 1. Connect to PostgreSQL
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[api] failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("[api] database ping failed: %v", err)
	}

	// 2. Ensure migrations are applied
	migrator := database.NewMigrator(db)
	if err := migrator.Up(context.Background()); err != nil {
		log.Fatalf("[api] failed to run database migrations: %v", err)
	}

	// 3. Connect to Redis
	redisCfg := queue.LoadRedisConfigFromEnv()
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisCfg.Addr,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
	})
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("[api] redis ping failed: %v", err)
	}

	// 4. Initialize components
	userRepo := auth.NewPostgresUserRepository(db)
	tokenMgr, err := auth.NewJWTTokenManager(auth.JWTConfig{
		Secret:     cfg.JWTSecret,
		Expiration: cfg.JWTExpiry,
	})
	if err != nil {
		log.Fatalf("[api] failed to initialize JWT token manager: %v", err)
	}

	authService := auth.NewService(userRepo, tokenMgr)
	authHandler := auth.NewHandler(authService)
	authMiddleware := auth.NewMiddleware(tokenMgr)

	limiter := ratelimit.NewRedisSlidingWindowLimiter(rdb, ratelimit.Config{
		Limit:  cfg.RateLimit,
		Window: cfg.RateWindow,
	})
	rateLimitMiddleware := ratelimit.NewMiddleware(limiter)

	// Submission components
	streamQueue := queue.NewRedisQueue(rdb, queue.DefaultStreamName, queue.DefaultConsumerGroup)
	submissionRepo := submission.NewPostgresSubmissionRepository(db)
	submissionSvc := submission.NewService(submissionRepo, streamQueue)
	submissionHandler := submission.NewHandler(submissionSvc)

	// 5. Setup ServeMux routing
	mux := http.NewServeMux()

	// Public auth routes
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)

	// Protected auth routes
	mux.Handle("/api/auth/me", authMiddleware.RequireAuth(http.HandlerFunc(authHandler.Me)))

	// Submission routes
	// POST /api/submissions: Requires JWT Auth + Rate Limiting
	mux.Handle("POST /api/submissions", authMiddleware.RequireAuth(rateLimitMiddleware.RequireRateLimit(http.HandlerFunc(submissionHandler.Create))))
	// GET /api/submissions: User submission history (Auth required, no submission quota limit)
	mux.Handle("GET /api/submissions", authMiddleware.RequireAuth(http.HandlerFunc(submissionHandler.List)))
	// GET /api/submissions/{id}: User submission detail (Auth required, ownership enforced)
	mux.Handle("GET /api/submissions/{id}", authMiddleware.RequireAuth(http.HandlerFunc(submissionHandler.Get)))
	// Submissions path fallback for non-pattern-matching routers
	mux.Handle("/api/submissions/", authMiddleware.RequireAuth(http.HandlerFunc(submissionHandler.RouteSubmissions)))
	mux.Handle("/api/submissions", authMiddleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			rateLimitMiddleware.RequireRateLimit(http.HandlerFunc(submissionHandler.Create)).ServeHTTP(w, r)
		} else {
			submissionHandler.RouteSubmissions(w, r)
		}
	})))

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 5. Graceful shutdown handler
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("[api] HTTP server listening on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("HTTP listen error: %w", err)
		}
	}()

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		log.Fatalf("[api] fatal server error: %v", err)
	case sig := <-shutdownCh:
		log.Printf("[api] received shutdown signal %v, shutting down HTTP server gracefully...", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			log.Fatalf("[api] could not stop HTTP server gracefully: %v", err)
		}
	}

	log.Println("[api] server stopped cleanly")
}
