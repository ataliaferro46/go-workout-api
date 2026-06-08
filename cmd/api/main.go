// Command api serves the fitness backend: workout plan generation and
// logged workout tracking, plus admin-edited exercise library and (when
// configured) biometric integrations that bias plan generation by recovery.
// It does only wiring — build dependencies, register routes, start the
// server, shut down gracefully. All behavior lives in internal/.
//
// Storage is selected at startup by DATABASE_URL:
//   - unset: in-memory repositories, suitable for `go run` and demos.
//   - set:   one shared Postgres pool, with migrations applied at boot.
//
// Either way the rest of the program is identical because every feature's
// repository satisfies a shared interface.
//
// Biometrics integrations are off by default. They activate when
// BIOMETRICS_MASTER_KEY is set (≥32 bytes) — without it, OAuth tokens have
// nowhere safe to live and the routes refuse to serve. Provider credentials
// (WHOOP_CLIENT_ID etc.) determine which real providers register; the Mock
// provider always registers so dev workflows have a working endpoint.
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

	"github.com/ataliaferro46/go-workout-api/internal/biometrics"
	"github.com/ataliaferro46/go-workout-api/internal/db"
	"github.com/ataliaferro46/go-workout-api/internal/exercise"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
	"github.com/ataliaferro46/go-workout-api/internal/plan"
	"github.com/ataliaferro46/go-workout-api/internal/web"
	"github.com/ataliaferro46/go-workout-api/internal/workout"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	addr := getenv("ADDR", ":8080")
	dsn := os.Getenv("DATABASE_URL")
	adminKey := os.Getenv("ADMIN_API_KEY")

	ctx := context.Background()
	deps, closer, err := buildStorage(ctx, logger, dsn)
	if err != nil {
		logger.Error("storage init failed", "error", err)
		os.Exit(1)
	}
	defer closer()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Exercise library — public read endpoints + admin write endpoints
	// gated by ADMIN_API_KEY.
	exercise.NewHandler(deps.exerciseSvc).Routes(mux, httpx.AdminAuth(adminKey))

	// Biometrics — registered only when BIOMETRICS_MASTER_KEY is configured.
	// Recovery is also threaded into plan generation via the adapter.
	var recoverySource plan.RecoverySource
	if deps.biometricsSvc != nil {
		biometrics.NewHandler(deps.biometricsSvc).Routes(mux)
		recoverySource = biometrics.NewPlanRecoveryAdapter(deps.biometricsSvc)
	}

	// Plan generation + retrieval.
	plan.NewHandler(plan.NewService(
		deps.exerciseSvc.Snapshot, deps.planRepo, recoverySource, nil, nil,
	)).Routes(mux)

	// Logged workout tracking.
	workout.NewHandler(workout.NewService(deps.workoutRepo, nil, nil)).Routes(mux)

	// Single-page UI at /. The embedded handler also serves /favicon.ico,
	// /robots.txt, and any future static files placed under internal/web/static.
	// Registered last so any specific route declared above wins via ServeMux's
	// longest-prefix-match precedence.
	mux.Handle("/", web.Handler())

	// Middleware order is load-bearing (see ADR-030).
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

	// Start the polling daemon if biometrics is enabled. The daemon's
	// context is cancelled on shutdown so it tears down with the server.
	daemonCtx, daemonCancel := context.WithCancel(context.Background())
	defer daemonCancel()
	if deps.biometricsDaemon != nil {
		go deps.biometricsDaemon.Run(daemonCtx)
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
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("server stopped cleanly")
	}
}

type deps struct {
	planRepo         plan.Repository
	workoutRepo      workout.Repository
	exerciseSvc      *exercise.Service
	biometricsSvc    *biometrics.Service
	biometricsDaemon *biometrics.Daemon
}

// buildStorage assembles every feature's dependencies and returns them in a
// single struct. The closer tears down the shared Postgres pool on shutdown
// (no-op in in-memory mode).
func buildStorage(ctx context.Context, logger *slog.Logger, dsn string) (deps, func(), error) {
	if dsn == "" {
		return buildInMemoryDeps(ctx, logger)
	}
	return buildPostgresDeps(ctx, logger, dsn)
}

