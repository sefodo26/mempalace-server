package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// registerTools wires every MCP tool name to its definition + handler.
func (s *Server) registerTools() {
	s.tools = map[string]toolDef{}
	s.router = map[string]func(map[string]any) (any, error){}

	s.add("mempalace_status", "Palace overview — total drawers, wing and room counts.",
		inputSchema{Type: "object"},
		s.toolStatus)

	s.add("mempalace_list_wings", "List all wings with drawer counts.",
		inputSchema{Type: "object"},
		s.toolListWings)

	s.add("mempalace_list_rooms",
		"List rooms within a wing (or all rooms if wing is omitted).",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"wing": {Type: "string", Description: "Wing name (optional)"},
			},
		},
		s.toolListRooms)

	s.add("mempalace_get_taxonomy",
		"Full taxonomy tree: wing → room → drawer count.",
		inputSchema{Type: "object"},
		s.toolGetTaxonomy)

	s.add("mempalace_search",
		"Semantic search — returns drawer content with similarity scores.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"query":        {Type: "string", Description: "Search query", MaxLength: intPtr(250)},
				"limit":        {Type: "integer", Description: "Max results (1-100)", Default: 5, Minimum: intPtr(1), Maximum: intPtr(100)},
				"wing":         {Type: "string", Description: "Restrict to this wing"},
				"room":         {Type: "string", Description: "Restrict to this room"},
				"max_distance": {Type: "number", Description: "Max cosine distance (0 = disabled)", Default: 1.5},
				"context":      {Type: "string", Description: "Extra context prepended to query"},
			},
			Required: []string{"query"},
		},
		s.toolSearch)

	s.add("mempalace_check_duplicate",
		"Check whether content already exists in the palace before filing.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"content":   {Type: "string", Description: "Content to check"},
				"threshold": {Type: "number", Description: "Similarity threshold 0-1 (default 0.9)", Default: 0.9},
			},
			Required: []string{"content"},
		},
		s.toolCheckDuplicate)

	s.add("mempalace_add_drawer",
		"File verbatim content into the palace.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"wing":        {Type: "string", Description: "Wing (broad category / project)"},
				"room":        {Type: "string", Description: "Room (aspect / topic)"},
				"content":     {Type: "string", Description: "Verbatim content to store"},
				"source_file": {Type: "string", Description: "Source file path (optional)"},
				"added_by":    {Type: "string", Description: "Who is filing (default: mcp)"},
			},
			Required: []string{"wing", "room", "content"},
		},
		s.toolAddDrawer)

	s.add("mempalace_delete_drawer",
		"Delete a drawer by its ID.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"drawer_id": {Type: "string", Description: "Drawer ID to delete"},
			},
			Required: []string{"drawer_id"},
		},
		s.toolDeleteDrawer)

	s.add("mempalace_get_drawer",
		"Fetch a single drawer by ID.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"drawer_id": {Type: "string", Description: "Drawer ID"},
			},
			Required: []string{"drawer_id"},
		},
		s.toolGetDrawer)

	s.add("mempalace_list_drawers",
		"List drawers with optional wing/room filter and pagination.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"wing":   {Type: "string", Description: "Filter by wing"},
				"room":   {Type: "string", Description: "Filter by room"},
				"limit":  {Type: "integer", Description: "Page size (1-100)", Default: 20, Minimum: intPtr(1), Maximum: intPtr(100)},
				"offset": {Type: "integer", Description: "Pagination offset", Default: 0, Minimum: intPtr(0)},
			},
		},
		s.toolListDrawers)

	s.add("mempalace_update_drawer",
		"Update an existing drawer's content and/or metadata.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"drawer_id": {Type: "string", Description: "Drawer ID to update"},
				"content":   {Type: "string", Description: "New content (optional)"},
				"wing":      {Type: "string", Description: "New wing (optional)"},
				"room":      {Type: "string", Description: "New room (optional)"},
			},
			Required: []string{"drawer_id"},
		},
		s.toolUpdateDrawer)

	s.add("mempalace_diary_write",
		"Write a diary entry (stored as a drawer with type=diary_entry).",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"agent_name": {Type: "string", Description: "Agent / author name"},
				"entry":      {Type: "string", Description: "Diary entry content"},
				"topic":      {Type: "string", Description: "Topic tag", Default: "general"},
			},
			Required: []string{"agent_name", "entry"},
		},
		s.toolDiaryWrite)

	s.add("mempalace_diary_read",
		"Read recent diary entries for an agent.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"agent_name": {Type: "string", Description: "Agent / author name"},
				"last_n":     {Type: "integer", Description: "Number of recent entries", Default: 10, Minimum: intPtr(1), Maximum: intPtr(100)},
			},
			Required: []string{"agent_name"},
		},
		s.toolDiaryRead)

	s.add("mempalace_reconnect",
		"Reconnect to the palace database (no-op for pgx pool — auto-reconnects).",
		inputSchema{Type: "object"},
		s.toolReconnect)
}

