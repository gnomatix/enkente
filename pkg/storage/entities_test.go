package storage_test

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

var _ = Describe("Entities", func() {
	var (
		dbStore *storage.BoltStorage
		now     time.Time
	)

	BeforeEach(func() {
		store, err := storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "entities.db"))
		Expect(err).NotTo(HaveOccurred())
		dbStore = store
		now = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	})

	AfterEach(func() {
		Expect(dbStore.Close()).To(Succeed())
	})

	It("round-trips a curated record with a normalized name", func() {
		saved, err := dbStore.PutEntity(storage.Entity{
			Kind: parser.MentionTag, Name: "GraphDB",
			Description: "property graph store",
			Attributes:  map[string]string{"owner": "alice"},
			Notes:       []storage.EntityNote{{Text: "chosen in sprint 3", Author: "bob"}},
		}, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Name).To(Equal("graphdb"))
		Expect(saved.Notes[0].AddedAt).To(Equal(now))

		got, err := dbStore.GetEntity(parser.MentionTag, "graphdb")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Description).To(Equal("property graph store"))
		Expect(got.Attributes).To(HaveKeyWithValue("owner", "alice"))
		Expect(got.UpdatedAt).To(Equal(now))
	})

	It("appends notes, creating the record on first note", func() {
		_, err := dbStore.AddEntityNote(parser.MentionUser, "Alice", storage.EntityNote{Text: "leads NLP"}, now)
		Expect(err).NotTo(HaveOccurred())
		e, err := dbStore.AddEntityNote(parser.MentionUser, "alice", storage.EntityNote{Text: "prefers Go", Author: "bob"}, now.Add(time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(e.Notes).To(HaveLen(2))
		Expect(e.Notes[1].Author).To(Equal("bob"))
		Expect(e.UpdatedAt).To(Equal(now.Add(time.Hour)))
	})

	It("rejects invalid names, kinds and empty notes", func() {
		_, err := dbStore.PutEntity(storage.Entity{Kind: parser.MentionTag, Name: "two words"}, now)
		Expect(err).To(MatchError(storage.ErrInvalidMention))
		_, err = dbStore.PutEntity(storage.Entity{Kind: "topic", Name: "x"}, now)
		Expect(err).To(MatchError(storage.ErrInvalidMention))
		_, err = dbStore.AddEntityNote(parser.MentionTag, "x", storage.EntityNote{Text: "  "}, now)
		Expect(err).To(HaveOccurred())
		_, err = dbStore.PutEntity(storage.Entity{Kind: parser.MentionTag, Name: "x", Notes: []storage.EntityNote{{}}}, now)
		Expect(err).To(HaveOccurred())
	})

	It("deletes a record and reports ErrNotFound afterwards", func() {
		_, err := dbStore.PutEntity(storage.Entity{Kind: parser.MentionTag, Name: "nlp"}, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(dbStore.DeleteEntity(parser.MentionTag, "nlp")).To(Succeed())
		_, err = dbStore.GetEntity(parser.MentionTag, "nlp")
		Expect(err).To(MatchError(storage.ErrNotFound))
		Expect(dbStore.DeleteEntity(parser.MentionTag, "nlp")).To(MatchError(storage.ErrNotFound))
	})

	It("lists records by kind then name, ignoring legacy plain keys", func() {
		Expect(dbStore.Put(storage.ConceptBucket, "legacy", []byte("x"))).To(Succeed())
		for _, e := range []storage.Entity{
			{Kind: parser.MentionUser, Name: "bob"},
			{Kind: parser.MentionTag, Name: "zeta"},
			{Kind: parser.MentionTag, Name: "alpha"},
		} {
			_, err := dbStore.PutEntity(e, now)
			Expect(err).NotTo(HaveOccurred())
		}
		list, err := dbStore.ListEntities()
		Expect(err).NotTo(HaveOccurred())
		var names []string
		for _, e := range list {
			names = append(names, string(e.Kind)+":"+e.Name)
		}
		Expect(names).To(Equal([]string{"tag:alpha", "tag:zeta", "user:bob"}))
	})

	Describe("ViewEntity", func() {
		BeforeEach(func() {
			Expect(dbStore.PutMessage(parser.AntigravityMessage{
				SessionID: "s1", MessageID: 0, Type: "user", User: "carol",
				Message: "intro to #graphdb", Timestamp: now,
			})).To(Succeed())
		})

		It("joins mentions with the curated record", func() {
			_, err := dbStore.PutEntity(storage.Entity{Kind: parser.MentionTag, Name: "graphdb", Description: "d"}, now)
			Expect(err).NotTo(HaveOccurred())
			v, err := dbStore.ViewEntity(parser.MentionTag, "GRAPHDB")
			Expect(err).NotTo(HaveOccurred())
			Expect(v.Curated.Description).To(Equal("d"))
			Expect(v.Mentions.FirstSeen.User).To(Equal("carol"))
		})

		It("works for mentioned-only and curated-only entities", func() {
			v, err := dbStore.ViewEntity(parser.MentionTag, "graphdb")
			Expect(err).NotTo(HaveOccurred())
			Expect(v.Curated).To(BeNil())
			Expect(v.Mentions.Messages).To(Equal(1))

			_, err = dbStore.PutEntity(storage.Entity{Kind: parser.MentionUser, Name: "dave"}, now)
			Expect(err).NotTo(HaveOccurred())
			v, err = dbStore.ViewEntity(parser.MentionUser, "dave")
			Expect(err).NotTo(HaveOccurred())
			Expect(v.Mentions).To(BeNil())
		})

		It("returns ErrNotFound when neither curated nor mentioned", func() {
			_, err := dbStore.ViewEntity(parser.MentionTag, "unknown")
			Expect(err).To(MatchError(storage.ErrNotFound))
		})
	})
})
