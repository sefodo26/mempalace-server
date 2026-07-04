package config

import "github.com/caarlos0/env/v11"

// Config holds all server settings sourced from environment variables.
// Values are parsed from the environment via struct tags (caarlos0/env):
// `env` names the variable, `envDefault` gives the fallback when it is unset.
type Config struct {
	// PostgreSQL
	DatabaseURL string `env:"MEMPALACE_DB_URL"`
	PoolMin     int32  `env:"MEMPALACE_PG_POOL_MIN" envDefault:"2"`
	PoolMax     int32  `env:"MEMPALACE_PG_POOL_MAX" envDefault:"10"`

	// Multi-tenancy
	TenantID string `env:"MEMPALACE_TENANT_ID" envDefault:"default"`

	// Auth
	MCPAPIKey         string `env:"MCP_API_KEY"`          // full access (read + write); required
	MCPAPIKeyReadOnly string `env:"MCP_API_KEY_READONLY"` // optional read-only key; "" disables it

	// Embedding (OpenAI-compatible API — works with Ollama, LM Studio, etc.)
	EmbedAPIURL string `env:"EMBED_API_URL" envDefault:"http://localhost:11434/v1"`
	EmbedAPIKey string `env:"EMBED_API_KEY"`                           // optional, e.g. for OpenAI
	EmbedModel  string `env:"EMBED_MODEL" envDefault:"embeddinggemma"` // multilingual (100+ langs); or nomic-embed-text, text-embedding-3-small
	EmbedDim    int    `env:"EMBED_DIM" envDefault:"768"`              // must equal model output, cannot exceed it; 768 for embeddinggemma (Matryoshka: also 512/256/128)

	// HNSW search quality (higher = better recall, slower)
	EFSearch int `env:"MEMPALACE_HNSW_EF_SEARCH" envDefault:"100"`

	// Optional plain REST/JSON API (off by default; MCP is always on)
	EnableRESTAPI bool `env:"ENABLE_REST_API" envDefault:"false"`

	// Knowledge-graph auto-population (opt-in). When enabled, add_drawer also
	// writes to the AGE graph using the selected extractor strategy. Default
	// off — add_drawer stays storage-only, preserving existing behavior.
	GraphAutoPopulate bool   `env:"MEMPALACE_GRAPH_AUTO_POPULATE" envDefault:"false"`
	GraphExtractor    string `env:"MEMPALACE_GRAPH_EXTRACTOR" envDefault:"structural"` // "structural" (no LLM) or "llm"

	// LLM extraction — only used when GraphExtractor == "llm".
	// OpenAI-compatible chat endpoint (Ollama, LM Studio, OpenAI, …).
	LLMAPIURL string `env:"LLM_API_URL"` // e.g. http://host.docker.internal:11434/v1
	LLMAPIKey string `env:"LLM_API_KEY"` // optional (empty for local servers)
	LLMModel  string `env:"LLM_MODEL"`   // e.g. llama3.1, qwen2.5

	// HTTP
	Port string `env:"PORT" envDefault:"8000"`
}

// Load parses the configuration from the environment. It returns an error when
// a variable is set to a value that cannot be parsed into its field type (e.g.
// EMBED_DIM=abc), so the server fails fast on misconfiguration instead of
// silently falling back to a default.
func Load() (Config, error) {
	return env.ParseAs[Config]()
}
