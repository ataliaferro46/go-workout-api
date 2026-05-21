// Command api serves the fitness backend: workout plan generation and logged
// workout tracking. It does only wiring — build dependencies, register routes,
// start the server, shut down gracefully. All behavior lives in internal/.
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

	"github.com/ataliaferro46/go-workout-api/internal/exercise"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
	"github.com/ataliaferro46/go-workout-api/internal/plan"
	"github.com/ataliaferro46/go-workout-api/internal/workout"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	addr := getenv("ADDR", ":8080")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Plan generation. The handler is stateless over the immutable library; to
	// persist generated plans, add a repository behind it (see README).
	plan.NewHandler(exercise.Library()).Routes(mux)

	// Logged workout tracking. Swap NewInMemoryRepository for a Postgres-backed
	// implementation of workout.Repository to persist sessions.
	workoutSvc := workout.NewService(workout.NewInMemoryRepository(), nil, nil)
	workout.NewHandler(workoutSvc).Routes(mux)

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

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
