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
	_ "github.com/lib/pq"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	JWTExpiry   time.Duration
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

	return Config{
		Port:        port,
		DatabaseURL: dbURL,
		JWTSecret:   secret,
		JWTExpiry:   time.Duration(expiryHours) * time.Hour,
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

	// 3. Initialize components
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

	// 4. Setup ServeMux routing
	mux := http.NewServeMux()

	// Public auth routes
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)

	// Protected auth routes
	mux.Handle("/api/auth/me", authMiddleware.RequireAuth(http.HandlerFunc(authHandler.Me)))

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
