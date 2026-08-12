package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/accesslog"
	"github.com/oniharnantyo/tinyroute/internal/auth"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dashboard"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/anthropic"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/gemini"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/openai"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/openairesponses"
	"github.com/oniharnantyo/tinyroute/internal/history"
	"github.com/oniharnantyo/tinyroute/internal/history/sqlite"
	"github.com/oniharnantyo/tinyroute/internal/probe"
	"github.com/oniharnantyo/tinyroute/internal/proxy"
	"github.com/oniharnantyo/tinyroute/internal/route"
	"github.com/oniharnantyo/tinyroute/internal/translate"
	"github.com/urfave/cli/v3"
)

// providerInfo maps a config.Provider to the runtime shape the proxy consumes.
// Every field the proxy reads must be propagated here: dropping one silently
// disables the branch that depends on it — notably Transport, which gates the
// cloudcode envelope path in proxy.Handler.
func providerInfo(name string, p config.Provider, store *credential.Store) proxy.ProviderInfo {
	return proxy.ProviderInfo{
		Dialect:    p.Dialect,
		BaseURL:    p.BaseURL,
		Transport:  p.Transport,
		APIKey:     p.APIKey,
		Credential: p.BuildCredential(name, store),
		Headers:    p.Headers,
	}
}

func cmdServe() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the HTTP proxy server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "env-file",
				Usage: "Explicit path to a .env file",
			},
			&cli.BoolFlag{
				Name:  "no-dashboard",
				Usage: "Disable web management dashboard",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			envFile := cmd.String("env-file")

			if err := config.LoadDotenv(envFile); err != nil {
				return err
			}

			svc, err := config.LoadService()
			if err != nil {
				return err
			}

			var level slog.Level
			switch strings.ToLower(svc.LogLevel) {
			case "debug":
				level = slog.LevelDebug
			case "info":
				level = slog.LevelInfo
			case "warn", "warning":
				level = slog.LevelWarn
			case "error":
				level = slog.LevelError
			default:
				level = slog.LevelInfo
			}

			var logHandler slog.Handler
			if strings.ToLower(svc.LogFormat) == "json" {
				logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
			} else {
				logHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
			}
			logger := slog.New(logHandler)

			// Auto-initialize if config files don't exist
			if needsInit(svc.ConfigPath, svc.KeysPath) {
				log.Println("config files not found - running first-time setup...")
				if err := runInit(); err != nil {
					return fmt.Errorf("auto-init failed: %w", err)
				}
				log.Println("setup complete - starting server...")
			}

			// Validate configuration files
			if err := validateConfig(svc); err != nil {
				return fmt.Errorf("config validation failed: %w", err)
			}

			topologyWatcher, err := config.NewWatcher(svc.ConfigPath, config.ParseTopology)
			if err != nil {
				return err
			}

			keyWatcher, err := config.NewWatcher(svc.KeysPath, auth.ParseKeyFile)
			if err != nil {
				return err
			}

			clock := route.RealClock{}
			health := route.NewHealthStore(clock, svc.StatePath)
			if err := health.Load(); err != nil {
				log.Printf("warning: failed to load state (%s): %v", svc.StatePath, err)
			}

			dbStore, err := sqlite.Open(svc.HistoryDBPath)
			if err != nil {
				return fmt.Errorf("open history db (%s): %w", svc.HistoryDBPath, err)
			}
			defer dbStore.Close()

			recorder := sqlite.NewStore(dbStore)

			rateLimiter := auth.NewRateLimiter(func(keyID string) *auth.RateSpec {
				ks := keyWatcher.Get()
				if ks == nil {
					return nil
				}
				key, ok := ks.Lookup(keyID)
				if !ok {
					return nil
				}
				return key.Rate
			})

			selector := &route.OrderedSelector{}

			getRouter := func() (*route.Router, error) {
				topo := topologyWatcher.Get()
				if topo == nil {
					return nil, fmt.Errorf("no topology loaded")
				}
				rawRoutes := make([]route.RawRoute, 0, len(topo.Routes))
				for _, r := range topo.Routes {
					rawRoutes = append(rawRoutes, route.RawRoute{
						From:  r.From,
						Match: r.Match,
						Chain: r.Chain,
					})
				}
				entries, err := route.ParseRoutes(rawRoutes)
				if err != nil {
					return nil, err
				}
				return route.New(entries, topo.Providers, route.WithTranslatable(translate.CanTranslate)), nil
			}

			credStore, err := credential.NewStore(svc.CredentialsPath)
			if err != nil {
				log.Printf("warning: failed to initialize credential store (%s): %v", svc.CredentialsPath, err)
			}

			getProvider := func(name string) (proxy.ProviderInfo, bool) {
				topo := topologyWatcher.Get()
				if topo == nil {
					return proxy.ProviderInfo{}, false
				}
				p, ok := topo.Providers[name]
				if !ok {
					return proxy.ProviderInfo{}, false
				}
				return providerInfo(name, p, credStore), true
			}

			getDialect := func(name string) (core.Dialect, bool) {
				return dialect.ByName(name)
			}

			transport := &http.Transport{
				ResponseHeaderTimeout: svc.Cooldown5xx,
			}

			deps := &proxy.Deps{
				Logger:      logger,
				Transport:   transport,
				GetProvider: getProvider,
				GetDialect:  getDialect,
				Health:      health,
				Selector:    selector,
				Recorder:    recorder,
				CaptureMode: svc.Capture,
				InjectUsage: svc.InjectUsage,
				Cooldown429: svc.Cooldown429,
				Cooldown5xx: svc.Cooldown5xx,
			}

			handler := proxy.Handler(deps)

			// Probe-scoped deps: a dashboard Test click must not pollute request
			// history or count against quota. Health penalties still apply — a
			// failing probe is genuine signal that the provider is unhealthy.
			probeDeps := *deps
			probeDeps.Recorder = nil
			probeDeps.UsageStore = nil
			probeDeps.NoPenalties = true
			probeHandler := proxy.Handler(&probeDeps)
			runProbe := func(ctx context.Context, provName, dialectName, model string, timeout time.Duration) (int, time.Duration, error) {
				router, err := getRouter()
				if err != nil {
					return 0, 0, fmt.Errorf("routing unavailable: %w", err)
				}
				return probe.RunInProcess(ctx, provName, dialectName, model, router.Resolve, probeHandler, timeout)
			}

			mux := http.NewServeMux()
			for _, name := range dialect.Names() {
				d, ok := dialect.ByName(name)
				if !ok {
					continue
				}
				for _, p := range d.MountPaths() {
					mux.Handle(p, requestHandler(d, logger, keyWatcher, rateLimiter, getRouter, handler))
				}

				modelsMount := d.ModelsMountPath()
				if modelsMount == "" {
					// Dialect shares another surface's models endpoint (e.g.
					// openai-responses is served by /openai/v1/models).
					continue
				}

				dCopy := d
				modelsPath := "GET " + modelsMount
				mux.HandleFunc(modelsPath, func(w http.ResponseWriter, r *http.Request) {
					router, err := getRouter()
					if err != nil {
						dCopy.WriteError(w, http.StatusInternalServerError, "api_error", err.Error())
						return
					}
					dCopy.WriteModels(w, router.Models(dCopy.Name()))
				})
			}

			noDashboard := cmd.Bool("no-dashboard")
			dashboardEnabled := svc.DashboardEnable && !noDashboard

			if dashboardEnabled {
				passStore, err := dashboard.NewPasswordStore(svc.DashboardPasswordPath)
				if err != nil {
					log.Printf("warning: failed to initialize dashboard password store: %v", err)
				} else {
					dashDeps := &dashboard.Deps{
						Service:         svc,
						PasswordStore:   passStore,
						SessionStore:    dashboard.NewSessionStore(),
						LoginLimiter:    dashboard.NewLoginLimiter(),
						TopologyWatcher: topologyWatcher,
						KeyWatcher:      keyWatcher,
						HealthStore:     health,
						HistoryQuerier:  recorder,
						RunProbe:        runProbe,
					}
					dashboard.RegisterRoutes(mux, dashDeps)
				}
			}

			srv := &http.Server{
				Addr:    svc.Listen,
				Handler: accesslog.Middleware(logger)(mux),
			}

			errCh := make(chan error, 1)
			go func() {
				var err error
				if svc.TLSCert != "" {
					log.Printf("tinyroute listening on https://%s", svc.Listen)
					if dashboardEnabled {
						dashURL := fmt.Sprintf("https://%s/dashboard/overview", svc.Listen)
						log.Printf("dashboard available at %s", dashURL)
					}
					err = srv.ListenAndServeTLS(svc.TLSCert, svc.TLSKey)
				} else {
					log.Printf("tinyroute listening on http://%s", svc.Listen)
					if dashboardEnabled {
						dashURL := fmt.Sprintf("http://%s/dashboard/overview", svc.Listen)
						log.Printf("dashboard available at %s", dashURL)
					}
					err = srv.ListenAndServe()
				}
				if err != nil {
					errCh <- err
				}
			}()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

			select {
			case err := <-errCh:
				return err
			case <-sigCh:
				log.Println("shutting down...")
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := srv.Shutdown(shutdownCtx); err != nil {
				return err
			}
			if err := health.Save(); err != nil {
				log.Printf("warning: failed to save state: %v", err)
			}
			return nil
		},
	}
}

