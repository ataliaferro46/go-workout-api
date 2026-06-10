// Command api serves the fitness backend: workout plan generation, logged
// workout tracking, exercise library, biometric integrations, and real
// email/password authentication. It does only wiring — build dependencies,
// register routes, start the server, shut down gracefully.
//
// Storage selected by DATABASE_URL: unset = in-memory (good for `go run`
// and demos), set = one shared Postgres pool.
//
// Auth is mandatory for the app surface and for /v1/{plans,workouts,
// biometrics}/* — every request without a valid session cookie returns 401.
// /v1/auth/*, /v1/exercises/* (the library is public reference data),
// /healthz, the OAuth callback, and the webhook endpoints are intentionally
// unauthenticated.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/auth"
	"github.com/ataliaferro46/go-workout-api/internal/biometrics"
	"github.com/ataliaferro46/go-workout-api/internal/db"
	"github.com/ataliaferro46/go-workout-api/internal/exercise"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
	"github.com/ataliaferro46/go-workout-api/internal/nutrition"
	"github.com/ataliaferro46/go-workout-api/internal/plan"
	"github.com/ataliaferro46/go-workout-api/internal/web"
	"github.com/ataliaferro46/go-workout-api/internal/workout"
	"github.com/jackc/pgx/v5/pgxpool"
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

	// Auth wiring. Public URL base is whatever the user hits; needed for
	// constructing verification links in emails.
	publicBase := getenv("PUBLIC_URL", "http://localhost:8080")
	authSvc := auth.NewService(
		deps.authUsers, deps.authSessions, deps.authVerifications,
		buildEmailer(logger),
		auth.Config{VerifyURLBase: publicBase},
	)
	// Cookie Secure flag — true on HTTPS deploys (production), false locally.
	cookieSecure := strings.HasPrefix(publicBase, "https://")
	authHandler := auth.NewHandler(authSvc, cookieSecure)
	requireAuth := auth.RequireAuth(authSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Auth surface — always public.
	authHandler.Routes(mux)

	// Exercise library — public read endpoints (library is reference data).
	// Admin write endpoints stay gated by ADMIN_API_KEY.
	exercise.NewHandler(deps.exerciseSvc).Routes(mux, httpx.AdminAuth(adminKey))

	// Biometrics — auth-required except for callback + webhook.
	if deps.biometricsSvc != nil {
		biometrics.NewHandler(deps.biometricsSvc).Routes(mux, requireAuth)
	}

	// Recovery adapter is wired regardless of whether biometrics is enabled.
	var recoverySource plan.RecoverySource
	if deps.biometricsSvc != nil {
		recoverySource = biometrics.NewPlanRecoveryAdapter(deps.biometricsSvc)
	}

	// Plan generation + retrieval — auth-required.
	plan.NewHandler(plan.NewService(
		deps.exerciseSvc.Snapshot, deps.planRepo, recoverySource, nil, nil,
	)).Routes(mux, requireAuth)

	// Logged workout tracking — auth-required. The intensity adapter lets
	// the workout handler ask the biometrics service for an HR summary
	// without importing the biometrics package (returns `any` to keep the
	// boundary clean).
	workoutHandler := workout.NewHandler(workout.NewService(deps.workoutRepo, nil, nil))
	if deps.biometricsSvc != nil {
		workoutHandler.SetIntensityFetcher(intensityAdapter{svc: deps.biometricsSvc})
	}
	// Fetch user weight from auth profile for cardio calorie estimation.
	authUsers := deps.authUsers
	workoutHandler.SetUserWeightFetcher(func(ctx context.Context, userID string) (float64, error) {
		c, err := authUsers.GetByID(ctx, userID)
		if err != nil {
			return 0, err
		}
		if c.User.WeightKG == nil {
			return 0, nil
		}
		return *c.User.WeightKG, nil
	})
	workoutHandler.Routes(mux, requireAuth)

	// Nutrition — only wired when Postgres is available, since it depends
	// on shared SQL queries (joining foods/logs/cardio_sessions).
	if pool := getPool(deps); pool != nil {
		nutritionRepo := nutrition.NewRepository(pool)
		nutritionHandler := nutrition.NewHandler(nutritionRepo, func(ctx context.Context, id string) (auth.User, error) {
			c, err := authUsers.GetByID(ctx, id)
			if err != nil {
				return auth.User{}, err
			}
			return c.User, nil
		})
		nutritionHandler.Routes(mux, requireAuth)
	}

	// Single-page UI. Pages handle their own auth check client-side and
	// redirect to /login on 401. Embedded files (HTML, CSS, JS) are served
	// here regardless of auth so the login page itself can load.
	mux.Handle("/", web.Handler())

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

	daemonCtx, daemonCancel := context.WithCancel(context.Background())
	defer daemonCancel()
	if deps.biometricsDaemon != nil {
		go deps.biometricsDaemon.Run(daemonCtx)
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", addr, "public_url", publicBase)
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
	planRepo          plan.Repository
	workoutRepo       workout.Repository
	exerciseSvc       *exercise.Service
	biometricsSvc     *biometrics.Service
	biometricsDaemon  *biometrics.Daemon
	authUsers         auth.UserRepository
	authSessions      auth.SessionRepository
	authVerifications auth.VerificationRepository
	pgPool            *pgxpool.Pool // nil in in-memory mode
}

func getPool(d deps) *pgxpool.Pool { return d.pgPool }

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
		planRepo:          plan.NewInMemoryRepository(),
		workoutRepo:       workout.NewInMemoryRepository(),
		exerciseSvc:       exerciseSvc,
		biometricsSvc:     bioSvc,
		biometricsDaemon:  bioDaemon,
		authUsers:         auth.NewInMemoryUserRepository(),
		authSessions:      auth.NewInMemorySessionRepository(),
		authVerifications: auth.NewInMemoryVerificationRepository(),
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
		planRepo:          plan.NewPostgresRepository(pool),
		workoutRepo:       workout.NewPostgresRepository(pool),
		exerciseSvc:       exerciseSvc,
		biometricsSvc:     bioSvc,
		biometricsDaemon:  bioDaemon,
		authUsers:         auth.NewPostgresUserRepository(pool),
		authSessions:      auth.NewPostgresSessionRepository(pool),
		authVerifications: auth.NewPostgresVerificationRepository(pool),
		pgPool:            pool,
	}, pool.Close, nil
}

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
	}
	if id := os.Getenv("OURA_CLIENT_ID"); id != "" {
		registry.Register(biometrics.NewOuraProvider(biometrics.OuraConfig{
			ClientID:      id,
			ClientSecret:  os.Getenv("OURA_CLIENT_SECRET"),
			RedirectURI:   os.Getenv("OURA_REDIRECT_URI"),
			WebhookSecret: os.Getenv("OURA_WEBHOOK_SECRET"),
		}))
	}

	svc := biometrics.NewService(registry, tokenStore, readingRepo, syncRepo, nil, nil, logger)
	daemon := biometrics.NewDaemon(svc, tokenRepo, tokenStore, biometrics.DaemonConfig{}, logger)
	logger.Info("biometrics enabled", "providers", registry.Names())
	return svc, daemon, nil
}

// buildEmailer chooses ResendEmailer when RESEND_API_KEY is set and falls
// back to ConsoleEmailer otherwise (logs the verification URL so devs can
// still complete the flow locally).
func buildEmailer(logger *slog.Logger) auth.Emailer {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		logger.Info("emailer", "mode", "console (RESEND_API_KEY not set)")
		return auth.ConsoleEmailer{Logger: logger}
	}
	from := getenv("RESEND_FROM", "noreply@workout-api.dev")
	logger.Info("emailer", "mode", "resend", "from", from)
	return auth.ResendEmailer{APIKey: apiKey, From: from}
}

// intensityAdapter wraps biometrics.Service to satisfy the workout
// package's IntensityFetcher interface without creating a package import
// from workout → biometrics.
type intensityAdapter struct{ svc *biometrics.Service }

func (a intensityAdapter) IntensityForWindow(ctx context.Context, userID string, start, end time.Time) (any, error) {
	return a.svc.IntensityForWindow(ctx, userID, start, end)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