func (s *Server) add(name, desc string, schema inputSchema, fn func(map[string]any) (any, error)) {
	s.tools[name] = toolDef{Name: name, Description: desc, InputSchema: schema}
	s.router[name] = fn
}

// ---------------------------------------------------------------------------
// Tool implementations
// ---------------------------------------------------------------------------

func (s *Server) toolStatus(args map[string]any) (any, error) {
	ctx := reqCtx()
	tree, total, err := s.col.WingRoomCounts(ctx)
	if err != nil {
		return nil, err
	}

	wings := map[string]int{}
	rooms := map[string]int{}
	for wing, roomMap := range tree {
		for room, cnt := range roomMap {
			wings[wing] += cnt
			rooms[room] += cnt
		}
	}

	return map[string]any{
		"total_drawers": total,
		"wings":         wings,
		"rooms":         rooms,
	}, nil
}

func (s *Server) toolListWings(args map[string]any) (any, error) {
	ctx := reqCtx()
	tree, _, err := s.col.WingRoomCounts(ctx)
	if err != nil {
		return nil, err
	}

	wingCounts := map[string]int{}
	for wing, roomMap := range tree {
		for _, cnt := range roomMap {
			wingCounts[wing] += cnt
		}
	}

	type wingInfo struct {
		Wing    string `json:"wing"`
		Drawers int    `json:"drawers"`
	}
	list := make([]wingInfo, 0, len(wingCounts))
	for w, c := range wingCounts {
		list = append(list, wingInfo{Wing: w, Drawers: c})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Wing < list[j].Wing })
	return map[string]any{"wings": list}, nil
}

func (s *Server) toolListRooms(args map[string]any) (any, error) {
	ctx := reqCtx()
	wing, _ := args["wing"].(string)

	tree, _, err := s.col.WingRoomCounts(ctx)
	if err != nil {
		return nil, err
	}

	type roomInfo struct {
		Wing    string `json:"wing"`
		Room    string `json:"room"`
		Drawers int    `json:"drawers"`
	}
	var list []roomInfo
	for w, roomMap := range tree {
		if wing != "" && w != wing {
			continue
		}
		for room, cnt := range roomMap {
			list = append(list, roomInfo{Wing: w, Room: room, Drawers: cnt})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Wing != list[j].Wing {
			return list[i].Wing < list[j].Wing
		}
		return list[i].Room < list[j].Room
	})
	return map[string]any{"rooms": list}, nil
}

func (s *Server) toolGetTaxonomy(args map[string]any) (any, error) {
	ctx := reqCtx()
	tree, total, err := s.col.WingRoomCounts(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"total": total, "taxonomy": tree}, nil
}

