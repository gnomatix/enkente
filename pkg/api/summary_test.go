package api_test

import (
	"net/http"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/api"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
	"github.com/gnomatix/enkente/pkg/summary"
)

var _ = Describe("Summary API", func() {
	var (
		h     http.Handler
		store *storage.BoltStorage
	)

	BeforeEach(func() {
		var err error
		store, err = storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "summary.db"))
		Expect(err).NotTo(HaveOccurred())
		h = api.NewServer(0, 1, func(int, parser.AntigravityMessage) {}, api.WithStore(store)).Handler()
		for _, b := range []string{
			`{"sessionId":"g","user":"alice","message":"graph database for concepts #graphdb"}`,
			`{"sessionId":"g","user":"bob","message":"graph traversal is cheap"}`,
			`{"sessionId":"r","user":"carol","message":"release schedule"}`,
		} {
			Expect(post(h, "/ingest", b).Code).To(Equal(http.StatusCreated))
		}
	})

	AfterEach(func() {
		Expect(store.Close()).To(Succeed())
	})

	It("summarizes a session", func() {
		rr := get(h, "/sessions/g/summary?keywords=3&highlights=1")
		Expect(rr.Code).To(Equal(http.StatusOK))
		var s summary.Summary
		decodeInto(rr, &s)
		Expect(s.Method).To(Equal(summary.Method))
		Expect(s.Messages).To(Equal(2))
		Expect(s.Tags).To(Equal([]string{"graphdb"}))
		Expect(s.Keywords).To(HaveLen(3))
		Expect(s.Keywords[0].Term).To(Equal("graph"))
		Expect(s.Highlights).To(HaveLen(1))
	})

	It("clusters a session into topics", func() {
		rr := get(h, "/sessions/g/topics?k=2")
		Expect(rr.Code).To(Equal(http.StatusOK))
		var topics []summary.Topic
		decodeInto(rr, &topics)
		Expect(topics).To(Equal([]summary.Topic{
			{Label: "#graphdb", MessageIDs: []int{1}},
			{Label: "graph", MessageIDs: []int{2}},
		}))
		Expect(get(h, "/sessions/g/topics?k=x").Code).To(Equal(http.StatusBadRequest))
		Expect(get(h, "/sessions/nope/topics").Code).To(Equal(http.StatusNotFound))
	})

	It("validates parameters and session", func() {
		Expect(get(h, "/sessions/nope/summary").Code).To(Equal(http.StatusNotFound))
		Expect(get(h, "/sessions/g/summary?keywords=-1").Code).To(Equal(http.StatusBadRequest))
		Expect(get(h, "/sessions/g/summary?highlights=x").Code).To(Equal(http.StatusBadRequest))
	})
})
