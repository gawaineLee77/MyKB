package graphview

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// Neo4j only executes fixed read queries. User strings are parameters; the
// validated KB label matches v0.8.0 NameSpace.Labels (UUID hyphens -> underscores).
type Neo4j struct {
	Driver   neo4j.Driver
	Database string
}

func (n *Neo4j) Read(ctx context.Context, q Query) (Graph, error) {
	if !identifier.MatchString(q.KBID) || q.Limit < 1 || q.Limit > 160 {
		return Graph{}, fmt.Errorf("invalid graph scope")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	label := "`ENTITY" + strings.ReplaceAll(q.KBID, "-", "_") + "`"
	params := map[string]any{"q": q.Search, "relation": q.Relation, "node": q.NodeID, "limit": q.Limit + 1}
	query := `MATCH (n:` + label + `)
WHERE ($q = '' OR toLower(n.name) CONTAINS toLower($q))
AND ($relation = '' OR EXISTS { MATCH (n)-[r]-(m:` + label + `) WHERE type(r) = $relation })
RETURN n ORDER BY n.name, elementId(n) LIMIT $limit`
	if q.NodeID != "" {
		query = `MATCH (root:` + label + `) WHERE elementId(root) = $node
MATCH (root)-[r]-(n:` + label + `) WHERE ($relation = '' OR type(r) = $relation)
RETURN DISTINCT n ORDER BY n.name, elementId(n) LIMIT $limit`
	}
	session := n.Driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead, DatabaseName: n.Database})
	defer session.Close(ctx)
	value, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		g := Graph{Nodes: []Node{}, Edges: []Edge{}}
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return g, err
		}
		ids := []string{}
		add := func(v neo4j.Node) {
			item := Node{ID: v.ElementId, Name: short(v.Props["name"], 200), KnowledgeID: short(v.Props["kg"], 128), Attributes: []string{}}
			if attrs, ok := v.Props["attributes"].([]any); ok {
				for _, a := range attrs {
					if len(item.Attributes) < 32 {
						item.Attributes = append(item.Attributes, short(a, 1000))
					}
				}
			}
			g.Nodes = append(g.Nodes, item)
			ids = append(ids, item.ID)
		}
		for result.Next(ctx) {
			if len(g.Nodes) == q.Limit {
				g.Truncated = true
				break
			}
			v, _ := result.Record().Get("n")
			add(v.(neo4j.Node))
		}
		if _, err := result.Consume(ctx); err != nil {
			return g, err
		}
		if q.NodeID != "" {
			root, err := tx.Run(ctx, `MATCH (n:`+label+`) WHERE elementId(n) = $node RETURN n`, params)
			if err != nil {
				return g, err
			}
			if root.Next(ctx) {
				v, _ := root.Record().Get("n")
				node := v.(neo4j.Node)
				found := false
				for _, id := range ids {
					if id == node.ElementId {
						found = true
					}
				}
				if !found {
					add(node)
				}
			}
			if _, err := root.Consume(ctx); err != nil {
				return g, err
			}
		}
		edges, err := tx.Run(ctx, `MATCH (a:`+label+`)-[r]->(b:`+label+`)
WHERE elementId(a) IN $ids AND elementId(b) IN $ids AND ($relation = '' OR type(r) = $relation)
RETURN r ORDER BY elementId(r) LIMIT 321`, map[string]any{"ids": ids, "relation": q.Relation})
		if err != nil {
			return g, err
		}
		for edges.Next(ctx) {
			if len(g.Edges) == 320 {
				g.Truncated = true
				break
			}
			v, _ := edges.Record().Get("r")
			e := v.(neo4j.Relationship)
			g.Edges = append(g.Edges, Edge{e.ElementId, e.StartElementId, e.EndElementId, e.Type})
		}
		if _, err := edges.Consume(ctx); err != nil {
			return g, err
		}
		return g, nil
	}, neo4j.WithTxTimeout(5*time.Second))
	if err != nil {
		return Graph{}, err
	}
	return value.(Graph), nil
}
func short(value any, max int) string {
	s, _ := value.(string)
	r := []rune(s)
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}
