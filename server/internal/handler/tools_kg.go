package handler

import (
	"fmt"
)

// registerKGTools wires the six Knowledge-Graph MCP tools.
func (s *Server) registerKGTools() {
	s.add("mempalace_kg_add_entity",
		"Add or update a knowledge-graph entity (MERGE by name).",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"name":        {Type: "string", Description: "Unique entity name"},
				"entity_type": {Type: "string", Description: "Entity type / label (e.g. Person, Project, Concept)"},
				"description": {Type: "string", Description: "Short description"},
			},
			Required: []string{"name"},
		},
		s.toolKGAddEntity)

	s.add("mempalace_kg_add_relation",
		"Add a directed relationship between two existing entities.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"from_entity":   {Type: "string", Description: "Source entity name"},
				"relation_type": {Type: "string", Description: "Relation type in UPPER_SNAKE_CASE (e.g. KNOWS, WORKS_ON, PART_OF)"},
				"to_entity":     {Type: "string", Description: "Target entity name"},
			},
			Required: []string{"from_entity", "relation_type", "to_entity"},
		},
		s.toolKGAddRelation)

	s.add("mempalace_kg_get_entity",
		"Fetch an entity and all its direct relationships.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"name": {Type: "string", Description: "Entity name"},
			},
			Required: []string{"name"},
		},
		s.toolKGGetEntity)

	s.add("mempalace_kg_search_entities",
		"Search entities by name substring, optionally filtered by type.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"query":       {Type: "string", Description: "Name substring to search"},
				"entity_type": {Type: "string", Description: "Optional type filter (e.g. Person)"},
				"limit":       {Type: "integer", Description: "Max results (1-100)", Default: 10, Minimum: intPtr(1), Maximum: intPtr(100)},
			},
			Required: []string{"query"},
		},
		s.toolKGSearchEntities)

	s.add("mempalace_kg_delete_entity",
		"Delete an entity and all its relationships from the knowledge graph.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"name": {Type: "string", Description: "Entity name to delete"},
			},
			Required: []string{"name"},
		},
		s.toolKGDeleteEntity)

	s.add("mempalace_kg_traverse",
		"Traverse the graph from a starting entity up to a given depth.",
		inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"start_entity": {Type: "string", Description: "Starting entity name"},
				"depth":        {Type: "integer", Description: "Max hop depth (1-3)", Default: 2, Minimum: intPtr(1), Maximum: intPtr(3)},
			},
			Required: []string{"start_entity"},
		},
		s.toolKGTraverse)
}

// ---------------------------------------------------------------------------
// Tool implementations
// ---------------------------------------------------------------------------

func (s *Server) toolKGAddEntity(args map[string]any) (any, error) {
	if s.graph == nil {
		return nil, fmt.Errorf("knowledge graph not available (AGE not installed)")
	}
	name, _ := args["name"].(string)
	entityType, _ := args["entity_type"].(string)
	description, _ := args["description"].(string)

	entity, err := s.graph.AddEntity(reqCtx(), name, entityType, description)
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "entity": entity}, nil
}

func (s *Server) toolKGAddRelation(args map[string]any) (any, error) {
	if s.graph == nil {
		return nil, fmt.Errorf("knowledge graph not available (AGE not installed)")
	}
	from, _ := args["from_entity"].(string)
	relType, _ := args["relation_type"].(string)
	to, _ := args["to_entity"].(string)

	if err := s.graph.AddRelation(reqCtx(), from, relType, to); err != nil {
		return nil, err
	}
	return map[string]any{
		"success": true,
		"from":    from,
		"type":    relType,
		"to":      to,
	}, nil
}

func (s *Server) toolKGGetEntity(args map[string]any) (any, error) {
	if s.graph == nil {
		return nil, fmt.Errorf("knowledge graph not available (AGE not installed)")
	}
	name, _ := args["name"].(string)

	entity, rels, err := s.graph.GetEntity(reqCtx(), name)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return map[string]any{"found": false, "name": name}, nil
	}
	return map[string]any{
		"found":          true,
		"entity":         entity,
		"relations":      rels,
		"relation_count": len(rels),
	}, nil
}

func (s *Server) toolKGSearchEntities(args map[string]any) (any, error) {
	if s.graph == nil {
		return nil, fmt.Errorf("knowledge graph not available (AGE not installed)")
	}
	query, _ := args["query"].(string)
	entityType, _ := args["entity_type"].(string)
	limit := intArg(args, "limit", 10)

	entities, err := s.graph.SearchEntities(reqCtx(), query, entityType, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"entities": entities, "count": len(entities)}, nil
}

func (s *Server) toolKGDeleteEntity(args map[string]any) (any, error) {
	if s.graph == nil {
		return nil, fmt.Errorf("knowledge graph not available (AGE not installed)")
	}
	name, _ := args["name"].(string)

	found, err := s.graph.DeleteEntity(reqCtx(), name)
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": found, "name": name}, nil
}

func (s *Server) toolKGTraverse(args map[string]any) (any, error) {
	if s.graph == nil {
		return nil, fmt.Errorf("knowledge graph not available (AGE not installed)")
	}
	start, _ := args["start_entity"].(string)
	depth := intArg(args, "depth", 2)

	entities, rels, err := s.graph.Traverse(reqCtx(), start, depth)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"start":          start,
		"depth":          depth,
		"entities":       entities,
		"relations":      rels,
		"entity_count":   len(entities),
		"relation_count": len(rels),
	}, nil
}
