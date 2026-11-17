package storage_test

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

var _ = Describe("Graph", func() {
	var (
		dbStore *storage.BoltStorage
		base    time.Time
	)

	BeforeEach(func() {
		store, err := storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "graph.db"))
		Expect(err).NotTo(HaveOccurred())
		dbStore = store
		base = time.Date(2025, 9, 4, 21, 0, 0, 0, time.UTC)
		put := func(session string, id int, user, text string, min int) {
			Expect(dbStore.PutMessage(parser.AntigravityMessage{
				SessionID: session, MessageID: id, Type: "user", User: user, Message: text,
				Timestamp: base.Add(time.Duration(min) * time.Minute),
			})).To(Succeed())
		}
		put("s1", 0, "Alice", "#graphdb and #nlp, @bob?", 0)
		put("s1", 1, "bob", "#GraphDB #nlp yes @alice", 1)
		put("s2", 0, "carol", "#graphdb alone, me @carol", 2)
		put("s2", 1, "", "system note #nlp", 3)
	})

	AfterEach(func() {
		Expect(dbStore.Close()).To(Succeed())
	})

	edge := func(g *storage.Graph, src, dst, rel string) int {
		for _, e := range g.Edges {
			if e.Source == src && e.Target == dst && e.Relation == rel {
				return e.Weight
			}
		}
		return 0
	}
	node := func(g *storage.Graph, id string) *storage.GraphNode {
		for i := range g.Nodes {
			if g.Nodes[i].ID == id {
				return &g.Nodes[i]
			}
		}
		return nil
	}

	It("builds nodes for authors and mentions, merging @user with authorship", func() {
		g, err := dbStore.Graph("", storage.MessageFilter{})
		Expect(err).NotTo(HaveOccurred())
		var ids []string
		for _, n := range g.Nodes {
			ids = append(ids, n.ID)
		}
		Expect(ids).To(Equal([]string{"tag:graphdb", "tag:nlp", "user:alice", "user:bob", "user:carol"}))
		Expect(node(g, "user:alice").Messages).To(Equal(2), "wrote one, mentioned in one")
		Expect(node(g, "tag:graphdb").Messages).To(Equal(3))
		Expect(node(g, "tag:nlp").FirstSeen).To(Equal(base))
		Expect(node(g, "tag:nlp").LastSeen).To(Equal(base.Add(3 * time.Minute)))
	})

	It("weights mention and co-occurrence edges by message count", func() {
		g, err := dbStore.Graph("", storage.MessageFilter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(edge(g, "user:alice", "tag:graphdb", storage.RelMentions)).To(Equal(1))
		Expect(edge(g, "user:alice", "user:bob", storage.RelMentions)).To(Equal(1))
		Expect(edge(g, "user:bob", "user:alice", storage.RelMentions)).To(Equal(1))
		Expect(edge(g, "tag:graphdb", "tag:nlp", storage.RelCoOccurs)).To(Equal(2))
		Expect(edge(g, "tag:nlp", "tag:graphdb", storage.RelCoOccurs)).To(Equal(0), "pairs are stored once, sorted")
	})

	It("skips self-mentions and authorless edges", func() {
		g, err := dbStore.Graph("", storage.MessageFilter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(edge(g, "user:carol", "user:carol", storage.RelMentions)).To(Equal(0))
		Expect(node(g, "user:carol").Messages).To(Equal(1))
		for _, e := range g.Edges {
			Expect(e.Source).NotTo(Equal("user:"))
		}
	})

	It("restricts to one session and to a filter", func() {
		g, err := dbStore.Graph("s2", storage.MessageFilter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(node(g, "user:alice")).To(BeNil())
		Expect(node(g, "tag:graphdb").Messages).To(Equal(1))

		g, err = dbStore.Graph("", storage.MessageFilter{User: "bob"})
		Expect(err).NotTo(HaveOccurred())
		Expect(g.Nodes).To(HaveLen(4))

		_, err = dbStore.Graph("missing", storage.MessageFilter{})
		Expect(err).To(MatchError(storage.ErrNotFound))
	})

	It("returns empty arrays for an empty store", func() {
		empty, err := storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "empty.db"))
		Expect(err).NotTo(HaveOccurred())
		defer empty.Close()
		g, err := empty.Graph("", storage.MessageFilter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(g.Nodes).NotTo(BeNil())
		Expect(g.Edges).NotTo(BeNil())
	})
})
