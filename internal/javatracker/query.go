package javatracker

import (
	"fmt"
	"sort"
	"strings"
)

type QueryOptions struct {
	Direction   Direction
	Depth       int
	Limit       int
	IncludeBody bool
}

func (p *Project) Search(query string, limit int) []SearchItem {
	query = strings.TrimSpace(strings.ToLower(query))
	if limit <= 0 {
		limit = 50
	}
	if query == "" {
		if len(p.SearchItems) < limit {
			limit = len(p.SearchItems)
		}
		return append([]SearchItem(nil), p.SearchItems[:limit]...)
	}

	results := make([]SearchItem, 0, limit)
	for _, item := range p.SearchItems {
		if matchesSearchItem(item, query) {
			results = append(results, item)
			if len(results) >= limit {
				break
			}
		}
	}
	return results
}

func matchesSearchItem(item SearchItem, query string) bool {
	if strings.Contains(strings.ToLower(item.Label), query) {
		return true
	}
	if strings.Contains(strings.ToLower(item.Description), query) {
		return true
	}
	if strings.Contains(strings.ToLower(item.Signature), query) {
		return true
	}
	if strings.Contains(strings.ToLower(item.FilePath), query) {
		return true
	}
	return false
}

func (p *Project) Graph(nodeID string, options QueryOptions) (GraphResponse, error) {
	if options.Depth <= 0 {
		options.Depth = 2
	}
	if options.Limit <= 0 {
		options.Limit = 160
	}
	if options.Direction == "" {
		options.Direction = DirectionBoth
	}

	if _, ok := p.SearchByID[nodeID]; !ok && p.lookupNodeKind(nodeID) == "" {
		return GraphResponse{}, fmt.Errorf("unknown node: %s", nodeID)
	}

	resp := GraphResponse{
		ProjectRoot: p.Root,
		FocusID:     nodeID,
		Direction:   options.Direction,
		Depth:       options.Depth,
		Nodes:       make([]GraphNode, 0, options.Limit),
		Edges:       make([]GraphEdge, 0, options.Limit*2),
	}

	nodeSeen := make(map[string]bool, options.Limit)
	edgeSeen := make(map[string]bool, options.Limit*2)
	type item struct {
		id    string
		depth int
		lane  string
	}
	queue := []item{{id: nodeID, depth: 0, lane: "focus"}}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if nodeSeen[curr.id] {
			continue
		}
		if len(resp.Nodes) >= options.Limit {
			resp.Truncated = true
			break
		}

		nodeSeen[curr.id] = true
		resp.Nodes = append(resp.Nodes, p.makeGraphNode(curr.id, curr.lane, curr.depth))

		if curr.depth >= options.Depth {
			continue
		}

		if options.Direction == DirectionBoth || options.Direction == DirectionDownstream {
			for _, edge := range p.Outgoing[curr.id] {
				key := edgeKey(edge)
				if !edgeSeen[key] {
					resp.Edges = append(resp.Edges, edge)
					edgeSeen[key] = true
				}
				if !nodeSeen[edge.Target] {
					queue = append(queue, item{id: edge.Target, depth: curr.depth + 1, lane: "downstream"})
				}
			}
		}

		if options.Direction == DirectionBoth || options.Direction == DirectionUpstream {
			for _, edge := range p.Incoming[curr.id] {
				key := edgeKey(edge)
				if !edgeSeen[key] {
					resp.Edges = append(resp.Edges, edge)
					edgeSeen[key] = true
				}
				if !nodeSeen[edge.Source] {
					queue = append(queue, item{id: edge.Source, depth: curr.depth + 1, lane: "upstream"})
				}
			}
		}
	}

	if options.IncludeBody && p.lookupNodeKind(nodeID) == NodeMethod {
		bodyNodes, bodyEdges := p.methodBodyGraph(nodeID)
		for _, bodyNode := range bodyNodes {
			if nodeSeen[bodyNode.ID] || len(resp.Nodes) >= options.Limit {
				if len(resp.Nodes) >= options.Limit {
					resp.Truncated = true
				}
				continue
			}
			nodeSeen[bodyNode.ID] = true
			resp.Nodes = append(resp.Nodes, bodyNode)
		}
		for _, edge := range bodyEdges {
			key := edgeKey(edge)
			if !edgeSeen[key] {
				resp.Edges = append(resp.Edges, edge)
				edgeSeen[key] = true
			}
		}
	}

	sort.Slice(resp.Nodes, func(i, j int) bool {
		if resp.Nodes[i].Depth != resp.Nodes[j].Depth {
			return resp.Nodes[i].Depth < resp.Nodes[j].Depth
		}
		if resp.Nodes[i].Lane != resp.Nodes[j].Lane {
			return resp.Nodes[i].Lane < resp.Nodes[j].Lane
		}
		return resp.Nodes[i].Label < resp.Nodes[j].Label
	})
	sort.Slice(resp.Edges, func(i, j int) bool {
		if resp.Edges[i].Kind != resp.Edges[j].Kind {
			return resp.Edges[i].Kind < resp.Edges[j].Kind
		}
		if resp.Edges[i].Source != resp.Edges[j].Source {
			return resp.Edges[i].Source < resp.Edges[j].Source
		}
		return resp.Edges[i].Target < resp.Edges[j].Target
	})

	for _, node := range resp.Nodes {
		switch node.Lane {
		case "upstream":
			resp.Summary.UpstreamCount++
		case "downstream":
			resp.Summary.DownstreamCount++
		case "body":
			resp.Summary.BodyCount++
		}
	}
	resp.Summary.NodeCount = len(resp.Nodes)
	resp.Summary.EdgeCount = len(resp.Edges)

	return resp, nil
}