func (s *Server) toolSearch(args map[string]any) (any, error) {
	ctx := reqCtx()

	rawQuery, _ := args["query"].(string)
	if rawQuery == "" {
		return nil, fmt.Errorf("query is required")
	}
	embedQuery := rawQuery
	if ctxStr, ok := args["context"].(string); ok && ctxStr != "" {
		embedQuery = ctxStr + "\n" + rawQuery
	}

	limit := intArg(args, "limit", 5)
	if limit < 1 || limit > 100 {
		limit = 5
	}
	maxDist := floatArg(args, "max_distance", 1.5)
	wing, _ := args["wing"].(string)
	room, _ := args["room"].(string)

	vec, err := s.embed.EmbedOne(ctx, embedQuery)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	where := buildWhere(wing, room)
	// Hybrid: vector similarity + full-text search (RRF merge)
	drawers, err := s.col.QueryHybrid(ctx, rawQuery, vec, where, limit)
	if err != nil {
		return nil, err
	}

	type hit struct {
		DrawerID   string         `json:"drawer_id"`
		Wing       string         `json:"wing"`
		Room       string         `json:"room"`
		Similarity float64        `json:"similarity"`
		Distance   float64        `json:"distance"`
		Content    string         `json:"content"`
		FiledAt    string         `json:"filed_at"`
		SourceFile string         `json:"source_file,omitempty"`
		Metadata   map[string]any `json:"metadata"`
	}

	results := make([]hit, 0, len(drawers))
	for _, d := range drawers {
		if maxDist > 0 && d.Distance > maxDist {
			continue
		}
		sim := 1.0 - d.Distance
		if sim < 0 {
			sim = 0
		}
		results = append(results, hit{
			DrawerID:   d.ID,
			Wing:       strMeta(d.Metadata, "wing"),
			Room:       strMeta(d.Metadata, "room"),
			Similarity: round3(sim),
			Distance:   round4(d.Distance),
			Content:    d.Document,
			FiledAt:    strMeta(d.Metadata, "filed_at"),
			SourceFile: strMeta(d.Metadata, "source_file"),
			Metadata:   d.Metadata,
		})
	}
	return map[string]any{"results": results, "count": len(results)}, nil
}

