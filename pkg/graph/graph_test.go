package graph_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/graph"
)

func TestGraph(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Graph Suite")
}

var _ = Describe("ConceptGraph", func() {
	var g *graph.ConceptGraph
	now := time.Now()

	BeforeEach(func() {
		g = graph.NewConceptGraph()
	})

	Describe("AddConcept", func() {
		It("should add a new concept", func() {
			c := &graph.Concept{
				ID:    "person:alice",
				Type:  graph.PersonType,
				Label: "alice",
				Provenance: graph.Provenance{
					Source:  "test",
					Created: now,
					Modified: now,
					Count:   1,
				},
			}
			g.AddConcept(c)
			Expect(g.GetConcept("person:alice")).ToNot(BeNil())
			Expect(g.GetConcept("person:alice").Label).To(Equal("alice"))
		})

		It("should merge duplicate concepts by incrementing count", func() {
			c1 := &graph.Concept{
				ID:    "person:alice",
				Type:  graph.PersonType,
				Label: "alice",
				Provenance: graph.Provenance{Count: 1, Created: now, Modified: now},
			}
			c2 := &graph.Concept{
				ID:    "person:alice",
				Type:  graph.PersonType,
				Label: "alice",
				Provenance: graph.Provenance{Count: 3, Created: now, Modified: now},
			}
			g.AddConcept(c1)
			g.AddConcept(c2)
			Expect(g.GetConcept("person:alice").Provenance.Count).To(Equal(4))
		})

		It("should merge aliases without duplicates", func() {
			c1 := &graph.Concept{
				ID:      "person:alice",
				Type:    graph.PersonType,
				Label:   "alice",
				Aliases: []string{"al"},
				Provenance: graph.Provenance{Count: 1, Created: now, Modified: now},
			}
			c2 := &graph.Concept{
				ID:      "person:alice",
				Type:    graph.PersonType,
				Label:   "alice",
				Aliases: []string{"al", "ally"},
				Provenance: graph.Provenance{Count: 1, Created: now, Modified: now},
			}
			g.AddConcept(c1)
			g.AddConcept(c2)
			Expect(g.GetConcept("person:alice").Aliases).To(ConsistOf("al", "ally"))
		})
	})

	Describe("AddEdge", func() {
		It("should add an edge and update adjacency lists", func() {
			g.AddConcept(&graph.Concept{ID: "person:alice", Type: graph.PersonType, Label: "alice", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddConcept(&graph.Concept{ID: "topic:go", Type: graph.TopicType, Label: "go", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})

			e := &graph.Edge{
				ID:      graph.EdgeKey("person:alice", "topic:go", graph.Discusses),
				Source:  "person:alice",
				Target:  "topic:go",
				Type:    graph.Discusses,
				Weight:  1,
				Created: now,
				Modified: now,
			}
			g.AddEdge(e)

			neighbors := g.GetNeighbors("person:alice")
			Expect(neighbors).To(HaveLen(1))
			Expect(neighbors[0].ID).To(Equal("topic:go"))
		})

		It("should increment weight on duplicate edges", func() {
			eid := graph.EdgeKey("a", "b", graph.CoOccurs)
			g.AddEdge(&graph.Edge{ID: eid, Source: "a", Target: "b", Type: graph.CoOccurs, Weight: 1, Created: now, Modified: now})
			g.AddEdge(&graph.Edge{ID: eid, Source: "a", Target: "b", Type: graph.CoOccurs, Weight: 2, Created: now, Modified: now})
			Expect(g.GetEdge(eid).Weight).To(Equal(float64(3)))
		})
	})

	Describe("GetNeighbors", func() {
		It("should return both outgoing and incoming neighbors", func() {
			g.AddConcept(&graph.Concept{ID: "a", Type: graph.PersonType, Label: "a", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddConcept(&graph.Concept{ID: "b", Type: graph.TopicType, Label: "b", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddConcept(&graph.Concept{ID: "c", Type: graph.TopicType, Label: "c", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})

			g.AddEdge(&graph.Edge{ID: "e1", Source: "a", Target: "b", Type: graph.Discusses, Weight: 1, Created: now, Modified: now})
			g.AddEdge(&graph.Edge{ID: "e2", Source: "c", Target: "a", Type: graph.Mentions, Weight: 1, Created: now, Modified: now})

			neighbors := g.GetNeighbors("a")
			Expect(neighbors).To(HaveLen(2))
		})
	})

	Describe("GetByType", func() {
		It("should filter concepts by type", func() {
			g.AddConcept(&graph.Concept{ID: "person:alice", Type: graph.PersonType, Label: "alice", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddConcept(&graph.Concept{ID: "topic:go", Type: graph.TopicType, Label: "go", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddConcept(&graph.Concept{ID: "person:bob", Type: graph.PersonType, Label: "bob", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})

			people := g.GetByType(graph.PersonType)
			Expect(people).To(HaveLen(2))

			topics := g.GetByType(graph.TopicType)
			Expect(topics).To(HaveLen(1))
		})
	})

	Describe("SubgraphFor", func() {
		It("should return a BFS subgraph within depth limit", func() {
			g.AddConcept(&graph.Concept{ID: "a", Type: graph.PersonType, Label: "a", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddConcept(&graph.Concept{ID: "b", Type: graph.TopicType, Label: "b", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddConcept(&graph.Concept{ID: "c", Type: graph.TopicType, Label: "c", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddConcept(&graph.Concept{ID: "d", Type: graph.TopicType, Label: "d", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})

			g.AddEdge(&graph.Edge{ID: "e1", Source: "a", Target: "b", Type: graph.Discusses, Weight: 1, Created: now, Modified: now})
			g.AddEdge(&graph.Edge{ID: "e2", Source: "b", Target: "c", Type: graph.CoOccurs, Weight: 1, Created: now, Modified: now})
			g.AddEdge(&graph.Edge{ID: "e3", Source: "c", Target: "d", Type: graph.CoOccurs, Weight: 1, Created: now, Modified: now})

			// Depth 1 from a: should get a, b
			concepts, edges := g.SubgraphFor("a", 1)
			Expect(concepts).To(HaveLen(2))
			Expect(edges).To(HaveLen(1))

			// Depth 2 from a: should get a, b, c
			concepts2, edges2 := g.SubgraphFor("a", 2)
			Expect(concepts2).To(HaveLen(3))
			Expect(edges2).To(HaveLen(2))
		})
	})

	Describe("Stats", func() {
		It("should return correct counts", func() {
			g.AddConcept(&graph.Concept{ID: "person:alice", Type: graph.PersonType, Label: "alice", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddConcept(&graph.Concept{ID: "topic:go", Type: graph.TopicType, Label: "go", Provenance: graph.Provenance{Count: 1, Created: now, Modified: now}})
			g.AddEdge(&graph.Edge{ID: "e1", Source: "person:alice", Target: "topic:go", Type: graph.Discusses, Weight: 1, Created: now, Modified: now})

			stats := g.Stats()
			Expect(stats.NumConcepts).To(Equal(2))
			Expect(stats.NumEdges).To(Equal(1))
			Expect(stats.ConceptCounts["person"]).To(Equal(1))
			Expect(stats.ConceptCounts["topic"]).To(Equal(1))
			Expect(stats.EdgeCounts["discusses"]).To(Equal(1))
		})
	})

	Describe("EdgeKey", func() {
		It("should produce deterministic keys", func() {
			k := graph.EdgeKey("person:alice", "topic:go", graph.Discusses)
			Expect(k).To(Equal("person:alice-[discusses]->topic:go"))
		})
	})
})
