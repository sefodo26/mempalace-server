package config

import (
	"os"
	"strconv"
)

// Config holds all server settings sourced from environment variables.
type Config struct {
	// PostgreSQL
	DatabaseURL string
	PoolMin     int32
	PoolMax     int32

	// Multi-tenancy
	TenantID string

	// Auth
	MCPAPIKey         string // full access (read + write); required
	MCPAPIKeyReadOnly string // optional read-only key; "" disables it

	// Embedding (OpenAI-compatible API — works with Ollama, LM Studio, etc.)
	EmbedAPIURL string // e.g. http://ollama:11434/v1
	EmbedAPIKey string // optional, e.g. for OpenAI
	EmbedModel  string // e.g. embeddinggemma (multilingual, 100+ langs), nomic-embed-text, text-embedding-3-small
	EmbedDim    int    // must equal model output, cannot exceed it; 768 for embeddinggemma (Matryoshka: also 512/256/128)

	// HNSW search quality (higher = better recall, slower)
	EFSearch int

	// Optional plain REST/JSON API (off by default; MCP is always on)
	EnableRESTAPI bool

	// HTTP
	Port string
}

func Load() Config {
	return Config{
		DatabaseURL:       env("MEMPALACE_DB_URL", ""),
		PoolMin:           int32(envInt("MEMPALACE_PG_POOL_MIN", 2)),
		PoolMax:           int32(envInt("MEMPALACE_PG_POOL_MAX", 10)),
		TenantID:          env("MEMPALACE_TENANT_ID", "default"),
		MCPAPIKey:         env("MCP_API_KEY", ""),
		MCPAPIKeyReadOnly: env("MCP_API_KEY_READONLY", ""),
		EmbedAPIURL:       env("EMBED_API_URL", "http://localhost:11434/v1"),
		EmbedAPIKey:       env("EMBED_API_KEY", ""),
		EmbedModel:        env("EMBED_MODEL", "embeddinggemma"),
		EmbedDim:          envInt("EMBED_DIM", 768),
		EFSearch:          envInt("MEMPALACE_HNSW_EF_SEARCH", 100),
		EnableRESTAPI:     envBool("ENABLE_REST_API", false),
		Port:              env("PORT", "8000"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
