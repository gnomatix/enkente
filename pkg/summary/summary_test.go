package summary_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/summary"
)

func TestSummary(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Summary Suite")
}

var _ = Describe("Tokens", func() {
	It("keeps content words and drops stopwords, short words, numbers and mentions", func() {
		Expect(summary.Tokens("The #GraphDB is a Graph database, @alice! 42 ok it's fine")).To(
			Equal([]string{"graph", "database", "fine"}))
	})
})

var _ = Describe("Baseline", func() {
	base := time.Date(2025, 9, 4, 21, 0, 0, 0, time.UTC)
	m := func(session string, i int, user, text string) parser.AntigravityMessage {
		return parser.AntigravityMessage{SessionID: session, MessageID: i, User: user, Message: text,
			Timestamp: base.Add(time.Duration(i) * time.Minute)}
	}
	corpus := map[string][]parser.AntigravityMessage{
		"s1": {
			m("s1", 0, "alice", "We should store concepts in a graph database #graphdb"),
			m("s1", 1, "bob", "Graph traversal makes concept lookup cheap"),
			m("s1", 2, "alice", "lunch?"),
			m("s1", 3, "alice", "Graph schema needs a concept node and an edge type #schema"),
		},
		"s2": {
			m("s2", 0, "carol", "We should plan the release schedule"),
			m("s2", 1, "dave", "release notes need a schedule too"),
		},
	}

	It("reports counts, span, participants and tags", func() {
		s := summary.Baseline{}.Summarize("s1", corpus, 5, 2)
		Expect(s.Method).To(Equal(summary.Method))
		Expect(s.Messages).To(Equal(4))
		Expect(s.Start).To(Equal(base))
		Expect(s.End).To(Equal(base.Add(3 * time.Minute)))
		Expect(s.Participants).To(Equal([]summary.Participant{{User: "alice", Messages: 3}, {User: "bob", Messages: 1}}))
		Expect(s.Tags).To(Equal([]string{"graphdb", "schema"}))
	})

	It("ranks session-specific terms first and caps the list", func() {
		s := summary.Baseline{}.Summarize("s1", corpus, 2, 0)
		Expect(s.Keywords).To(HaveLen(2))
		Expect(s.Keywords[0].Term).To(Equal("graph"))
		Expect(s.Keywords[0].Count).To(Equal(3))
		Expect(s.Keywords[1].Term).To(Equal("concept"))
		Expect(s.Highlights).To(BeEmpty())
	})

	It("down-weights terms shared with other sessions", func() {
		s := summary.Baseline{}.Summarize("s2", corpus, -1, 0)
		var terms []string
		for _, k := range s.Keywords {
			terms = append(terms, k.Term)
		}
		Expect(terms[:2]).To(ConsistOf("release", "schedule"))
	})

	It("returns highlights in chronological order and skips content-free messages", func() {
		s := summary.Baseline{}.Summarize("s1", corpus, 0, 3)
		Expect(s.Highlights).To(HaveLen(3))
		ids := []int{s.Highlights[0].MessageID, s.Highlights[1].MessageID, s.Highlights[2].MessageID}
		Expect(ids).To(Equal([]int{0, 1, 3}), "lunch? has no content words")
	})

	It("returns empty, non-nil lists for an unknown or empty session", func() {
		s := summary.Baseline{}.Summarize("none", corpus, 5, 5)
		Expect(s.Messages).To(Equal(0))
		Expect(s.Keywords).NotTo(BeNil())
		Expect(s.Highlights).NotTo(BeNil())
		Expect(s.Participants).NotTo(BeNil())
	})
})
