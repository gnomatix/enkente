package sentiment_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/graph"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/sentiment"
)

func TestSentiment(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sentiment Suite")
}

var _ = Describe("Sentiment Analysis", func() {
	Describe("Analyze", func() {
		It("should detect positive sentiment", func() {
			msg := parser.AntigravityMessage{
				User:    "alice",
				Message: "This is a great idea, I love it! Absolutely fantastic work.",
			}
			result := sentiment.Analyze(msg)
			Expect(result.Sentiment).To(Equal(sentiment.Positive))
			Expect(result.Score).To(BeNumerically(">", 0.15))
		})

		It("should detect negative sentiment", func() {
			msg := parser.AntigravityMessage{
				User:    "bob",
				Message: "This is terrible and broken. The whole thing is a frustrating mess.",
			}
			result := sentiment.Analyze(msg)
			Expect(result.Sentiment).To(Equal(sentiment.Negative))
			Expect(result.Score).To(BeNumerically("<", -0.15))
		})

		It("should detect neutral sentiment", func() {
			msg := parser.AntigravityMessage{
				User:    "carol",
				Message: "The meeting is scheduled for tomorrow at 3pm in the conference room.",
			}
			result := sentiment.Analyze(msg)
			Expect(result.Sentiment).To(Equal(sentiment.Neutral))
		})

		It("should handle negation", func() {
			msg := parser.AntigravityMessage{
				User:    "dave",
				Message: "This is not good at all.",
			}
			result := sentiment.Analyze(msg)
			Expect(result.Score).To(BeNumerically("<", 0))
		})

		It("should detect question tone", func() {
			msg := parser.AntigravityMessage{
				User:    "eve",
				Message: "What do you think about this approach?",
			}
			result := sentiment.Analyze(msg)
			Expect(result.Tones).To(ContainElement(sentiment.Questioning))
		})

		It("should detect enthusiastic tone", func() {
			msg := parser.AntigravityMessage{
				User:    "frank",
				Message: "This is amazing! I can't believe how well it works!",
			}
			result := sentiment.Analyze(msg)
			Expect(result.Tones).To(ContainElement(sentiment.Enthusiastic))
		})
	})

	Describe("Tracker", func() {
		var tracker *sentiment.Tracker

		BeforeEach(func() {
			tracker = sentiment.NewTracker()
		})

		It("should track concept sentiments", func() {
			analysis := sentiment.Analysis{
				Score:     0.5,
				Sentiment: sentiment.Positive,
				Tones:     []sentiment.ToneTag{sentiment.Enthusiastic},
			}
			tracker.RecordForConcepts([]string{"topic:go", "topic:testing"}, analysis)

			cs := tracker.GetConceptSentiment("topic:go")
			Expect(cs).NotTo(BeNil())
			Expect(cs.AvgScore).To(Equal(0.5))
			Expect(cs.Observations).To(Equal(1))
		})

		It("should aggregate multiple observations", func() {
			tracker.RecordForConcepts([]string{"topic:go"}, sentiment.Analysis{Score: 0.5})
			tracker.RecordForConcepts([]string{"topic:go"}, sentiment.Analysis{Score: -0.3})

			cs := tracker.GetConceptSentiment("topic:go")
			Expect(cs).NotTo(BeNil())
			Expect(cs.Observations).To(Equal(2))
			Expect(cs.AvgScore).To(BeNumerically("~", 0.1, 0.01))
		})

		It("should apply sentiment to graph", func() {
			g := graph.NewConceptGraph()
			g.AddConcept(&graph.Concept{
				ID:    "topic:go",
				Type:  graph.TopicType,
				Label: "go",
			})

			tracker.RecordForConcepts([]string{"topic:go"}, sentiment.Analysis{Score: 0.7})
			tracker.ApplyToGraph(g)

			c := g.GetConcept("topic:go")
			Expect(c.Properties).To(HaveKey("sentiment_avg"))
			Expect(c.Properties["sentiment_avg"]).To(Equal(0.7))
		})
	})
})
