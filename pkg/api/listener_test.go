package api_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/api"
	"github.com/gnomatix/enkente/pkg/listener"
	"github.com/gnomatix/enkente/pkg/parser"
)

var _ = Describe("Listener API", func() {
	noop := func(int, parser.AntigravityMessage) {}
	var h http.Handler

	BeforeEach(func() {
		lex, err := listener.NewLexicon([]listener.Entry{
			{Term: "database", Senses: []string{"storage engine", "dataset"}, Synonyms: []string{"db"}},
		})
		Expect(err).NotTo(HaveOccurred())
		h = api.NewServer(0, 1, noop, api.WithLexicon(lex)).Handler()
	})

	It("attaches listener metadata to the ingest response", func() {
		rr := post(h, "/ingest", `{"message":"put it in the db"}`)
		Expect(rr.Code).To(Equal(http.StatusAccepted))
		var body struct {
			Listener listener.Summary `json:"listener"`
		}
		decodeInto(rr, &body)
		Expect(body.Listener.Overloaded).To(Equal([]string{"database"}))
		Expect(body.Listener.Flags[0].Matched).To(Equal("db"))
		Expect(body.Listener.Flags[0].Synonym).To(BeTrue())
	})

	It("previews a draft without ingesting it", func() {
		rr := post(h, "/listener/analyze", `{"message":"Which database?"}`)
		Expect(rr.Code).To(Equal(http.StatusOK))
		var sum listener.Summary
		decodeInto(rr, &sum)
		Expect(sum.Overloaded).To(Equal([]string{"database"}))
		Expect(sum.Flags[0].Senses).To(ConsistOf("storage engine", "dataset"))
	})

	It("returns empty lists for text with no jargon", func() {
		rr := post(h, "/listener/analyze", `{"message":"hello"}`)
		Expect(rr.Body.String()).To(MatchJSON(`{"flags":[],"overloaded":[]}`))
	})

	It("rejects a malformed preview body", func() {
		Expect(post(h, "/listener/analyze", `not json`).Code).To(Equal(http.StatusBadRequest))
	})

	It("omits listener metadata and refuses previews without a lexicon", func() {
		bare := api.NewServer(0, 1, noop).Handler()
		rr := post(bare, "/ingest", `{"message":"db"}`)
		Expect(rr.Body.String()).NotTo(ContainSubstring(`"listener"`))
		Expect(post(bare, "/listener/analyze", `{"message":"db"}`).Code).To(Equal(http.StatusServiceUnavailable))
	})
})
