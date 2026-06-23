package handler

import "mempalace/server/internal/auth"

// writeTools lists every tool that mutates state. Tools not in this set are
// treated as read-only. A read-only API key (PermRead) may only call read
// tools; calling a write tool is rejected.
var writeTools = map[string]bool{
	// Drawers
	"mempalace_add_drawer":    true,
	"mempalace_update_drawer": true,
	"mempalace_delete_drawer": true,
	"mempalace_diary_write":   true,
	// Tunnels
	"mempalace_create_tunnel": true,
	"mempalace_delete_tunnel": true,
	// Temporal knowledge graph
	"mempalace_kg_add":        true,
	"mempalace_kg_invalidate": true,
	// Entity graph
	"mempalace_kg_add_entity":    true,
	"mempalace_kg_add_relation":  true,
	"mempalace_kg_delete_entity": true,
	// Settings (can mutate hook behavior)
	"mempalace_hook_settings": true,
}

// isWriteTool reports whether calling the named tool requires write access.
func isWriteTool(name string) bool { return writeTools[name] }

// allowsTool reports whether the given permission level may call the named tool.
func allowsTool(perm auth.Perm, name string) bool {
	if isWriteTool(name) {
		return perm == auth.PermReadWrite
	}
	return perm == auth.PermRead || perm == auth.PermReadWrite
}
