// Command api serves the fitness backend: workout plan generation and logged
// workout tracking. It does only wiring — build dependencies, register routes,
// start the server, shut down gracefully. All behavior lives in internal/.
//
// Storage is selected at startup by DATABASE_URL:
//   - unset: in-memory repositories, suitable for `go run` and demos.
//   - set:   one shared Postgres pool, with migrations applied at boot.
//
// Either way the rest of the program is identical because both repositories
// satisfy their feature's interface.
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
	library := exercise.Library()

	// Build storage once, share across features. A single pgxpool serves both
	// the plan and workout repositories — connection pooling exists precisely
	// to multiplex concurrent users across a bounded resource, so opening two
	// pools against the same database would double connection usage for no
	// benefit. closer tears the pool down on shutdown.
	planRepo, workoutRepo, closer, err := buildStorage(context.Background(), logger, dsn)
	if err != nil {
		logger.Error("storage init failed", "error", err)
		os.Exit(1)
	}
	defer closer()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Plan generation + retrieval. The Service wraps the engine with
	// persistence so generated plans are durable.
	plan.NewHandler(plan.NewService(library, planRepo, nil, nil)).Routes(mux)

	// Logged workout tracking.
	workout.NewHandler(workout.NewService(workoutRepo, nil, nil)).Routes(mux)

	// Middleware order is load-bearing:
	//   - RequestID is OUTERMOST so the ID it puts into the request context is
	//     visible to every layer that wraps the handler (Logger and Recover
	//     both read request_id off r.Context() on the response path).
	//   - Logger sits in the middle so its one-line-per-request log fires
	//     whether the handler returned normally or Recover turned a panic into
	//     a 500 — its statusRecorder captures whatever status was finally
	//     written.
	//   - Recover is INNERMOST around the handler so a panic in the handler
	//     is caught synchronously and converted to a 500 with the standard
	//     envelope. Logger then logs that 500 as a normal completed request.
	root := httpx.Chain(mux,
		httpx.RequestID,
		httpx.Logger(logger),
		httpx.Recover(logger),
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

// buildStorage returns the plan + workout repositories backed either by
// in-memory implementations (when dsn is empty) or by a single shared Postgres
// pool. The closer tears down the pool on shutdown; in in-memory mode it is a
// no-op. Returning a single closer keeps `main` insulated from which storage
// mode was selected.
func buildStorage(ctx context.Context, logger *slog.Logger, dsn string) (plan.Repository, workout.Repository, func(), error) {
	if dsn == "" {
		logger.Info("storage", "mode", "in-memory")
		return plan.NewInMemoryRepository(),
			workout.NewInMemoryRepository(),
			func() {}, nil
	}

	logger.Info("storage", "mode", "postgres")
	if err := db.Migrate(dsn); err != nil {
		return nil, nil, nil, err
	}
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		return nil, nil, nil, err
	}
	return plan.NewPostgresRepository(pool),
		workout.NewPostgresRepository(pool),
		pool.Close, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