// requestHandler wires the per-request flow described in the design: auth,
// dialect resolution, body parsing, key verification, rate limiting, route
// resolution, then the shared proxy.Handler for the attempt loop.
// needsInit checks if the critical config files exist.
func needsInit(configPath, keysPath string) bool {
	_, configErr := os.Stat(configPath)
	_, keysErr := os.Stat(keysPath)
	return os.IsNotExist(configErr) || os.IsNotExist(keysErr)
}

// runInit executes the first-time initialization logic (minimal version, no .env).
func runInit() error {
	// Run minimal init that skips .env creation
	return cmdMinimalInit()
}

// validateConfig checks the configuration files for errors before starting server.
func validateConfig(svc config.Service) error {
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}

	topo, err := config.ParseTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	errs := config.ValidateTopology(topo, dialect.Names())
	if len(errs) == 0 {
		log.Printf("config validated: %d provider(s), %d route(s)\n", len(topo.Providers), len(topo.Routes))
		return nil
	}

	for _, e := range errs {
		log.Printf("validation error: %v\n", e)
	}
	return fmt.Errorf("%d validation error(s)", len(errs))
}

// requestHandler wires the per-request flow described in the design: auth,
// dialect resolution, body parsing, key verification, rate limiting, route
// resolution, then the shared proxy.Handler for the attempt loop.
func requestHandler(
	d core.Dialect,
	logger *slog.Logger,
	keyWatcher *config.Watcher[auth.KeyStore],
	rateLimiter *auth.RateLimiter,
	getRouter func() (*route.Router, error),
	next http.HandlerFunc,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqID := accesslog.RequestID(r.Context())
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			authHeader = r.Header.Get("x-api-key")
		}
		token := bearerToken(authHeader)

		body, err := readBody(r)
		if err != nil {
			if logger != nil {
				logger.Debug("read body failed", slog.String("request_id", reqID), slog.Any("error", err))
			}
			d.WriteError(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
			return
		}

		parsed, err := d.ParseRequest(body)
		if err != nil {
			if logger != nil {
				logger.Debug("parse request failed", slog.String("request_id", reqID), slog.Any("error", err))
			}
			d.WriteError(w, http.StatusBadRequest, "invalid_request_error", "failed to parse request body")
			return
		}

		ks := keyWatcher.Get()
		keyID, err := ks.Verify(token, d.Name(), parsed.Model)
		if err != nil {
			if logger != nil {
				logger.Debug("authentication failed", slog.String("request_id", reqID), slog.String("model", parsed.Model), slog.Any("error", err))
			}
			d.WriteError(w, http.StatusUnauthorized, "authentication_error", err.Error())
			return
		}

		if allowed, retryAfter := rateLimiter.Allow(keyID); !allowed {
			if logger != nil {
				logger.Debug("rate limit exceeded", slog.String("request_id", reqID), slog.String("key_id", keyID), slog.Duration("retry_after", retryAfter))
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			d.WriteError(w, http.StatusTooManyRequests, "rate_limit_error", "rate limit exceeded")
			return
		}

		router, err := getRouter()
		if err != nil {
			if logger != nil {
				logger.Debug("routing unavailable", slog.String("request_id", reqID), slog.Any("error", err))
			}
			d.WriteError(w, http.StatusInternalServerError, "api_error", "routing is unavailable")
			return
		}

		resolved, err := router.Resolve(d.Name(), parsed.Model)
		if err != nil {
			if logger != nil {
				logger.Debug("model resolution failed", slog.String("request_id", reqID), slog.String("model", parsed.Model), slog.Any("error", err))
			}
			d.WriteError(w, http.StatusNotFound, "not_found_error", err.Error())
			return
		}

		sessionID := history.DeriveSessionID(r.Header.Get("X-Session-Id"), parsed, time.Now())

		rc := &proxy.RequestCtx{
			Dialect:   d,
			Route:     resolved,
			Parsed:    parsed,
			RequestID: reqID,
			KeyID:     keyID,
			SessionID: sessionID,
		}

		ctx := proxy.WithRequestContext(r.Context(), rc)
		next(w, r.WithContext(ctx))
	}
}

// readBody reads and returns the full request body, restoring r.Body so it
// can be read again downstream.
func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func bearerToken(authHeader string) string {
	const prefix = "Bearer "
	if len(authHeader) >= len(prefix) && strings.EqualFold(authHeader[:len(prefix)], prefix) {
		return strings.TrimSpace(authHeader[len(prefix):])
	}
	return strings.TrimSpace(authHeader)
}
