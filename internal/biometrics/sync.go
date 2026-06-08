package biometrics

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Daemon polls every connected (user, provider) pair on a fixed interval,
// fetching new readings since the last sync watermark and ingesting them.
// Started from main.go as `go d.Run(ctx)`.
//
// The bounded-concurrency semaphore prevents a thousand connected users
// from hitting Whoop simultaneously every interval — a real concern at
// scale that backend interviewers like to probe.
type Daemon struct {
	svc        *Service
	tokens     TokenRepository // direct access so we can list (user, provider) pairs
	tokenStore *TokenStore     // for decryption inside the daemon
	interval   time.Duration
	parallel   int
	logger     *slog.Logger
}

// DaemonConfig captures the tunable knobs.
type DaemonConfig struct {
	Interval time.Duration // default 15m
	Parallel int           // default 8
}

// NewDaemon returns a polling daemon. svc.tokens is the encrypted store;
// tokens (the underlying repo) is used here only to enumerate (user,
// provider) pairs — production reads through svc.tokens to get plaintext.
func NewDaemon(svc *Service, tokenRepo TokenRepository, tokenStore *TokenStore, cfg DaemonConfig, logger *slog.Logger) *Daemon {
	if cfg.Interval == 0 {
		cfg.Interval = 15 * time.Minute
	}
	if cfg.Parallel == 0 {
		cfg.Parallel = 8
	}
	return &Daemon{
		svc:        svc,
		tokens:     tokenRepo,
		tokenStore: tokenStore,
		interval:   cfg.Interval,
		parallel:   cfg.Parallel,
		logger:     logger,
	}
}

// Run blocks until ctx is cancelled, ticking syncAll at d.interval.
func (d *Daemon) Run(ctx context.Context) {
	t := time.NewTicker(d.interval)
	defer t.Stop()
	d.logger.Info("biometrics daemon started", "interval", d.interval, "parallel", d.parallel)
	for {
		select {
		case <-ctx.Done():
			d.logger.Info("biometrics daemon stopped")
			return
		case <-t.C:
			d.syncAll(ctx)
		}
	}
}

// pairLister is the TokenRepository ability to enumerate (user, provider)
// pairs. We add this only on the implementations the daemon actually uses;
// production wires PostgresTokenRepository which satisfies it.
type pairLister interface {
	ListPairs(ctx context.Context) ([]TokenPair, error)
}

// TokenPair is a (userID, provider) reference returned by ListPairs.
type TokenPair struct {
	UserID, Provider string
}

func (d *Daemon) syncAll(ctx context.Context) {
	lister, ok := d.tokens.(pairLister)
	if !ok {
		// In-memory repo (used in dev mode without real providers) doesn't
		// support pair listing; nothing to sync.
		return
	}
	pairs, err := lister.ListPairs(ctx)
	if err != nil {
		d.logger.Error("biometrics daemon list pairs", "error", err)
		return
	}
	sem := make(chan struct{}, d.parallel)
	var wg sync.WaitGroup
	for _, pair := range pairs {
		wg.Add(1)
		sem <- struct{}{}
		go func(p TokenPair) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := d.syncOne(ctx, p); err != nil {
				d.logger.Warn("biometrics sync failed",
					"user_id", p.UserID, "provider", p.Provider, "error", err)
			}
		}(pair)
	}
	wg.Wait()
}

func (d *Daemon) syncOne(ctx context.Context, pair TokenPair) error {
	provider, err := d.svc.registry.Lookup(pair.Provider)
	if err != nil {
		return err
	}
	tok, err := d.tokenStore.Load(ctx, pair.UserID, pair.Provider)
	if err != nil {
		return err
	}

	state, err := d.svc.syncState.Get(ctx, pair.UserID, pair.Provider)
	since := time.Now().Add(-24 * time.Hour) // bootstrap to last 24h
	if err == nil {
		since = state.LastSyncedAt
	} else if !errors.Is(err, ErrSyncStateNotFound) {
		return err
	}

	readings, err := provider.LatestSince(ctx, tok.AccessToken, since)
	if err != nil {
		return err
	}
	if _, err := d.svc.IngestReadings(ctx, pair.UserID, readings); err != nil {
		return err
	}
	return d.svc.syncState.Set(ctx, SyncState{
		UserID:       pair.UserID,
		Provider:     pair.Provider,
		LastSyncedAt: d.svc.now(),
	})
}