func (s *Server) toolCheckDuplicate(args map[string]any) (any, error) {
	ctx := reqCtx()

	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	threshold := floatArg(args, "threshold", 0.9)

	vec, err := s.embed.EmbedOne(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	drawers, err := s.col.Query(ctx, vec, nil, 5)
	if err != nil {
		return nil, err
	}

	type dup struct {
		DrawerID   string  `json:"drawer_id"`
		Wing       string  `json:"wing"`
		Room       string  `json:"room"`
		Similarity float64 `json:"similarity"`
		Preview    string  `json:"content"`
	}
	var dups []dup
	for _, d := range drawers {
		sim := 1.0 - d.Distance
		if sim >= threshold {
			preview := d.Document
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			dups = append(dups, dup{
				DrawerID:   d.ID,
				Wing:       strMeta(d.Metadata, "wing"),
				Room:       strMeta(d.Metadata, "room"),
				Similarity: round3(sim),
				Preview:    preview,
			})
		}
	}

	return map[string]any{
		"is_duplicate": len(dups) > 0,
		"duplicates":   dups,
	}, nil
}

func (s *Server) toolAddDrawer(args map[string]any) (any, error) {
	ctx := reqCtx()

	wing, _ := args["wing"].(string)
	room, _ := args["room"].(string)
	content, _ := args["content"].(string)
	sourceFile, _ := args["source_file"].(string)
	addedBy, _ := args["added_by"].(string)
	if addedBy == "" {
		addedBy = "mcp"
	}

	if wing == "" || room == "" || content == "" {
		return nil, fmt.Errorf("wing, room, and content are required")
	}

	// Split bullet-point lists into individual drawers for precise retrieval
	if bullets := splitBullets(content); len(bullets) > 0 {
		ids := make([]string, 0, len(bullets))
		for i, bullet := range bullets {
			prefix := bullet
			if len(prefix) > 500 {
				prefix = prefix[:500]
			}
			h := sha256.Sum256([]byte(wing + "/" + room + "/" + prefix))
			ids = append(ids, hex.EncodeToString(h[:])[:16])
			_ = i
		}
		stored := 0
		for i, bullet := range bullets {
			exists, err := s.col.Exists(ctx, ids[i])
			if err != nil {
				return nil, err
			}
			if exists {
				continue
			}
			vec, err := s.embed.EmbedOne(ctx, bullet)
			if err != nil {
				return nil, fmt.Errorf("embed bullet: %w", err)
			}
			meta := map[string]any{
				"wing":        wing,
				"room":        room,
				"chunk_index": i,
				"added_by":    addedBy,
				"filed_at":    time.Now().Format(time.RFC3339Nano),
			}
			if sourceFile != "" {
				meta["source_file"] = sourceFile
			}
			if err := s.col.Add(ctx, []string{ids[i]}, []string{bullet}, []map[string]any{meta}, [][]float32{vec}); err != nil {
				return nil, err
			}
			stored++
		}
		return map[string]any{
			"success":        true,
			"bullets_stored": stored,
			"bullets_total":  len(bullets),
			"wing":           wing,
			"room":           room,
		}, nil
	}

	// Deterministic ID: sha256(wing/room/content[:500])[:16]
	prefix := content
	if len(prefix) > 500 {
		prefix = prefix[:500]
	}
	h := sha256.Sum256([]byte(wing + "/" + room + "/" + prefix))
	drawerID := hex.EncodeToString(h[:])[:16]

	// Idempotency check
	exists, err := s.col.Exists(ctx, drawerID)
	if err != nil {
		return nil, err
	}
	if exists {
		return map[string]any{
			"success":   true,
			"reason":    "already_exists",
			"drawer_id": drawerID,
		}, nil
	}

	vec, err := s.embed.EmbedOne(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embed content: %w", err)
	}

	meta := map[string]any{
		"wing":        wing,
		"room":        room,
		"chunk_index": 0,
		"added_by":    addedBy,
		"filed_at":    time.Now().Format(time.RFC3339Nano),
	}
	if sourceFile != "" {
		meta["source_file"] = sourceFile
	}

	if err := s.col.Add(ctx, []string{drawerID}, []string{content}, []map[string]any{meta}, [][]float32{vec}); err != nil {
		return nil, err
	}

	return map[string]any{
		"success":   true,
		"drawer_id": drawerID,
		"wing":      wing,
		"room":      room,
	}, nil
}

// splitBullets returns individual bullet items if the entire content is a bullet list (2+ items).
// Returns nil for mixed or non-list content.
func splitBullets(text string) []string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	var bullets []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		item, ok := cutBulletPrefix(trimmed)
		if !ok || strings.TrimSpace(item) == "" {
			return nil // non-bullet line found — not a pure list
		}
		bullets = append(bullets, strings.TrimSpace(item))
	}
	if len(bullets) < 2 {
		return nil
	}
	return bullets
}

func cutBulletPrefix(s string) (string, bool) {
	for _, p := range []string{"- ", "* ", "• "} {
		if strings.HasPrefix(s, p) {
			return s[len(p):], true
		}
	}
	// Numbered: "1. ", "12. "
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i < len(s)-1 && s[i] == '.' && s[i+1] == ' ' {
		return s[i+2:], true
	}
	return "", false
}

func (s *Server) toolDeleteDrawer(args map[string]any) (any, error) {
	ctx := reqCtx()

	id, _ := args["drawer_id"].(string)
	if id == "" {
		return nil, fmt.Errorf("drawer_id is required")
	}

	drawers, err := s.col.GetByIDs(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if len(drawers) == 0 {
		return map[string]any{"success": false, "error": "drawer not found: " + id}, nil
	}

	if err := s.col.Delete(ctx, []string{id}); err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "drawer_id": id}, nil
}

