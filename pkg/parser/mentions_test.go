package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
)

var _ = Describe("ExtractMentions", func() {
	tag := func(n string) parser.Mention { return parser.Mention{Kind: parser.MentionTag, Name: n} }
	user := func(n string) parser.Mention { return parser.Mention{Kind: parser.MentionUser, Name: n} }

	DescribeTable("finds explicit references",
		func(text string, want []parser.Mention) {
			Expect(parser.ExtractMentions(text)).To(Equal(want))
		},
		Entry("tag and user", "@alice what about #graph_databases?", []parser.Mention{user("alice"), tag("graph_databases")}),
		Entry("start of text", "#nlp first", []parser.Mention{tag("nlp")}),
		Entry("normalizes case and dedupes", "#GraphDB vs #graphdb and #GRAPHDB", []parser.Mention{tag("graphdb")}),
		Entry("drops sentence punctuation", "ask @bob. Then #rss-feeds.", []parser.Mention{user("bob"), tag("rss-feeds")}),
		Entry("keeps inner dots and hyphens", "#v1.2-beta and @j.doe", []parser.Mention{tag("v1.2-beta"), user("j.doe")}),
		Entry("inside parentheses", "(see #ontology)", []parser.Mention{tag("ontology")}),
		Entry("unicode names", "#café @renée", []parser.Mention{tag("café"), user("renée")}),
		Entry("ignores e-mail addresses", "mail me at brett@example.com", nil),
		Entry("ignores URL fragments", "https://example.com/page#section and /path/#x", nil),
		Entry("ignores markdown headings", "# Heading\n## Sub", nil),
		Entry("ignores numeric issue refs", "fixed in #42", nil),
		Entry("ignores bare sigils", "@ # @@ ##", nil),
		Entry("ignores doubled sigils", "##tag @@user", nil),
	)

	It("parses kind spellings", func() {
		for _, s := range []string{"tag", "TAGS", "#"} {
			k, ok := parser.ParseMentionKind(s)
			Expect(ok).To(BeTrue())
			Expect(k).To(Equal(parser.MentionTag))
		}
		for _, s := range []string{"user", "users", "@"} {
			k, ok := parser.ParseMentionKind(s)
			Expect(ok).To(BeTrue())
			Expect(k).To(Equal(parser.MentionUser))
		}
		_, ok := parser.ParseMentionKind("topic")
		Expect(ok).To(BeFalse())
	})
})
