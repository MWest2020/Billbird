package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mwesterweel/billbird/internal/admin"
	"github.com/mwesterweel/billbird/internal/api"
	"github.com/mwesterweel/billbird/internal/apitoken"
	"github.com/mwesterweel/billbird/internal/auth"
	"github.com/mwesterweel/billbird/internal/client"
	"github.com/mwesterweel/billbird/internal/config"
	"github.com/mwesterweel/billbird/internal/db"
	gh "github.com/mwesterweel/billbird/internal/github"
	"github.com/mwesterweel/billbird/internal/planentry"
	"github.com/mwesterweel/billbird/internal/timeentry"
	"github.com/mwesterweel/billbird/internal/webhook"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pool, err := db.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	ghClient, err := gh.NewClient(cfg.GitHubAppID, cfg.GitHubPrivateKey)
	if err != nil {
		log.Fatalf("github client: %v", err)
	}

	timeEntryStore := timeentry.NewStore(pool)
	planEntryStore := planentry.NewStore(pool)
	tokenStore := apitoken.NewStore(pool)
	clientResolver := client.NewResolver(pool)
	deliveries := webhook.NewDeliveryStore(pool)

	authCfg := auth.Config{
		ClientID:      cfg.GitHubClientID,
		ClientSecret:  cfg.GitHubClientSecret,
		AllowedOrgs:   cfg.AllowedOrgs,
		SessionSecret: cfg.SessionSecret,
		BaseURL:       envOrDefault("BASE_URL", "http://localhost:"+cfg.Port),
	}
	authHandler := auth.NewHandler(authCfg, pool)

	// One membership policy is shared by both webhook command auth and
	// REST API token auth — same cache, same rate-limit footprint, same
	// dev-bypass switch.
	var policy auth.MembershipPolicy
	if os.Getenv("BILLBIRD_DEV_MEMBERSHIP_BYPASS") == "true" {
		log.Print("!!! BILLBIRD_DEV_MEMBERSHIP_BYPASS=true: every bearer token is treated as a member of an allowed org. NEVER use this in production. !!!")
		policy = devAllowAllPolicy{}
	} else {
		checker := auth.NewMembershipChecker(ghClient, cfg.AllowedOrgs, 5*time.Minute)
		if err := checker.PrimeInstallations(); err != nil {
			// Non-fatal: a fresh app may have no installations yet. Token auth
			// will refresh on first miss.
			log.Printf("warn: priming installations failed: %v", err)
		}
		policy = checker
	}

	webhookHandler := webhook.NewHandler(
		cfg.GitHubWebhookSecret,
		policy,
		deliveries,
		ghClient,
		timeEntryStore,
		planEntryStore,
	)
	webhookHandler.SetClientResolver(clientResolver)

	apiAuthDeps := auth.APIAuthDependencies{
		Cookie:     authHandler,
		Tokens:     tokenStore,
		Membership: policy,
	}

	apiHandler := api.NewHandler(pool, timeEntryStore, planEntryStore, tokenStore)
	adminHandler, err := admin.NewHandler(pool, timeEntryStore, planEntryStore, tokenStore)
	if err != nil {
		log.Fatalf("admin handler: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusFound)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("POST /webhook", webhookHandler.Handle)

	authHandler.RegisterRoutes(mux)
	apiHandler.RegisterRoutes(mux, apiAuthDeps.RequireAPIAuth)
	adminHandler.RegisterRoutes(mux, authHandler.RequireAuth)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("billbird listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Print("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// devAllowAllPolicy unconditionally allows any GitHub user. Used only
// when BILLBIRD_DEV_MEMBERSHIP_BYPASS=true is set explicitly. The
// startup banner warns operators that this is a development-only
// shortcut and must never be enabled in production.
type devAllowAllPolicy struct{}

func (devAllowAllPolicy) IsAllowed(string) bool { return true }
