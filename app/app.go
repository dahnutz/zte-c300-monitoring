package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	rds "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"zte-c300-monitoring/config"
	"zte-c300-monitoring/internal/handler"
	"zte-c300-monitoring/internal/health"
	"zte-c300-monitoring/internal/poller"
	"zte-c300-monitoring/internal/repository"
	"zte-c300-monitoring/internal/reqctx"
	"zte-c300-monitoring/internal/store"
	"zte-c300-monitoring/pkg/graceful"
	"zte-c300-monitoring/pkg/logger"
	"zte-c300-monitoring/pkg/redis"
)

// App represents the main application structure that holds the HTTP router
// and manages the application lifecycle, including dependency initialization
// and server startup.
type App struct {
	router http.Handler
}

// New creates and returns a new instance of the App with initialized dependencies.
// It prepares the application for startup but does not start the server.
func New() *App {
	return &App{}
}

// Start initializes the application components, sets up connections to external
// services (Redis and SNMP), and starts the HTTP server. It handles graceful
// shutdown on context cancellation and ensures proper cleanup of resources.
func (a *App) Start(ctx context.Context) error {
	// Load configuration from environment variables (no config file needed).
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("failed to load config", zap.Error(err))
		return err
	}

	if cfg.APIKey == "" && len(cfg.APIUsers) == 0 {
		return fmt.Errorf("API_KEY or API_USERS is required")
	}

	// Initialize Redis client.
	redisClient := redis.NewRedisClient(cfg)

	// Check Redis connection.
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error("failed to ping redis server", zap.Error(err))
	} else {
		logger.Info("redis server successfully connected")
	}

	// Close Redis client on shutdown.
	defer func(redisClient *rds.Client) {
		if err := redisClient.Close(); err != nil {
			logger.Error("failed to close redis client", zap.Error(err))
		}
	}(redisClient)

	redisRepo := repository.NewOnuRedisRepo(redisClient)

	// Build the dynamic OLT registry. Each OLT gets its own SNMP pool, repo,
	// usecase and handler; all share Redis. The default OLT keeps unprefixed
	// cache keys for backward compatibility.
	reg := NewOLTRegistry(cfg, redisRepo, cfg.DefaultOLT)
	reg.Reconcile(cfg.OLTs)
	defer reg.Close()

	if reg.Len() == 0 {
		return fmt.Errorf("no OLT could be initialized")
	}

	// Pre-warm cache for every registered OLT in the background.
	if cfg.CacheCfg.PreWarm {
		for _, id := range reg.List() {
			if e, ok := reg.Get(id); ok {
				go e.UC.PreWarmCache(ctx)
			}
		}
	}

	// Register dependency probes for /readyz. Redis is critical (5s cache).
	// The default OLT SNMP probe is critical (pod not-ready if it's down);
	// an aggregate registry probe is optional so a single secondary OLT
	// going down surfaces as degraded rather than taking the pod down.
	checker := health.NewChecker(2 * time.Second)
	checker.Register("redis", 5*time.Second, func(ctx context.Context) error {
		return redisClient.Ping(ctx).Err()
	})
	checker.Register("snmp_default", 30*time.Second, func(ctx context.Context) error {
		entry, ok := reg.GetDefault()
		if !ok {
			return fmt.Errorf("no default OLT registered")
		}
		done := make(chan error, 1)
		go func() { done <- entry.Repo.Ping() }()
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	checker.RegisterOptional("snmp_olts", 30*time.Second, reg.HealthCheck)

	var historyStore store.Store
	if cfg.StoreCfg.Enabled() {
		ts, err := store.OpenTimescale(ctx, cfg.StoreCfg)
		if err != nil {
			if cfg.PollCfg.Enabled {
				return fmt.Errorf("timescaledb: %w", err)
			}
			logger.Error("timescaledb_unavailable", zap.Error(err))
		} else {
			historyStore = ts
			defer ts.Close()
			checker.Register("timescaledb", 5*time.Second, ts.Ping)
			logger.Info("timescaledb_connected", zap.String("host", cfg.StoreCfg.Host))
		}
	}

	if cfg.PollCfg.Enabled {
		if historyStore == nil {
			return fmt.Errorf("poller enabled but history store is not connected")
		}
		p := poller.New(historyStore, reg, cfg.PollCfg)
		go p.Run(ctx)
		logger.Info("poller_started",
			zap.Duration("interval", cfg.PollCfg.Interval),
			zap.Duration("start_delay", cfg.PollCfg.StartDelay),
		)
	}

	var historyHandler *handler.HistoryHandler
	if historyStore != nil {
		historyHandler = handler.NewHistoryHandler(historyStore)
	}

	// Build the api_key -> Principal registry for per-tenant auth (nil when
	// API_USERS is unset; the legacy single API_KEY then applies).
	var principals map[string]reqctx.Principal
	if len(cfg.APIUsers) > 0 {
		principals = make(map[string]reqctx.Principal, len(cfg.APIUsers))
		for key, u := range cfg.APIUsers {
			principals[key] = reqctx.Principal{UserID: u.UserID, Admin: u.IsAdmin()}
		}
	}

	// Initialize router with the dynamic OLT registry and health checker.
	a.router = loadRoutesWithRegistry(reg, checker, principals, cfg.APIKey, historyHandler)

	// Start server.
	addr := os.Getenv("SERVER_PORT")
	if addr == "" {
		addr = "8081"
	}
	server := &http.Server{
		Addr:              serverAddress(addr),
		Handler:           a.router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("application started", zap.String("addr", addr))

	// Graceful shutdown.
	return graceful.Shutdown(ctx, server)
}

func serverAddress(port string) string {
	host := os.Getenv("SERVER_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
