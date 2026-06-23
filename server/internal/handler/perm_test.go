package handler

import (
	"testing"

	"mempalace/server/internal/auth"
)

func TestAllowsTool(t *testing.T) {
	cases := []struct {
		perm  auth.Perm
		tool  string
		allow bool
	}{
		// Read-only key: reads allowed, writes denied.
		{auth.PermRead, "mempalace_search", true},
		{auth.PermRead, "mempalace_list_drawers", true},
		{auth.PermRead, "mempalace_add_drawer", false},
		{auth.PermRead, "mempalace_delete_drawer", false},
		{auth.PermRead, "mempalace_kg_add", false},
		{auth.PermRead, "mempalace_hook_settings", false},

		// Full key: everything allowed.
		{auth.PermReadWrite, "mempalace_search", true},
		{auth.PermReadWrite, "mempalace_add_drawer", true},
		{auth.PermReadWrite, "mempalace_delete_drawer", true},

		// No valid key: nothing allowed.
		{auth.PermNone, "mempalace_search", false},
		{auth.PermNone, "mempalace_add_drawer", false},
	}

	for _, tc := range cases {
		if got := allowsTool(tc.perm, tc.tool); got != tc.allow {
			t.Errorf("allowsTool(%d, %q) = %v, want %v", tc.perm, tc.tool, got, tc.allow)
		}
	}
}