func (s *Server) toolGetDrawer(args map[string]any) (any, error) {
	ctx := reqCtx()

	id, _ := args["drawer_id"].(string)
	if id == "" {
		return nil, fmt.Errorf("drawer_id is required")
	}

	drawers, err := s.col.GetByIDs(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if len(drawers) == 0 {
		return map[string]any{"error": "drawer not found: " + id}, nil
	}
	d := drawers[0]
	return map[string]any{
		"drawer_id": d.ID,
		"content":   d.Document,
		"wing":      strMeta(d.Metadata, "wing"),
		"room":      strMeta(d.Metadata, "room"),
		"metadata":  d.Metadata,
	}, nil
}

func (s *Server) toolListDrawers(args map[string]any) (any, error) {
	ctx := reqCtx()

	wing, _ := args["wing"].(string)
	room, _ := args["room"].(string)
	limit := intArg(args, "limit", 20)
	offset := intArg(args, "offset", 0)
	if limit < 1 || limit > 100 {
		limit = 20
	}

	where := buildWhere(wing, room)
	drawers, err := s.col.GetWhere(ctx, where, limit, offset)
	if err != nil {
		return nil, err
	}

	type item struct {
		DrawerID string `json:"drawer_id"`
		Wing     string `json:"wing"`
		Room     string `json:"room"`
		Preview  string `json:"content_preview"`
		FiledAt  string `json:"filed_at"`
	}
	list := make([]item, len(drawers))
	for i, d := range drawers {
		preview := d.Document
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		list[i] = item{
			DrawerID: d.ID,
			Wing:     strMeta(d.Metadata, "wing"),
			Room:     strMeta(d.Metadata, "room"),
			Preview:  preview,
			FiledAt:  strMeta(d.Metadata, "filed_at"),
		}
	}
	return map[string]any{"drawers": list, "count": len(list), "offset": offset}, nil
}

func (s *Server) toolUpdateDrawer(args map[string]any) (any, error) {
	ctx := reqCtx()

	id, _ := args["drawer_id"].(string)
	if id == "" {
		return nil, fmt.Errorf("drawer_id is required")
	}

	newContent, _ := args["content"].(string)
	newWing, _ := args["wing"].(string)
	newRoom, _ := args["room"].(string)

	// Fetch current state
	drawers, err := s.col.GetByIDs(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if len(drawers) == 0 {
		return map[string]any{"success": false, "error": "drawer not found: " + id}, nil
	}
	cur := drawers[0]

	// Merge metadata
	newMeta := copyMeta(cur.Metadata)
	if newWing != "" {
		newMeta["wing"] = newWing
	}
	if newRoom != "" {
		newMeta["room"] = newRoom
	}
	newMeta["updated_at"] = time.Now().Format(time.RFC3339Nano)

	// Re-embed only if content changed
	var newEmb []float32
	doc := newContent
	if doc == "" {
		doc = cur.Document // keep existing — no re-embedding needed
	} else {
		newEmb, err = s.embed.EmbedOne(ctx, doc)
		if err != nil {
			return nil, fmt.Errorf("embed: %w", err)
		}
	}

	if err := s.col.UpdateOne(ctx, id, newContent, newMeta, newEmb); err != nil {
		return nil, err
	}
	return map[string]any{
		"success":   true,
		"drawer_id": id,
		"wing":      newMeta["wing"],
		"room":      newMeta["room"],
	}, nil
}

func (s *Server) toolDiaryWrite(args map[string]any) (any, error) {
	ctx := reqCtx()

	agent, _ := args["agent_name"].(string)
	entry, _ := args["entry"].(string)
	topic, _ := args["topic"].(string)
	if agent == "" || entry == "" {
		return nil, fmt.Errorf("agent_name and entry are required")
	}
	if topic == "" {
		topic = "general"
	}

	now := time.Now()
	dateStr := now.Format("2006-01-02")
	wing := agent
	room := "diary"

	prefix := entry
	if len(prefix) > 500 {
		prefix = prefix[:500]
	}
	h := sha256.Sum256([]byte(wing + "/" + room + "/" + prefix))
	drawerID := hex.EncodeToString(h[:])[:16]

	vec, err := s.embed.EmbedOne(ctx, entry)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	meta := map[string]any{
		"wing":        wing,
		"room":        room,
		"type":        "diary_entry",
		"hall":        "hall_diary",
		"agent":       agent,
		"topic":       topic,
		"date":        dateStr,
		"added_by":    "mcp",
		"filed_at":    now.Format(time.RFC3339Nano),
		"chunk_index": 0,
	}

	if err := s.col.Upsert(ctx, []string{drawerID}, []string{entry}, []map[string]any{meta}, [][]float32{vec}); err != nil {
		return nil, err
	}
	return map[string]any{
		"success":   true,
		"drawer_id": drawerID,
		"agent":     agent,
		"topic":     topic,
		"date":      dateStr,
	}, nil
}

func (s *Server) toolDiaryRead(args map[string]any) (any, error) {
	ctx := reqCtx()

	agent, _ := args["agent_name"].(string)
	if agent == "" {
		return nil, fmt.Errorf("agent_name is required")
	}
	lastN := intArg(args, "last_n", 10)
	if lastN < 1 || lastN > 100 {
		lastN = 10
	}

	where := map[string]any{
		"$and": []any{
			map[string]any{"agent": agent},
			map[string]any{"type": "diary_entry"},
		},
	}
	drawers, err := s.col.GetWhere(ctx, where, lastN, 0)
	if err != nil {
		return nil, err
	}

	// Sort newest first by filed_at
	sort.Slice(drawers, func(i, j int) bool {
		return strMeta(drawers[i].Metadata, "filed_at") > strMeta(drawers[j].Metadata, "filed_at")
	})

	type entry struct {
		Date      string `json:"date"`
		Timestamp string `json:"timestamp"`
		Topic     string `json:"topic"`
		Content   string `json:"content"`
	}
	entries := make([]entry, len(drawers))
	for i, d := range drawers {
		entries[i] = entry{
			Date:      strMeta(d.Metadata, "date"),
			Timestamp: strMeta(d.Metadata, "filed_at"),
			Topic:     strMeta(d.Metadata, "topic"),
			Content:   d.Document,
		}
	}
	return map[string]any{"agent": agent, "entries": entries, "count": len(entries)}, nil
}

func (s *Server) toolReconnect(_ map[string]any) (any, error) {
	ctx := reqCtx()
	count, err := s.col.Count(ctx)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{
		"success": true,
		"message": "pgx pool is healthy — connections are managed automatically",
		"drawers": count,
	}, nil
}

// ---------------------------------------------------------------------------
// Argument helpers
// ---------------------------------------------------------------------------

func intArg(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
	}
	return def
}

func floatArg(args map[string]any, key string, def float64) float64 {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f
		}
	}
	return def
}

func strMeta(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// boolArgPtr returns a pointer to the bool at key, or nil if absent.
// Lets callers distinguish "not provided" from an explicit false.
func boolArgPtr(args map[string]any, key string) *bool {
	v, ok := args[key]
	if !ok {
		return nil
	}
	if b, ok := v.(bool); ok {
		return &b
	}
	return nil
}

func buildWhere(wing, room string) map[string]any {
	if wing == "" && room == "" {
		return nil
	}
	if wing != "" && room == "" {
		return map[string]any{"wing": wing}
	}
	if wing == "" && room != "" {
		return map[string]any{"room": room}
	}
	return map[string]any{
		"$and": []any{
			map[string]any{"wing": wing},
			map[string]any{"room": room},
		},
	}
}

func copyMeta(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }
func round4(f float64) float64 { return float64(int(f*10000+0.5)) / 10000 }