func buildInMemoryDeps(ctx context.Context, logger *slog.Logger) (deps, func(), error) {
	logger.Info("storage", "mode", "in-memory")

	exerciseRepo := exercise.NewInMemoryRepository(exercise.Seed())
	exerciseSvc := exercise.NewService(exerciseRepo)
	if err := exerciseSvc.LoadSnapshot(ctx); err != nil {
		return deps{}, nil, err
	}

	tokenRepo := biometrics.NewInMemoryTokenRepository()
	bioSvc, bioDaemon, err := buildBiometrics(ctx, logger,
		tokenRepo,
		biometrics.NewInMemoryReadingRepository(),
		biometrics.NewInMemorySyncStateRepository())
	if err != nil {
		return deps{}, nil, err
	}

	return deps{
		planRepo:         plan.NewInMemoryRepository(),
		workoutRepo:      workout.NewInMemoryRepository(),
		exerciseSvc:      exerciseSvc,
		biometricsSvc:    bioSvc,
		biometricsDaemon: bioDaemon,
	}, func() {}, nil
}

func buildPostgresDeps(ctx context.Context, logger *slog.Logger, dsn string) (deps, func(), error) {
	logger.Info("storage", "mode", "postgres")
	if err := db.Migrate(dsn); err != nil {
		return deps{}, nil, err
	}
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		return deps{}, nil, err
	}

	exerciseRepo := exercise.NewPostgresRepository(pool)
	if inserted, err := exercise.SeedIfEmpty(ctx, exerciseRepo, exercise.Seed()); err != nil {
		pool.Close()
		return deps{}, nil, err
	} else if inserted > 0 {
		logger.Info("seeded exercise library", "rows", inserted)
	}
	exerciseSvc := exercise.NewService(exerciseRepo)
	if err := exerciseSvc.LoadSnapshot(ctx); err != nil {
		pool.Close()
		return deps{}, nil, err
	}

	bioSvc, bioDaemon, err := buildBiometrics(ctx, logger,
		biometrics.NewPostgresTokenRepository(pool),
		biometrics.NewPostgresReadingRepository(pool),
		biometrics.NewPostgresSyncStateRepository(pool))
	if err != nil {
		pool.Close()
		return deps{}, nil, err
	}

	return deps{
		planRepo:         plan.NewPostgresRepository(pool),
		workoutRepo:      workout.NewPostgresRepository(pool),
		exerciseSvc:      exerciseSvc,
		biometricsSvc:    bioSvc,
		biometricsDaemon: bioDaemon,
	}, pool.Close, nil
}

// buildBiometrics constructs the biometrics service and polling daemon, or
// returns (nil, nil, nil) when BIOMETRICS_MASTER_KEY isn't set. Disabled
// biometrics is a valid state — every other feature works fine without it.
func buildBiometrics(
	ctx context.Context,
	logger *slog.Logger,
	tokenRepo biometrics.TokenRepository,
	readingRepo biometrics.ReadingRepository,
	syncRepo biometrics.SyncStateRepository,
) (*biometrics.Service, *biometrics.Daemon, error) {
	_ = ctx
	masterKey := os.Getenv("BIOMETRICS_MASTER_KEY")
	if masterKey == "" {
		logger.Info("biometrics disabled (BIOMETRICS_MASTER_KEY not set)")
		return nil, nil, nil
	}

	tokenStore, err := biometrics.NewTokenStore(tokenRepo, []byte(masterKey))
	if err != nil {
		return nil, nil, err
	}

	registry := biometrics.NewRegistry()
	registry.Register(biometrics.NewMockProvider(getenv("MOCK_WEBHOOK_SECRET", "dev-mock-secret")))
	if id := os.Getenv("WHOOP_CLIENT_ID"); id != "" {
		registry.Register(biometrics.NewWhoopProvider(biometrics.WhoopConfig{
			ClientID:      id,
			ClientSecret:  os.Getenv("WHOOP_CLIENT_SECRET"),
			RedirectURI:   os.Getenv("WHOOP_REDIRECT_URI"),
			WebhookSecret: os.Getenv("WHOOP_WEBHOOK_SECRET"),
		}))
		logger.Info("biometrics provider registered", "provider", "whoop")
	}
	if id := os.Getenv("OURA_CLIENT_ID"); id != "" {
		registry.Register(biometrics.NewOuraProvider(biometrics.OuraConfig{
			ClientID:      id,
			ClientSecret:  os.Getenv("OURA_CLIENT_SECRET"),
			RedirectURI:   os.Getenv("OURA_REDIRECT_URI"),
			WebhookSecret: os.Getenv("OURA_WEBHOOK_SECRET"),
		}))
		logger.Info("biometrics provider registered", "provider", "oura")
	}

	svc := biometrics.NewService(registry, tokenStore, readingRepo, syncRepo, nil, nil, logger)
	daemon := biometrics.NewDaemon(svc, tokenRepo, tokenStore, biometrics.DaemonConfig{}, logger)
	logger.Info("biometrics enabled", "providers", registry.Names())
	return svc, daemon, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
