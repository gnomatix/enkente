package api_test

import (
	"net/http"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/api"
	"github.com/gnomatix/enkente/pkg/listener"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
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

var _ = Describe("Active listener", func() {
	var (
		h     http.Handler
		store *storage.BoltStorage
		rec   *recorder
	)

	BeforeEach(func() {
		lex, err := listener.NewLexicon([]listener.Entry{
			{Term: "database", Senses: []string{"storage engine", "dataset"}},
		})
		Expect(err).NotTo(HaveOccurred())
		store, err = storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "active.db"))
		Expect(err).NotTo(HaveOccurred())
		rec = &recorder{}
		h = api.NewServer(0, 1, rec.handle, api.WithStore(store), api.WithLexicon(lex), api.WithActiveListener()).Handler()
	})

	AfterEach(func() {
		Expect(store.Close()).To(Succeed())
	})

	It("posts a stored, dispatched clarification prompt into the same session", func() {
		rr := post(h, "/ingest", `{"sessionId":"s","user":"alice","project":"p","message":"which database?"}`)
		Expect(rr.Code).To(Equal(http.StatusCreated))
		var body struct {
			Message parser.AntigravityMessage  `json:"message"`
			Prompt  *parser.AntigravityMessage `json:"prompt"`
		}
		decodeInto(rr, &body)
		Expect(body.Prompt).NotTo(BeNil())
		Expect(body.Prompt.Type).To(Equal("system"))
		Expect(body.Prompt.User).To(Equal(listener.PromptUser))
		Expect(body.Prompt.SessionID).To(Equal("s"))
		Expect(body.Prompt.Project).To(Equal("p"))
		Expect(body.Prompt.MessageID).To(BeNumerically(">", body.Message.MessageID))
		Expect(body.Prompt.Message).To(HavePrefix("@alice Listener here"))

		var msgs []parser.AntigravityMessage
		decodeInto(get(h, "/sessions/s/messages"), &msgs)
		Expect(msgs).To(HaveLen(2))
		Eventually(rec.count).Should(Equal(2))
	})

	It("does not prompt on unambiguous, system or listener messages", func() {
		for _, b := range []string{
			`{"sessionId":"s","message":"hello"}`,
			`{"sessionId":"s","type":"system","message":"database"}`,
			`{"sessionId":"s","user":"listener","message":"database"}`,
		} {
			rr := post(h, "/ingest", b)
			Expect(rr.Body.String()).NotTo(ContainSubstring(`"prompt"`), b)
		}
		var msgs []parser.AntigravityMessage
		decodeInto(get(h, "/sessions/s/messages"), &msgs)
		Expect(msgs).To(HaveLen(3))
	})
})
