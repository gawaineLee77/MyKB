package graphview

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// Only run against a dedicated disposable test database. The fixture uses its
// own labels and cleanup never touches any other graph.
func TestNeo4jIntegration(t *testing.T) {
	uri := os.Getenv("GRAPHVIEW_TEST_URI")
	if uri == "" {
		t.Skip("set GRAPHVIEW_TEST_URI for disposable Neo4j")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	driver, err := neo4j.NewDriver(uri, neo4j.BasicAuth("neo4j", "synthetic-graphview-test", ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = driver.Close(context.Background()) })
	seed := `CREATE (a:ENTITYgraphview_fixture {name:'Astra',kg:'doc-a',attributes:['service']})
CREATE (b:ENTITYgraphview_fixture {name:'Helios',kg:'doc-a'}), (c:ENTITYgraphview_other {name:'Private',kg:'doc-other'})
CREATE (a)-[:DEPENDS_ON]->(b), (a)-[:HIDDEN]->(c)
WITH a UNWIND range(1,170) AS i CREATE (n:ENTITYgraphview_fixture {name:'Node '+toString(i),kg:'doc-a'}) CREATE (a)-[:USES]->(n)`
	_, err = neo4j.ExecuteQuery(ctx, driver, `MATCH (n) WHERE n:ENTITYgraphview_fixture OR n:ENTITYgraphview_other DETACH DELETE n`, nil, neo4j.EagerResultTransformer)
	if err != nil {
		t.Fatal(err)
	}
	_, err = neo4j.ExecuteQuery(ctx, driver, seed, nil, neo4j.EagerResultTransformer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = neo4j.ExecuteQuery(context.Background(), driver, `MATCH (n) WHERE n:ENTITYgraphview_fixture OR n:ENTITYgraphview_other DETACH DELETE n`, nil, neo4j.EagerResultTransformer)
	})
	reader := &Neo4j{Driver: driver, Database: "neo4j"}
	g, err := reader.Read(ctx, Query{KBID: "graphview-fixture", Limit: 80})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 80 || !g.Truncated {
		t.Fatalf("initial not bounded: %d %+v", len(g.Nodes), g.Truncated)
	}
	var root string
	for _, n := range g.Nodes {
		if n.Name == "Astra" {
			root = n.ID
		}
		if n.Name == "Private" {
			t.Fatal("foreign node exposed")
		}
	}
	g, err = reader.Read(ctx, Query{KBID: "graphview-fixture", Relation: "DEPENDS_ON", Limit: 80})
	if err != nil || len(g.Nodes) != 2 || len(g.Edges) != 1 {
		t.Fatalf("filter: %+v %v", g, err)
	}
	g, err = reader.Read(ctx, Query{KBID: "graphview-fixture", NodeID: root, Limit: 60})
	if err != nil || len(g.Nodes) != 61 || !g.Truncated {
		t.Fatalf("expansion: %d %v", len(g.Nodes), err)
	}
	for _, e := range g.Edges {
		if e.Type == "HIDDEN" {
			t.Fatal("foreign edge exposed")
		}
	}
	g, err = reader.Read(ctx, Query{KBID: "graphview-other", NodeID: root, Limit: 60})
	if err != nil || len(g.Nodes) != 0 {
		t.Fatalf("foreign root exposed: %v", err)
	}
	g, err = reader.Read(ctx, Query{KBID: "graphview-fixture", Search: "helios", Limit: 80})
	if err != nil || len(g.Nodes) != 1 {
		t.Fatalf("search: %+v %v", g, err)
	}
	g, err = reader.Read(ctx, Query{KBID: "graphview-fixture", Search: "' MATCH (n) DETACH DELETE n //", Limit: 80})
	if err != nil || len(g.Nodes) != 0 {
		t.Fatalf("search is not parameterized: %v", err)
	}
	if _, err = reader.Read(ctx, Query{KBID: "x`) MATCH (n)", Limit: 80}); err == nil {
		t.Fatal("invalid label accepted")
	}
}
