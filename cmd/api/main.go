// Command api serves the fitness backend: workout plan generation and logged
// workout tracking. It does only wiring — build dependencies, register routes,
// start the server, shut down gracefully. All behavior lives in internal/.
//
// Storage is selected at startup by DATABASE_URL:
//   - unset: in-memory repository, suitable for `go run` and demos.
//   - set:   Postgres pool, with migrations applied at boot.
//
// Either way the rest of the program is identical because both repositories
// satisfy the same workout.Repository interface.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/db"
	"github.com/ataliaferro46/go-workout-api/internal/exercise"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
	"github.com/ataliaferro46/go-workout-api/internal/plan"
	"github.com/ataliaferro46/go-workout-api/internal/workout"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	addr := getenv("ADDR", ":8080")
	dsn := os.Getenv("DATABASE_URL")

	// Build the workout repository before the HTTP server starts so a
	// misconfigured DSN fails fast instead of surfacing as a 500 on the first
	// request. The pool's lifetime is bound to a closer registered below.
	repo, repoClose, err := buildWorkoutRepo(context.Background(), logger, dsn)
	if err != nil {
		logger.Error("storage init failed", "error", err)
		os.Exit(1)
	}
	defer repoClose()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Plan generation. The handler is stateless over the immutable library; to
	// persist generated plans, add a repository behind it (see README).
	plan.NewHandler(exercise.Library()).Routes(mux)

	// Logged workout tracking. Repo is in-memory or Postgres depending on
	// DATABASE_URL; nothing else in this file changes.
	workout.NewHandler(workout.NewService(repo, nil, nil)).Routes(mux)

	root := httpx.Chain(mux,
		httpx.Recover(logger),
		httpx.Logger(logger),
		httpx.RequestID,
	)

	srv := &http.Server{
		Addr:         addr,
		Handler:      root,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", addr)
		serverErr <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	case sig := <-stop:
		logger.Info("shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("server stopped cleanly")
	}
}

// buildWorkoutRepo returns a Repository plus a closer the caller must defer.
// The closer is a no-op for in-memory mode; it tears down the pgxpool for
// Postgres mode. Returning a closer rather than the pool directly keeps the
// caller insulated from which storage was selected.
func buildWorkoutRepo(ctx context.Context, logger *slog.Logger, dsn string) (workout.Repository, func(), error) {
	if dsn == "" {
		logger.Info("storage", "mode", "in-memory")
		return workout.NewInMemoryRepository(), func() {}, nil
	}

	logger.Info("storage", "mode", "postgres")
	if err := db.Migrate(dsn); err != nil {
		return nil, nil, err
	}
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		return nil, nil, err
	}
	return workout.NewPostgresRepository(pool), pool.Close, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
