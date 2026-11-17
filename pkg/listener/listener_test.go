package listener_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/listener"
)

func TestListener(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Listener Suite")
}

var _ = Describe("Lexicon", func() {
	var lex *listener.Lexicon

	BeforeEach(func() {
		var err error
		lex, err = listener.NewLexicon([]listener.Entry{
			{Term: "database", Senses: []string{"storage engine", "dataset"}, Synonyms: []string{"db", "datastore"}},
			{Term: "graph database", Synonyms: []string{"graph db"}},
			{Term: "ontology", Senses: []string{"controlled vocabulary"}},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("flags an overloaded term with its senses", func() {
		got := lex.Analyze("Which Database do we mean?")
		Expect(got).To(HaveLen(1))
		Expect(got[0].Term).To(Equal("database"))
		Expect(got[0].Overloaded).To(BeTrue())
		Expect(got[0].Senses).To(ConsistOf("storage engine", "dataset"))
		Expect(got[0].Synonym).To(BeFalse())
	})

	It("maps synonyms to the canonical term", func() {
		got := lex.Analyze("the datastore and the db")
		Expect(got).To(HaveLen(2))
		Expect(got[0].Matched).To(Equal("datastore"))
		Expect(got[0].Term).To(Equal("database"))
		Expect(got[0].Synonym).To(BeTrue())
		Expect(got[1].Matched).To(Equal("db"))
	})

	It("prefers the longest phrase at a position", func() {
		got := lex.Analyze("a graph database, not just a database")
		Expect(got).To(HaveLen(2))
		Expect(got[0].Term).To(Equal("graph database"))
		Expect(got[0].Overloaded).To(BeFalse())
		Expect(got[1].Term).To(Equal("database"))
	})

	It("matches multi-word synonyms across punctuation and case", func() {
		got := lex.Analyze("Graph-DB? no: Graph DB.")
		Expect(got).To(HaveLen(1))
		Expect(got[0].Term).To(Equal("graph database"))
		Expect(got[0].Count).To(Equal(1))
	})

	It("counts repeats and keeps first-occurrence order", func() {
		got := lex.Analyze("ontology db ontology")
		Expect(got).To(HaveLen(2))
		Expect(got[0].Term).To(Equal("ontology"))
		Expect(got[0].Count).To(Equal(2))
		Expect(got[0].Overloaded).To(BeFalse(), "one sense is not ambiguous")
	})

	It("matches whole words only", func() {
		Expect(lex.Analyze("databases dbx ontologyish")).To(BeEmpty())
	})

	It("summarizes the overloaded terms", func() {
		s := lex.Summarize("db vs graph db vs database")
		Expect(s.Overloaded).To(Equal([]string{"database"}))
		Expect(s.Flags).To(HaveLen(3))
	})

	It("is a no-op when nil or empty", func() {
		var none *listener.Lexicon
		Expect(none.Analyze("database")).To(BeEmpty())
		empty, err := listener.NewLexicon(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(empty.Summarize("database").Overloaded).To(BeEmpty())
	})

	It("rejects a phrase claimed by two entries, and empty terms", func() {
		_, err := listener.NewLexicon([]listener.Entry{{Term: "db"}, {Term: "database", Synonyms: []string{"DB"}}})
		Expect(err).To(MatchError(ContainSubstring(`"db"`)))
		_, err = listener.NewLexicon([]listener.Entry{{Term: "  "}})
		Expect(err).To(HaveOccurred())
	})

	It("loads a JSON lexicon file", func() {
		path := filepath.Join(GinkgoT().TempDir(), "lex.json")
		Expect(os.WriteFile(path, []byte(`[{"term":"model","senses":["ML model","data model"]}]`), 0o644)).To(Succeed())
		l, err := listener.LoadLexicon(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(l.Len()).To(Equal(1))
		Expect(l.Summarize("the model").Overloaded).To(Equal([]string{"model"}))

		Expect(os.WriteFile(path, []byte(`{"not":"an array"}`), 0o644)).To(Succeed())
		_, err = listener.LoadLexicon(path)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Prompt", func() {
	lex, _ := listener.NewLexicon([]listener.Entry{
		{Term: "database", Senses: []string{"storage engine", "dataset"}, Synonyms: []string{"db"}},
		{Term: "model", Senses: []string{"ML model", "schema"}},
		{Term: "ontology", Senses: []string{"controlled vocabulary"}},
	})

	It("asks about one overloaded term with lettered senses", func() {
		Expect(listener.Prompt(lex.Summarize("the db and the db"), "alice")).To(Equal(
			`@alice Listener here: which sense of "database" did you mean? (a) storage engine (b) dataset`))
	})

	It("lists several terms in first-occurrence order", func() {
		Expect(listener.Prompt(lex.Summarize("model in the database"), "")).To(Equal(
			`Listener here: which senses did you mean? "model" (a) ML model (b) schema; "database" (a) storage engine (b) dataset`))
	})

	It("is empty when nothing is ambiguous, and skips unaddressable authors", func() {
		Expect(listener.Prompt(lex.Summarize("an ontology"), "alice")).To(BeEmpty())
		Expect(listener.Prompt(lex.Summarize("db"), "Alice Smith")).To(HavePrefix("Listener here:"))
	})
})
