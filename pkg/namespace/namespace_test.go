package namespace_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/graph"
	"github.com/gnomatix/enkente/pkg/namespace"
)

func TestNamespace(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Namespace Suite")
}

var _ = Describe("Namespace Registry", func() {
	var reg *namespace.Registry

	BeforeEach(func() {
		reg = namespace.NewRegistry()
	})

	Describe("CreateNamespace", func() {
		It("should create a namespace", func() {
			ns := reg.CreateNamespace(namespace.SessionDim, "session1", "")
			Expect(ns).NotTo(BeNil())
			Expect(ns.ID).To(Equal("session:session1"))
			Expect(ns.Dimension).To(Equal(namespace.SessionDim))
		})

		It("should return existing namespace on duplicate", func() {
			ns1 := reg.CreateNamespace(namespace.UserDim, "alice", "")
			ns2 := reg.CreateNamespace(namespace.UserDim, "alice", "")
			Expect(ns1.ID).To(Equal(ns2.ID))
		})

		It("should support hierarchical namespaces", func() {
			parent := reg.CreateNamespace(namespace.ProjectDim, "enkente", "")
			child := reg.CreateNamespace(namespace.SubjectDim, "graph_model", parent.ID)
			Expect(child.Parent).To(Equal(parent.ID))
		})
	})

	Describe("Bind and NamespaceConcepts", func() {
		It("should bind concept to namespace", func() {
			ns := reg.CreateNamespace(namespace.SessionDim, "s1", "")
			err := reg.Bind("topic:go", ns.ID, 1.0)
			Expect(err).NotTo(HaveOccurred())

			concepts := reg.NamespaceConcepts(ns.ID)
			Expect(concepts).To(ContainElement("topic:go"))
		})

		It("should return error for nonexistent namespace", func() {
			err := reg.Bind("topic:go", "nonexistent:ns", 1.0)
			Expect(err).To(HaveOccurred())
		})

		It("should strengthen binding on re-bind", func() {
			ns := reg.CreateNamespace(namespace.SessionDim, "s1", "")
			_ = reg.Bind("topic:go", ns.ID, 0.3)
			_ = reg.Bind("topic:go", ns.ID, 0.5)

			bindings := reg.ConceptNamespaces("topic:go")
			Expect(bindings).To(HaveLen(1))
			Expect(bindings[0].Strength).To(BeNumerically("~", 0.8, 0.01))
		})
	})

	Describe("Unbind", func() {
		It("should remove binding", func() {
			ns := reg.CreateNamespace(namespace.SessionDim, "s1", "")
			_ = reg.Bind("topic:go", ns.ID, 1.0)
			reg.Unbind("topic:go", ns.ID)

			concepts := reg.NamespaceConcepts(ns.ID)
			Expect(concepts).NotTo(ContainElement("topic:go"))
		})
	})

	Describe("ConceptNamespaces", func() {
		It("should return all namespaces for a concept", func() {
			ns1 := reg.CreateNamespace(namespace.SessionDim, "s1", "")
			ns2 := reg.CreateNamespace(namespace.UserDim, "alice", "")
			_ = reg.Bind("topic:go", ns1.ID, 1.0)
			_ = reg.Bind("topic:go", ns2.ID, 0.5)

			bindings := reg.ConceptNamespaces("topic:go")
			Expect(bindings).To(HaveLen(2))
		})
	})

	Describe("FilterByNamespace", func() {
		It("should return subgraph for a namespace", func() {
			g := graph.NewConceptGraph()
			now := time.Now()
			g.AddConcept(&graph.Concept{ID: "topic:go", Type: graph.TopicType, Label: "go", Provenance: graph.Provenance{Created: now}})
			g.AddConcept(&graph.Concept{ID: "topic:rust", Type: graph.TopicType, Label: "rust", Provenance: graph.Provenance{Created: now}})
			g.AddConcept(&graph.Concept{ID: "topic:python", Type: graph.TopicType, Label: "python", Provenance: graph.Provenance{Created: now}})
			g.AddEdge(&graph.Edge{ID: "e1", Source: "topic:go", Target: "topic:rust", Type: graph.CoOccurs, Weight: 1, Created: now, Modified: now})
			g.AddEdge(&graph.Edge{ID: "e2", Source: "topic:go", Target: "topic:python", Type: graph.CoOccurs, Weight: 1, Created: now, Modified: now})

			ns := reg.CreateNamespace(namespace.SubjectDim, "systems", "")
			_ = reg.Bind("topic:go", ns.ID, 1.0)
			_ = reg.Bind("topic:rust", ns.ID, 1.0)
			// python NOT bound to this namespace

			concepts, edges := reg.FilterByNamespace(g, ns.ID)
			Expect(concepts).To(HaveLen(2))
			// Only edge e1 (go->rust) has both endpoints in the namespace
			Expect(edges).To(HaveLen(1))
			Expect(edges[0].ID).To(Equal("e1"))
		})
	})

	Describe("ListNamespaces", func() {
		It("should filter by dimension", func() {
			reg.CreateNamespace(namespace.SessionDim, "s1", "")
			reg.CreateNamespace(namespace.UserDim, "alice", "")
			reg.CreateNamespace(namespace.SessionDim, "s2", "")

			sessions := reg.ListNamespaces(namespace.SessionDim)
			Expect(sessions).To(HaveLen(2))

			users := reg.ListNamespaces(namespace.UserDim)
			Expect(users).To(HaveLen(1))

			all := reg.ListNamespaces("")
			Expect(all).To(HaveLen(3))
		})
	})

	Describe("AutoBindFromExtraction", func() {
		It("should auto-bind concepts based on provenance", func() {
			concepts := []*graph.Concept{
				{
					ID:    "topic:go",
					Type:  graph.TopicType,
					Label: "go",
					Provenance: graph.Provenance{
						Session: "s1",
						User:    "alice",
					},
				},
			}
			reg.AutoBindFromExtraction(concepts)

			bindings := reg.ConceptNamespaces("topic:go")
			Expect(len(bindings)).To(BeNumerically(">=", 2)) // session + user + subject
		})
	})
})
