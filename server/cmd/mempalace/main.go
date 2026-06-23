package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mempalace/server/internal/auth"
	"mempalace/server/internal/config"
	"mempalace/server/internal/embed"
	"mempalace/server/internal/handler"
	"mempalace/server/internal/storage"
)

func main() {
	cfg := config.Load()

	if cfg.DatabaseURL == "" {
		log.Fatal("MEMPALACE_DB_URL is required")
	}
	if cfg.MCPAPIKey == "" {
		log.Println("WARNING: MCP_API_KEY is not set — all requests will be rejected")
	}
	if cfg.MCPAPIKeyReadOnly != "" {
		if cfg.MCPAPIKeyReadOnly == cfg.MCPAPIKey {
			log.Fatal("MCP_API_KEY_READONLY must differ from MCP_API_KEY")
		}
		log.Println("read-only API key enabled (MCP_API_KEY_READONLY)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- Database ---
	pool, err := storage.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	if err := storage.Provision(ctx, pool, cfg.TenantID, cfg.EmbedDim); err != nil {
		log.Fatalf("provision schema: %v", err)
	}

	col := storage.NewCollection(pool, cfg.TenantID, cfg.EFSearch)

	// --- Temporal knowledge graph, tunnels, settings (plain SQL) ---
	// These are always available; provisioning is idempotent.
	if err := storage.ProvisionKG(ctx, pool, cfg.TenantID); err != nil {
		log.Fatalf("provision knowledge graph: %v", err)
	}
	if err := storage.ProvisionTunnels(ctx, pool, cfg.TenantID); err != nil {
		log.Fatalf("provision tunnels: %v", err)
	}
	if err := storage.ProvisionSettings(ctx, pool, cfg.TenantID); err != nil {
		log.Fatalf("provision settings: %v", err)
	}
	triples := storage.NewTripleStore(pool, cfg.TenantID)
	tunnels := storage.NewTunnelStore(pool, cfg.TenantID)
	settings := storage.NewSettingsStore(pool, cfg.TenantID)

	// --- Entity-graph (Apache AGE) ---
	// Best-effort: if AGE is not installed the server still starts,
	// and the AGE-backed entity tools return a clear error message instead.
	var graph *storage.Graph
	if err := storage.ProvisionGraph(ctx, pool, cfg.TenantID); err != nil {
		log.Printf("entity graph (AGE) unavailable: %v", err)
	} else {
		graph = storage.NewGraph(pool, cfg.TenantID)
	}

	// --- Embedding client ---
	embedClient := embed.NewClient(cfg.EmbedAPIURL, cfg.EmbedAPIKey, cfg.EmbedModel, cfg.EmbedDim)

	// --- HTTP server ---
	mux := http.NewServeMux()
	h := handler.New(col, graph, triples, tunnels, settings, embedClient, cfg)
	h.Register(mux)

	// Optional plain REST/JSON API (off unless ENABLE_REST_API=true).
	if cfg.EnableRESTAPI {
		h.RegisterREST(mux)
		log.Printf("REST API enabled at /mp/api/v1")
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      auth.Bearer(cfg.MCPAPIKey, cfg.MCPAPIKeyReadOnly, mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on SIGTERM / SIGINT
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("mempalace-go listening on :%s (tenant=%s)", cfg.Port, cfg.TenantID)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-stop
	log.Println("shutting down…")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("done")
}
