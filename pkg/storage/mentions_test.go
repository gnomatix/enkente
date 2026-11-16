package storage_test

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

var _ = Describe("Mentions", func() {
	var (
		dbStore *storage.BoltStorage
		base    time.Time
	)

	BeforeEach(func() {
		store, err := storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "mentions.db"))
		Expect(err).NotTo(HaveOccurred())
		dbStore = store
		base = time.Date(2025, 9, 4, 21, 0, 0, 0, time.UTC)
	})

	AfterEach(func() {
		Expect(dbStore.Close()).To(Succeed())
	})

	put := func(session string, id int, user, text string, minute int) {
		Expect(dbStore.PutMessage(parser.AntigravityMessage{
			SessionID: session, MessageID: id, Type: "user", User: user,
			Message: text, Timestamp: base.Add(time.Duration(minute) * time.Minute),
		})).To(Succeed())
	}

	It("returns an empty list for an empty store", func() {
		got, err := dbStore.ListMentions("")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeEmpty())
	})

	Context("with messages across sessions", func() {
		BeforeEach(func() {
			put("s2", 0, "carol", "I think #GraphDB fits, @alice?", 5)
			put("s1", 0, "alice", "Let's talk #graphdb and #nlp", 1)
			put("s1", 1, "bob", "#graphdb #graphdb again, cc @alice", 2)
			put("s1", 2, "bob", "no mentions here, mail bob@example.com", 3)
		})

		It("counts messages and sessions per mention, most-referenced first", func() {
			got, err := dbStore.ListMentions("")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(HaveLen(3))

			Expect(got[0].Kind).To(Equal(parser.MentionTag))
			Expect(got[0].Name).To(Equal("graphdb"))
			Expect(got[0].Messages).To(Equal(3))
			Expect(got[0].Sessions).To(Equal(2))

			Expect(got[1].Kind).To(Equal(parser.MentionUser))
			Expect(got[1].Name).To(Equal("alice"))
			Expect(got[1].Messages).To(Equal(2))
			Expect(got[1].Sessions).To(Equal(2))
			Expect(got[2].Name).To(Equal("nlp"))
			Expect(got[2].Messages).To(Equal(1))
		})

		It("sorts equal counts by kind then name", func() {
			put("s3", 0, "dave", "#zeta @nlp #nlp", 9)
			got, err := dbStore.ListMentions("")
			Expect(err).NotTo(HaveOccurred())
			var names []string
			for _, m := range got {
				names = append(names, string(m.Kind)+":"+m.Name)
			}
			Expect(names).To(Equal([]string{"tag:graphdb", "tag:nlp", "user:alice", "tag:zeta", "user:nlp"}))
		})

		It("records who introduced each entity, by time not scan order", func() {
			got, err := dbStore.ListMentions(parser.MentionTag)
			Expect(err).NotTo(HaveOccurred())
			Expect(got[0].Name).To(Equal("graphdb"))
			Expect(got[0].FirstSeen.User).To(Equal("alice"))
			Expect(got[0].FirstSeen.SessionID).To(Equal("s1"))
			Expect(got[0].FirstSeen.MessageID).To(Equal(0))
		})

		It("filters by kind", func() {
			got, err := dbStore.ListMentions(parser.MentionUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(HaveLen(1))
			Expect(got[0].Name).To(Equal("alice"))
		})

		It("lists the messages mentioning an entity, oldest first, case-insensitively", func() {
			got, err := dbStore.MessagesMentioning(parser.MentionTag, "GRAPHDB", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(HaveLen(3))
			Expect([]string{got[0].User, got[1].User, got[2].User}).To(Equal([]string{"alice", "bob", "carol"}))
		})

		It("applies the limit after ordering", func() {
			got, err := dbStore.MessagesMentioning(parser.MentionTag, "graphdb", 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(HaveLen(2))
			Expect(got[1].User).To(Equal("bob"))
		})

		It("returns an empty slice for an unmentioned entity", func() {
			got, err := dbStore.MessagesMentioning(parser.MentionUser, "nobody", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(BeEmpty())
		})

		It("rejects names that are not a single valid mention", func() {
			for _, bad := range []string{"", "42", "two words", "a#b"} {
				_, err := dbStore.MessagesMentioning(parser.MentionTag, bad, 0)
				Expect(err).To(MatchError(storage.ErrInvalidMention), bad)
			}
		})

		It("reflects edits and deletes immediately", func() {
			put("s1", 0, "alice", "edited: nothing tagged", 1)
			Expect(dbStore.DeleteSession("s2")).To(Succeed())
			got, err := dbStore.ListMentions(parser.MentionTag)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(HaveLen(1))
			Expect(got[0].Messages).To(Equal(1))
			Expect(got[0].FirstSeen.User).To(Equal("bob"))
		})
	})
})
