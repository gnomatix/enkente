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

var _ = Describe("Mentions API", func() {
	var (
		h     http.Handler
		store *storage.BoltStorage
	)

	BeforeEach(func() {
		var err error
		store, err = storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "mentions.db"))
		Expect(err).NotTo(HaveOccurred())
		h = api.NewServer(0, 1, func(int, parser.AntigravityMessage) {}, api.WithStore(store)).Handler()

		for _, body := range []string{
			`{"sessionId":"s1","user":"alice","message":"Start on #GraphDB with @bob","timestamp":"2025-09-04T21:00:00Z"}`,
			`{"sessionId":"s1","user":"bob","message":"#graphdb needs #nlp too","timestamp":"2025-09-04T21:01:00Z"}`,
			`{"sessionId":"s2","user":"carol","message":"what about #graphdb?","timestamp":"2025-09-04T21:02:00Z"}`,
		} {
			Expect(post(h, "/ingest", body).Code).To(Equal(http.StatusCreated))
		}
	})

	AfterEach(func() {
		Expect(store.Close()).To(Succeed())
	})

	It("summarizes mentions ingested through the API", func() {
		rr := get(h, "/mentions")
		Expect(rr.Code).To(Equal(http.StatusOK))
		var got []storage.MentionSummary
		decodeInto(rr, &got)
		Expect(got).To(HaveLen(3))
		Expect(got[0].Name).To(Equal("graphdb"))
		Expect(got[0].Messages).To(Equal(3))
		Expect(got[0].Sessions).To(Equal(2))
		Expect(got[0].FirstSeen.User).To(Equal("alice"))
	})

	It("filters by kind", func() {
		var got []storage.MentionSummary
		decodeInto(get(h, "/mentions?kind=user"), &got)
		Expect(got).To(HaveLen(1))
		Expect(got[0].Name).To(Equal("bob"))
	})

	It("rejects an unknown kind filter", func() {
		Expect(get(h, "/mentions?kind=topic").Code).To(Equal(http.StatusBadRequest))
	})

	It("lists messages mentioning a tag in chat-log shape", func() {
		rr := get(h, "/mentions/tag/GraphDB/messages")
		Expect(rr.Code).To(Equal(http.StatusOK))
		var got []parser.AntigravityMessage
		decodeInto(rr, &got)
		Expect(got).To(HaveLen(3))
		Expect(got[0].User).To(Equal("alice"))
		Expect(got[2].SessionID).To(Equal("s2"))
	})

	It("honours limit", func() {
		var got []parser.AntigravityMessage
		decodeInto(get(h, "/mentions/tag/graphdb/messages?limit=1"), &got)
		Expect(got).To(HaveLen(1))
	})

	It("returns an empty array for an unmentioned entity", func() {
		rr := get(h, "/mentions/user/nobody/messages")
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(MatchJSON(`[]`))
	})

	It("rejects a bad kind, name or limit", func() {
		Expect(get(h, "/mentions/topic/graphdb/messages").Code).To(Equal(http.StatusBadRequest))
		Expect(get(h, "/mentions/tag/42/messages").Code).To(Equal(http.StatusBadRequest))
		Expect(get(h, "/mentions/tag/graphdb/messages?limit=-1").Code).To(Equal(http.StatusBadRequest))
	})

	It("returns 503 without a store", func() {
		bare := api.NewServer(0, 1, func(int, parser.AntigravityMessage) {}).Handler()
		Expect(get(bare, "/mentions").Code).To(Equal(http.StatusServiceUnavailable))
		Expect(get(bare, "/mentions/tag/x/messages").Code).To(Equal(http.StatusServiceUnavailable))
	})
})
