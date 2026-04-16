package extract_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/extract"
	"github.com/gnomatix/enkente/pkg/graph"
	"github.com/gnomatix/enkente/pkg/parser"
)

func TestExtract(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Extract Suite")
}

var _ = Describe("FromMessage", func() {
	now := time.Now()

	makeMsg := func(user, message string) parser.AntigravityMessage {
		return parser.AntigravityMessage{
			SessionID: "test-session",
			MessageID: 1,
			Type:      "user",
			User:      user,
			Message:   message,
			Timestamp: now,
		}
	}

	It("should extract the sender as a person concept", func() {
		result := extract.FromMessage(makeMsg("alice", "hello world"))
		var personIDs []string
		for _, c := range result.Concepts {
			if c.Type == graph.PersonType {
				personIDs = append(personIDs, c.ID)
			}
		}
		Expect(personIDs).To(ContainElement("person:alice"))
	})

	It("should extract @mentions as person concepts", func() {
		result := extract.FromMessage(makeMsg("alice", "Hey @bob and @charlie"))
		var personIDs []string
		for _, c := range result.Concepts {
			if c.Type == graph.PersonType {
				personIDs = append(personIDs, c.ID)
			}
		}
		Expect(personIDs).To(ContainElement("person:bob"))
		Expect(personIDs).To(ContainElement("person:charlie"))
	})

	It("should create mentions edges from sender to mentioned persons", func() {
		result := extract.FromMessage(makeMsg("alice", "Hey @bob"))
		var mentionEdges []string
		for _, e := range result.Edges {
			if e.Type == graph.Mentions {
				mentionEdges = append(mentionEdges, e.Source+"->"+e.Target)
			}
		}
		Expect(mentionEdges).To(ContainElement("person:alice->person:bob"))
	})

	It("should extract #tags as topic concepts", func() {
		result := extract.FromMessage(makeMsg("alice", "Let us discuss #graph_databases and #semantics"))
		var topicIDs []string
		for _, c := range result.Concepts {
			if c.Type == graph.TopicType {
				topicIDs = append(topicIDs, c.ID)
			}
		}
		Expect(topicIDs).To(ContainElement("topic:graph_databases"))
		Expect(topicIDs).To(ContainElement("topic:semantics"))
	})

	It("should create discusses edges from sender to topics", func() {
		result := extract.FromMessage(makeMsg("alice", "talking about #graphs"))
		var discussEdges []string
		for _, e := range result.Edges {
			if e.Type == graph.Discusses {
				discussEdges = append(discussEdges, e.Source+"->"+e.Target)
			}
		}
		Expect(discussEdges).To(ContainElement("person:alice->topic:graphs"))
	})

	It("should extract quoted terms", func() {
		result := extract.FromMessage(makeMsg("alice", `She said "semantic encoding" is important`))
		var quotedIDs []string
		for _, c := range result.Concepts {
			if c.Type == graph.QuotedTermType {
				quotedIDs = append(quotedIDs, c.ID)
			}
		}
		Expect(quotedIDs).To(ContainElement("quoted_term:semantic_encoding"))
	})

	It("should create session participation edges", func() {
		result := extract.FromMessage(makeMsg("alice", "hello"))
		var participates bool
		for _, e := range result.Edges {
			if e.Type == graph.Participates && e.Source == "person:alice" {
				participates = true
			}
		}
		Expect(participates).To(BeTrue())
	})

	It("should create co-occurrence edges between non-person concepts", func() {
		result := extract.FromMessage(makeMsg("alice", `discussing #graphs and "semantic encoding"`))
		var coOccurs bool
		for _, e := range result.Edges {
			if e.Type == graph.CoOccurs {
				coOccurs = true
			}
		}
		Expect(coOccurs).To(BeTrue())
	})

	It("should handle messages with no user gracefully", func() {
		msg := parser.AntigravityMessage{
			SessionID: "test",
			Type:      "system",
			Message:   "System message with #topic",
			Timestamp: now,
		}
		result := extract.FromMessage(msg)
		// Should still extract the topic
		var topicFound bool
		for _, c := range result.Concepts {
			if c.ID == "topic:topic" {
				topicFound = true
			}
		}
		Expect(topicFound).To(BeTrue())
	})

	It("should apply results to a ConceptGraph", func() {
		g := graph.NewConceptGraph()
		result := extract.FromMessage(makeMsg("alice", "Hey @bob, let us discuss #graphs"))
		extract.ApplyResult(g, result)

		Expect(g.GetConcept("person:alice")).ToNot(BeNil())
		Expect(g.GetConcept("person:bob")).ToNot(BeNil())
		Expect(g.GetConcept("topic:graphs")).ToNot(BeNil())

		stats := g.Stats()
		Expect(stats.NumConcepts).To(BeNumerically(">=", 3))
		Expect(stats.NumEdges).To(BeNumerically(">=", 2))
	})
})