func (p *Project) Details(nodeID string) (NodeDetails, error) {
	switch {
	case p.Classes[nodeID] != nil:
		cls := p.Classes[nodeID]
		source, _ := p.FileCache.Slice(cls.FilePath, cls.StartLine, minNonZero(cls.EndLine, cls.StartLine+80))
		return NodeDetails{
			ID:          cls.ID,
			Label:       cls.FullName,
			Kind:        cls.Kind,
			FilePath:    cls.FilePath,
			StartLine:   cls.StartLine,
			EndLine:     cls.EndLine,
			Description: cls.Description,
			Meta: map[string]any{
				"package":    cls.Package,
				"extends":    cls.Extends,
				"implements": cls.Implements,
				"methods":    len(cls.MethodIDs),
				"fields":     len(cls.FieldIDs),
			},
			Source: source,
		}, nil
	case p.Methods[nodeID] != nil:
		method := p.Methods[nodeID]
		source, _ := p.FileCache.Slice(method.FilePath, method.StartLine, method.EndLine)
		return NodeDetails{
			ID:          method.ID,
			Label:       method.ClassName + "." + method.Name,
			Kind:        NodeMethod,
			FilePath:    method.FilePath,
			StartLine:   method.StartLine,
			EndLine:     method.EndLine,
			Signature:   formatMethodSignature(method),
			Description: method.Description,
			Meta: map[string]any{
				"class_name":      method.ClassName,
				"parameters":      method.Parameters,
				"return_type":     method.ReturnType,
				"calls":           len(method.CallSites),
				"accessed_fields": method.AccessedFields,
			},
			Source: source,
		}, nil
	case p.Fields[nodeID] != nil:
		field := p.Fields[nodeID]
		source, _ := p.FileCache.Slice(field.FilePath, field.StartLine, field.StartLine+8)
		return NodeDetails{
			ID:          field.ID,
			Label:       field.ClassName + "." + field.Name,
			Kind:        NodeField,
			FilePath:    field.FilePath,
			StartLine:   field.StartLine,
			EndLine:     field.StartLine,
			Description: field.Type,
			Meta: map[string]any{
				"class_name": field.ClassName,
				"type":       field.Type,
			},
			Source: source,
		}, nil
	default:
		if strings.Contains(nodeID, "#body:") {
			return NodeDetails{
				ID:    nodeID,
				Label: nodeID,
				Kind:  NodeStatement,
			}, nil
		}
	}
	return NodeDetails{}, fmt.Errorf("unknown node: %s", nodeID)
}

func (p *Project) lookupNodeKind(id string) NodeKind {
	switch {
	case p.Classes[id] != nil:
		return p.Classes[id].Kind
	case p.Methods[id] != nil:
		return NodeMethod
	case p.Fields[id] != nil:
		return NodeField
	case strings.Contains(id, "#body:"):
		return NodeStatement
	default:
		return ""
	}
}

