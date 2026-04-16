package listener_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/listener"
)

func TestListener(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Listener Suite")
}

var _ = Describe("Listener", func() {
	var l *listener.Listener

	BeforeEach(func() {
		l = listener.NewListener(listener.Passive)
	})

	Describe("Mode", func() {
		It("should start in passive mode", func() {
			Expect(l.GetMode()).To(Equal(listener.Passive))
		})

		It("should switch to active mode", func() {
			l.SetMode(listener.Active)
			Expect(l.GetMode()).To(Equal(listener.Active))
		})
	})

	Describe("RegisterDefinition", func() {
		It("should register a term definition", func() {
			l.RegisterDefinition("database", "a structured collection of data", "alice", "general")
			profile := l.GetTermProfile("database")
			Expect(profile).NotTo(BeNil())
			Expect(profile.Definitions).To(HaveLen(1))
			Expect(profile.Definitions[0].Meaning).To(Equal("a structured collection of data"))
		})

		It("should increment use count for duplicate definitions", func() {
			l.RegisterDefinition("database", "a structured collection of data", "alice", "general")
			l.RegisterDefinition("database", "a structured collection of data", "bob", "general")
			profile := l.GetTermProfile("database")
			Expect(profile.Definitions).To(HaveLen(1))
			Expect(profile.Definitions[0].UseCount).To(Equal(2))
		})

		It("should track multiple definitions", func() {
			l.RegisterDefinition("model", "a data model", "alice", "cs")
			l.RegisterDefinition("model", "a machine learning model", "bob", "ml")
			profile := l.GetTermProfile("model")
			Expect(profile.Definitions).To(HaveLen(2))
		})
	})

	Describe("RegisterSynonym", func() {
		It("should register bidirectional synonyms", func() {
			l.RegisterSynonym("DB", "database")
			profileDB := l.GetTermProfile("db")
			profileDatabase := l.GetTermProfile("database")

			Expect(profileDB.Synonyms).To(ContainElement("database"))
			Expect(profileDatabase.Synonyms).To(ContainElement("db"))
		})
	})

	Describe("AnalyzeMessage", func() {
		It("should flag quoted terms as jargon", func() {
			flags := l.AnalyzeMessage(`Let's use "semantic encoding" for this`, "alice", "session1")
			Expect(flags).To(HaveLen(1))
			Expect(flags[0].Type).To(Equal(listener.Jargon))
			Expect(flags[0].Term).To(Equal("semantic encoding"))
		})

		It("should detect overloaded terms", func() {
			// Register conflicting definitions
			l.RegisterDefinition("model", "a data schema", "alice", "db")
			l.RegisterDefinition("model", "a neural network", "bob", "ml")

			flags := l.AnalyzeMessage("We need to update the model", "carol", "session1")
			var overloaded bool
			for _, f := range flags {
				if f.Type == listener.OverloadedTerm && f.Term == "model" {
					overloaded = true
				}
			}
			Expect(overloaded).To(BeTrue())
		})

		It("should generate prompts in active mode", func() {
			l.SetMode(listener.Active)
			flags := l.AnalyzeMessage(`What about "graph normalization"?`, "alice", "s1")
			Expect(flags).NotTo(BeEmpty())
			Expect(flags[0].Prompt).To(ContainSubstring("Listener here"))
		})

		It("should not generate prompts in passive mode", func() {
			flags := l.AnalyzeMessage(`What about "graph normalization"?`, "alice", "s1")
			if len(flags) > 0 {
				Expect(flags[0].Prompt).To(BeEmpty())
			}
		})
	})

	Describe("ResolveFlag", func() {
		It("should resolve a flag", func() {
			flags := l.AnalyzeMessage(`Let's use "semantic encoding"`, "alice", "s1")
			Expect(flags).NotTo(BeEmpty())

			err := l.ResolveFlag(flags[0].ID, "converting text to vector representations")
			Expect(err).NotTo(HaveOccurred())

			allFlags := l.GetFlags(true)
			var resolved bool
			for _, f := range allFlags {
				if f.ID == flags[0].ID {
					resolved = f.Resolved
				}
			}
			Expect(resolved).To(BeTrue())
		})

		It("should return error for unknown flag", func() {
			err := l.ResolveFlag("flag:999", "meaning")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetFlags filtering", func() {
		It("should filter out resolved flags by default", func() {
			l.AnalyzeMessage(`Let's use "term_a"`, "alice", "s1")
			l.AnalyzeMessage(`What about "term_b"?`, "bob", "s1")

			allFlags := l.GetFlags(false)
			count := len(allFlags)
			Expect(count).To(BeNumerically(">=", 2))

			// Resolve one
			_ = l.ResolveFlag(allFlags[0].ID, "meaning")

			unresolvedFlags := l.GetFlags(false)
			Expect(unresolvedFlags).To(HaveLen(count - 1))

			allIncluding := l.GetFlags(true)
			Expect(allIncluding).To(HaveLen(count))
		})
	})
})
