package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.EmbedDim != 768 {
		t.Errorf("EmbedDim = %d, want 768", cfg.EmbedDim)
	}
	if cfg.EmbedModel != "embeddinggemma" {
		t.Errorf("EmbedModel = %q, want embeddinggemma", cfg.EmbedModel)
	}
	if cfg.GraphExtractor != "structural" {
		t.Errorf("GraphExtractor = %q, want structural", cfg.GraphExtractor)
	}
	if cfg.GraphAutoPopulate {
		t.Error("GraphAutoPopulate = true, want false by default")
	}
	if cfg.PoolMin != 2 || cfg.PoolMax != 10 {
		t.Errorf("pool = %d/%d, want 2/10", cfg.PoolMin, cfg.PoolMax)
	}
	if cfg.Port != "8000" {
		t.Errorf("Port = %q, want 8000", cfg.Port)
	}
}

func TestLoadOverride(t *testing.T) {
	t.Setenv("EMBED_DIM", "512")
	t.Setenv("MEMPALACE_GRAPH_AUTO_POPULATE", "true")
	t.Setenv("MEMPALACE_GRAPH_EXTRACTOR", "llm")
	t.Setenv("LLM_MODEL", "llama3.1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.EmbedDim != 512 {
		t.Errorf("EmbedDim = %d, want 512", cfg.EmbedDim)
	}
	if !cfg.GraphAutoPopulate {
		t.Error("GraphAutoPopulate = false, want true")
	}
	if cfg.GraphExtractor != "llm" {
		t.Errorf("GraphExtractor = %q, want llm", cfg.GraphExtractor)
	}
	if cfg.LLMModel != "llama3.1" {
		t.Errorf("LLMModel = %q, want llama3.1", cfg.LLMModel)
	}
}

func TestLoadInvalidIntFailsFast(t *testing.T) {
	t.Setenv("EMBED_DIM", "not-a-number")
	if _, err := Load(); err == nil {
		t.Error("expected error for non-numeric EMBED_DIM, got nil")
	}
}