func (p *Project) makeGraphNode(id, lane string, depth int) GraphNode {
	switch {
	case p.Classes[id] != nil:
		cls := p.Classes[id]
		return GraphNode{
			ID:          cls.ID,
			Label:       cls.FullName,
			Kind:        cls.Kind,
			Lane:        lane,
			Depth:       depth,
			FilePath:    cls.FilePath,
			Line:        cls.StartLine,
			Description: cls.Description,
			Meta: map[string]any{
				"methods": len(cls.MethodIDs),
				"fields":  len(cls.FieldIDs),
			},
		}
	case p.Methods[id] != nil:
		method := p.Methods[id]
		return GraphNode{
			ID:          method.ID,
			Label:       method.ClassName + "." + method.Name,
			Kind:        NodeMethod,
			Lane:        lane,
			Depth:       depth,
			FilePath:    method.FilePath,
			Line:        method.StartLine,
			Signature:   formatMethodSignature(method),
			Description: method.Description,
			Meta: map[string]any{
				"param_count": method.ParamCount,
				"return_type": method.ReturnType,
			},
		}
	case p.Fields[id] != nil:
		field := p.Fields[id]
		return GraphNode{
			ID:          field.ID,
			Label:       field.ClassName + "." + field.Name,
			Kind:        NodeField,
			Lane:        lane,
			Depth:       depth,
			FilePath:    field.FilePath,
			Line:        field.StartLine,
			Description: field.Type,
		}
	default:
		return GraphNode{
			ID:    id,
			Label: id,
			Kind:  NodeStatement,
			Lane:  lane,
			Depth: depth,
		}
	}
}

func (p *Project) methodBodyGraph(methodID string) ([]GraphNode, []GraphEdge) {
	method := p.Methods[methodID]
	if method == nil {
		return nil, nil
	}
	lines, err := p.FileCache.Slice(method.FilePath, method.StartLine, method.EndLine)
	if err != nil {
		return nil, nil
	}

	nodes := make([]GraphNode, 0, len(lines))
	edges := make([]GraphEdge, 0, len(lines)*2)
	var previous string
	for _, line := range lines {
		text := strings.TrimSpace(stripInlineComments(line.Text))
		if text == "" || text == "{" || text == "}" {
			continue
		}

		kind := bodyNodeKind(text)
		nodeID := fmt.Sprintf("%s#body:%d", methodID, line.Number)
		label := text
		if len(label) > 80 {
			label = label[:77] + "..."
		}
		node := GraphNode{
			ID:          nodeID,
			Label:       label,
			Kind:        kind,
			Lane:        "body",
			Depth:       1,
			FilePath:    method.FilePath,
			Line:        line.Number,
			Description: text,
		}
		nodes = append(nodes, node)
		edges = append(edges, GraphEdge{Source: methodID, Target: nodeID, Kind: EdgeAST, Label: "body"})
		if previous != "" {
			edges = append(edges, GraphEdge{Source: previous, Target: nodeID, Kind: EdgeCFG, Label: "next"})
		}

		for name, typ := range method.LocalVarTypes {
			if wordBoundaryContains(text, name) {
				varNodeID := fmt.Sprintf("%s#var:%s", methodID, name)
				nodes = append(nodes, GraphNode{
					ID:          varNodeID,
					Label:       name,
					Kind:        NodeVariable,
					Lane:        "body",
					Depth:       1,
					FilePath:    method.FilePath,
					Line:        line.Number,
					Description: typ,
				})
				edges = append(edges, GraphEdge{Source: varNodeID, Target: nodeID, Kind: EdgePDG, Label: "data"})
			}
		}
		previous = nodeID
	}

	nodes = dedupeGraphNodes(nodes)
	edges = dedupeGraphEdges(edges)
	return nodes, edges
}

func bodyNodeKind(text string) NodeKind {
	switch {
	case strings.HasPrefix(text, "if ") || strings.HasPrefix(text, "if(") || strings.HasPrefix(text, "for ") || strings.HasPrefix(text, "for(") || strings.HasPrefix(text, "while ") || strings.HasPrefix(text, "while(") || strings.HasPrefix(text, "switch ") || strings.HasPrefix(text, "switch("):
		return NodeControl
	case strings.HasPrefix(text, "return"):
		return NodeReturn
	case reVarDecl.MatchString(text):
		return NodeVariable
	case strings.Contains(text, "(") && strings.Contains(text, ")"):
		return NodeExpression
	default:
		return NodeStatement
	}
}

func dedupeGraphNodes(input []GraphNode) []GraphNode {
	seen := make(map[string]bool, len(input))
	out := make([]GraphNode, 0, len(input))
	for _, node := range input {
		if seen[node.ID] {
			continue
		}
		seen[node.ID] = true
		out = append(out, node)
	}
	return out
}

func dedupeGraphEdges(input []GraphEdge) []GraphEdge {
	seen := make(map[string]bool, len(input))
	out := make([]GraphEdge, 0, len(input))
	for _, edge := range input {
		key := edgeKey(edge)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, edge)
	}
	return out
}

func edgeKey(edge GraphEdge) string {
	return string(edge.Kind) + "|" + edge.Source + "|" + edge.Target + "|" + edge.Label
}

func minNonZero(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}
