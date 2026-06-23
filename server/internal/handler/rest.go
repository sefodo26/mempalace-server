// Package handler — optional plain REST/JSON API.
//
// This is a thin, OPTIONAL convenience layer on top of the same tool handlers
// the MCP endpoint uses. It is only mounted when ENABLE_REST_API=true.
// MCP at POST /mp/mcp is always available and is unaffected by this file.
//
// Every endpoint reuses an existing tool, so behaviour (validation, embedding,
// storage) stays identical to MCP. Auth is the same bearer token as MCP.
//
// Base path: /mp/api/v1
//
//	GET    /mp/api/v1/health
//	GET    /mp/api/v1/status
//	GET    /mp/api/v1/wings
//	GET    /mp/api/v1/rooms?wing=
//	GET    /mp/api/v1/taxonomy
//	POST   /mp/api/v1/search          {query, limit?, wing?, room?, max_distance?, context?}
//	GET    /mp/api/v1/drawers?wing=&room=&limit=&offset=
//	POST   /mp/api/v1/drawers         {wing, room, content, source_file?, added_by?}
//	GET    /mp/api/v1/drawers/{id}
//	PATCH  /mp/api/v1/drawers/{id}    {content?, wing?, room?}
//	DELETE /mp/api/v1/drawers/{id}
package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"mempalace/server/internal/auth"
)

const restBase = "/mp/api/v1"

// RegisterREST mounts the optional REST/JSON API on the given mux.
// Call this only when cfg.EnableRESTAPI is true.
func (s *Server) RegisterREST(mux *http.ServeMux) {
	mux.HandleFunc("GET "+restBase+"/health", s.handleHealth)

	mux.HandleFunc("GET "+restBase+"/status", s.restGet("mempalace_status"))
	mux.HandleFunc("GET "+restBase+"/wings", s.restGet("mempalace_list_wings"))
	mux.HandleFunc("GET "+restBase+"/rooms", s.restGet("mempalace_list_rooms", "wing"))
	mux.HandleFunc("GET "+restBase+"/taxonomy", s.restGet("mempalace_get_taxonomy"))

	mux.HandleFunc("POST "+restBase+"/search", s.restBody("mempalace_search"))

	mux.HandleFunc("GET "+restBase+"/drawers", s.restGet("mempalace_list_drawers", "wing", "room", "limit", "offset"))
	mux.HandleFunc("POST "+restBase+"/drawers", s.restBody("mempalace_add_drawer"))
	mux.HandleFunc("GET "+restBase+"/drawers/{id}", s.restPath("mempalace_get_drawer"))
	mux.HandleFunc("PATCH "+restBase+"/drawers/{id}", s.restPath("mempalace_update_drawer"))
	mux.HandleFunc("DELETE "+restBase+"/drawers/{id}", s.restPath("mempalace_delete_drawer"))
}

// restGet builds a handler for GET endpoints. Listed query params are copied
// into the tool args (numeric-looking values become numbers so int/float
// args parse correctly).
func (s *Server) restGet(tool string, queryKeys ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args := map[string]any{}
		for _, k := range queryKeys {
			if v := r.URL.Query().Get(k); v != "" {
				args[k] = coerce(v)
			}
		}
		s.runTool(w, r, tool, args)
	}
}

// restBody builds a handler that decodes the JSON request body into tool args.
func (s *Server) restBody(tool string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args := map[string]any{}
		if r.Body != nil {
			dec := json.NewDecoder(r.Body)
			dec.UseNumber() // so intArg/floatArg parse cleanly
			if err := dec.Decode(&args); err != nil && err.Error() != "EOF" {
				writeRESTError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
				return
			}
		}
		s.runTool(w, r, tool, args)
	}
}

// restPath builds a handler for endpoints with a {id} path value, optionally
// merged with a JSON body (used by PATCH update).
func (s *Server) restPath(tool string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args := map[string]any{}
		if r.Body != nil && r.ContentLength != 0 {
			dec := json.NewDecoder(r.Body)
			dec.UseNumber()
			if err := dec.Decode(&args); err != nil && err.Error() != "EOF" {
				writeRESTError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
				return
			}
		}
		args["drawer_id"] = r.PathValue("id")
		s.runTool(w, r, tool, args)
	}
}

// runTool invokes a registered tool by name and writes its result as JSON.
// Args are whitelisted to the tool's declared schema, exactly like MCP.
func (s *Server) runTool(w http.ResponseWriter, r *http.Request, tool string, args map[string]any) {
	fn, ok := s.router[tool]
	if !ok {
		writeRESTError(w, http.StatusNotFound, "unknown endpoint")
		return
	}

	// Read-only keys may only call non-mutating tools.
	if !allowsTool(auth.PermFromContext(r.Context()), tool) {
		writeRESTError(w, http.StatusForbidden, "write permission required")
		return
	}

	if def, hasDef := s.tools[tool]; hasDef {
		allowed := map[string]any{}
		for k, v := range args {
			if _, declared := def.InputSchema.Properties[k]; declared {
				allowed[k] = v
			}
		}
		args = allowed
	}

	result, err := fn(args)
	if err != nil {
		log.Printf("rest tool %s error: %v", tool, err)
		writeRESTError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("rest write response: %v", err)
	}
}

func writeRESTError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

// coerce turns a query-string value into a number when it looks like one,
// so numeric tool args (limit, offset, …) parse; otherwise keeps the string.
func coerce(v string) any {
	if i, err := strconv.Atoi(v); err == nil {
		return float64(i)
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return v
}
