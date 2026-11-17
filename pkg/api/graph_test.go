package api_test

import (
	"net/http"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/api"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

var _ = Describe("Graph API", func() {
	var (
		h     http.Handler
		store *storage.BoltStorage
	)

	BeforeEach(func() {
		var err error
		store, err = storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "graph.db"))
		Expect(err).NotTo(HaveOccurred())
		h = api.NewServer(0, 1, func(int, parser.AntigravityMessage) {}, api.WithStore(store)).Handler()
		for _, b := range []string{
			`{"sessionId":"a","user":"alice","message":"#graphdb #nlp","timestamp":"2025-09-04T21:00:00Z"}`,
			`{"sessionId":"b","user":"bob","message":"#graphdb @alice","timestamp":"2025-09-04T22:00:00Z"}`,
		} {
			Expect(post(h, "/ingest", b).Code).To(Equal(http.StatusCreated))
		}
	})

	AfterEach(func() {
		Expect(store.Close()).To(Succeed())
	})

	It("returns nodes and weighted edges", func() {
		rr := get(h, "/graph")
		Expect(rr.Code).To(Equal(http.StatusOK))
		var g storage.Graph
		decodeInto(rr, &g)
		Expect(g.Nodes).To(HaveLen(4))
		Expect(g.Edges).To(ContainElement(storage.GraphEdge{Source: "tag:graphdb", Target: "tag:nlp", Relation: storage.RelCoOccurs, Weight: 1}))
		Expect(g.Edges).To(ContainElement(storage.GraphEdge{Source: "user:bob", Target: "user:alice", Relation: storage.RelMentions, Weight: 1}))
	})

	It("applies session and time filters", func() {
		var g storage.Graph
		decodeInto(get(h, "/graph?session=a"), &g)
		Expect(g.Nodes).To(HaveLen(3))
		decodeInto(get(h, "/graph?since=2025-09-04T21:30:00Z"), &g)
		Expect(g.Nodes).To(HaveLen(3))
		Expect(get(h, "/graph?session=zzz").Code).To(Equal(http.StatusNotFound))
		Expect(get(h, "/graph?until=bad").Code).To(Equal(http.StatusBadRequest))
	})

	It("returns 503 without a store", func() {
		bare := api.NewServer(0, 1, func(int, parser.AntigravityMessage) {}).Handler()
		Expect(get(bare, "/graph").Code).To(Equal(http.StatusServiceUnavailable))
	})
})
