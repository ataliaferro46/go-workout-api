// Command api is the HTTP entrypoint. It does only wiring: construct
// dependencies, register routes, start the server, and shut down gracefully.
// All behavior lives in the internal packages.
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

	"github.com/ataliaferro46/go-workout-api/internal/httpx"
	"github.com/ataliaferro46/go-workout-api/internal/workout"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	addr := getenv("ADDR", ":8080")

	// Dependency wiring. To persist data, replace NewInMemoryRepository with
	// a type that implements workout.Repository against Postgres (see README)
	// — nothing else in the program needs to change.
	repo := workout.NewInMemoryRepository()
	svc := workout.NewService(repo, nil, nil)
	handler := workout.NewHandler(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	handler.Routes(mux)

	// Middleware order (outermost first): recover wraps everything so a panic
	// in any layer is caught; logging records the final status; request ID is
	// available to both.
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

	// Start the server in a goroutine so main can block on either a fatal
	// server error or an OS shutdown signal.
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

// getenv returns the value of key, or fallback if key is unset or empty.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
