// Package handler implements the MCP JSON-RPC over HTTP transport.
package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"

	"mempalace/server/internal/auth"
	"mempalace/server/internal/config"
	"mempalace/server/internal/embed"
	"mempalace/server/internal/storage"
)

const (
	serverName    = "mempalace-go"
	serverVersion = "1.0.0"
)

var supportedProtocolVersions = []string{
	"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05",
}

// Server handles MCP JSON-RPC requests over HTTP.
type Server struct {
	col      *storage.Collection
	graph    *storage.Graph // nil when AGE is not installed
	triples  *storage.TripleStore
	tunnels  *storage.TunnelStore
	settings *storage.SettingsStore
	embed    *embed.Client
	cfg      config.Config
	tools    map[string]toolDef
	router   map[string]func(args map[string]any) (any, error)
}

// New creates a Server and wires up all tool handlers.
// graph may be nil — the AGE-backed entity-graph tools return a clear error in
// that case. The temporal KG (triples), tunnels and settings stores are plain
// SQL and are always available.
func New(col *storage.Collection, graph *storage.Graph, triples *storage.TripleStore,
	tunnels *storage.TunnelStore, settings *storage.SettingsStore,
	embedClient *embed.Client, cfg config.Config) *Server {
	s := &Server{
		col: col, graph: graph, triples: triples, tunnels: tunnels,
		settings: settings, embed: embedClient, cfg: cfg,
	}
	s.registerTools()
	s.registerKGTools()
	s.registerTripleTools()
	s.registerGraphTools()
	s.registerMetaTools()
	return s
}

// reqCtx returns a background context for tool handler use.
func reqCtx() context.Context { return context.Background() }

// Register attaches HTTP routes to the provided mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /mp/mcp/health", s.handleHealth)
	mux.HandleFunc("POST /mp/mcp", s.handleMCP)
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","service":"mempalace-mcp-go"}`)) //nolint:errcheck
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, errResp(nil, codeParseError, "parse error"))
		return
	}

	// MCP Streamable HTTP (2025-03-26+): return a session ID on initialize.
	if req.Method == "initialize" {
		w.Header().Set("Mcp-Session-Id", newSessionID())
	}

	resp := s.dispatch(r, req)
	// Notifications (no ID) must not receive a response
	if req.ID == nil && resp.Error == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, resp)
}

// ---------------------------------------------------------------------------
// MCP protocol dispatch
// ---------------------------------------------------------------------------

func (s *Server) dispatch(r *http.Request, req rpcRequest) rpcResponse {
	id := req.ID

	switch req.Method {
	case "initialize":
		return s.handleInitialize(id, req.Params)
	case "notifications/initialized":
		return rpcResponse{} // notification — no response
	case "tools/list":
		return s.handleToolsList(id)
	case "tools/call":
		return s.handleToolsCall(r, id, req.Params)
	default:
		return errResp(id, codeMethodNotFound, "unknown method: "+req.Method)
	}
}

func (s *Server) handleInitialize(id any, params json.RawMessage) rpcResponse {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	json.Unmarshal(params, &p) //nolint:errcheck

	// Negotiate protocol version
	negotiated := supportedProtocolVersions[0]
	for _, v := range supportedProtocolVersions {
		if v == p.ProtocolVersion {
			negotiated = v
			break
		}
	}

	return okResp(id, map[string]any{
		"protocolVersion": negotiated,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo": map[string]any{
			"name":    serverName,
			"version": serverVersion,
		},
	})
}

func (s *Server) handleToolsList(id any) rpcResponse {
	defs := make([]toolDef, 0, len(s.tools))
	for _, def := range s.tools {
		defs = append(defs, def)
	}
	return okResp(id, toolsListResult{Tools: defs})
}

func (s *Server) handleToolsCall(r *http.Request, id any, params json.RawMessage) rpcResponse {
	var p toolsCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return errResp(id, codeInvalidParams, "invalid params: "+err.Error())
	}

	fn, ok := s.router[p.Name]
	if !ok {
		return errResp(id, codeMethodNotFound, "unknown tool: "+p.Name)
	}

	// Read-only keys may only call non-mutating tools.
	if !allowsTool(auth.PermFromContext(r.Context()), p.Name) {
		return errResp(id, codeForbidden, "write permission required for tool: "+p.Name)
	}

	// Whitelist args to declared schema properties only
	if def, hasDef := s.tools[p.Name]; hasDef {
		allowed := map[string]any{}
		for k, v := range p.Arguments {
			if _, declared := def.InputSchema.Properties[k]; declared {
				allowed[k] = v
			}
		}
		p.Arguments = allowed
	}

	result, err := fn(p.Arguments)
	if err != nil {
		log.Printf("tool %s error: %v", p.Name, err)
		return errResp(id, codeInternalError, err.Error())
	}

	text, _ := json.MarshalIndent(result, "", "  ")
	return okResp(id, toolResult{
		Content: []toolContent{{Type: "text", Text: string(text)}},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func newSessionID() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}
