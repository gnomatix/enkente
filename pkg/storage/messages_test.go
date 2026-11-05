package storage_test

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

var _ = Describe("Chat Message Persistence", func() {
	var (
		dbPath  string
		dbStore *storage.BoltStorage
		ts      time.Time
	)

	BeforeEach(func() {
		dbPath = filepath.Join(GinkgoT().TempDir(), "messages.db")
		store, err := storage.NewBoltStorage(dbPath)
		Expect(err).NotTo(HaveOccurred())
		dbStore = store
		ts = time.Date(2025, 9, 4, 21, 27, 18, 909000000, time.UTC)
	})

	AfterEach(func() {
		Expect(dbStore.Close()).To(Succeed())
	})

	msg := func(session string, id int, text string) parser.AntigravityMessage {
		return parser.AntigravityMessage{
			SessionID: session,
			MessageID: id,
			Type:      "user",
			User:      "brett",
			Message:   text,
			Timestamp: ts,
		}
	}

	Describe("PutMessage / GetMessage", func() {
		It("round-trips a message including its timestamp", func() {
			Expect(dbStore.PutMessage(msg("s1", 0, "hello"))).To(Succeed())

			got, err := dbStore.GetMessage("s1", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(got.SessionID).To(Equal("s1"))
			Expect(got.MessageID).To(Equal(0))
			Expect(got.Type).To(Equal("user"))
			Expect(got.User).To(Equal("brett"))
			Expect(got.Message).To(Equal("hello"))
			Expect(got.Timestamp.Equal(ts)).To(BeTrue())
		})

		It("is idempotent: re-putting the same identity replaces rather than duplicates", func() {
			Expect(dbStore.PutMessage(msg("s1", 3, "first"))).To(Succeed())
			Expect(dbStore.PutMessage(msg("s1", 3, "revised"))).To(Succeed())

			list, err := dbStore.ListMessages("s1")
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(1))
			Expect(list[0].Message).To(Equal("revised"))
		})

		It("returns ErrNotFound for an unknown session or message", func() {
			_, err := dbStore.GetMessage("nope", 0)
			Expect(err).To(MatchError(storage.ErrNotFound))

			Expect(dbStore.PutMessage(msg("s1", 0, "hello"))).To(Succeed())
			_, err = dbStore.GetMessage("s1", 99)
			Expect(err).To(MatchError(storage.ErrNotFound))

			_, err = dbStore.GetMessage("s1", -1)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("rejects messages with an empty session or negative ID", func() {
			Expect(dbStore.PutMessage(msg("", 0, "x"))).NotTo(Succeed())
			Expect(dbStore.PutMessage(msg("s1", -1, "x"))).NotTo(Succeed())
		})
	})

	Describe("ListMessages", func() {
		It("returns messages in numeric message-ID order regardless of insertion order", func() {
			for _, id := range []int{300, 2, 1000, 0, 17} {
				Expect(dbStore.PutMessage(msg("s1", id, "m"))).To(Succeed())
			}

			list, err := dbStore.ListMessages("s1")
			Expect(err).NotTo(HaveOccurred())

			ids := make([]int, len(list))
			for i, m := range list {
				ids[i] = m.MessageID
			}
			Expect(ids).To(Equal([]int{0, 2, 17, 300, 1000}))
		})

		It("isolates sessions from one another", func() {
			Expect(dbStore.PutMessage(msg("a", 0, "in a"))).To(Succeed())
			Expect(dbStore.PutMessage(msg("b", 0, "in b"))).To(Succeed())

			listA, err := dbStore.ListMessages("a")
			Expect(err).NotTo(HaveOccurred())
			Expect(listA).To(HaveLen(1))
			Expect(listA[0].Message).To(Equal("in a"))
		})

		It("returns ErrNotFound for an unknown session", func() {
			_, err := dbStore.ListMessages("missing")
			Expect(err).To(MatchError(storage.ErrNotFound))
		})
	})

	Describe("ListMessagesAfter", func() {
		ids := func(list []parser.AntigravityMessage) []int {
			out := make([]int, len(list))
			for i, m := range list {
				out[i] = m.MessageID
			}
			return out
		}

		BeforeEach(func() {
			for _, id := range []int{0, 1, 2, 5, 9} {
				Expect(dbStore.PutMessage(msg("s1", id, "m"))).To(Succeed())
			}
		})

		It("returns only messages with an ID greater than afterID", func() {
			list, err := dbStore.ListMessagesAfter("s1", 2, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(ids(list)).To(Equal([]int{5, 9}))
		})

		It("seeks past gaps: afterID need not be an existing ID", func() {
			list, err := dbStore.ListMessagesAfter("s1", 3, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(ids(list)).To(Equal([]int{5, 9}))
		})

		It("applies the limit", func() {
			list, err := dbStore.ListMessagesAfter("s1", -1, 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(ids(list)).To(Equal([]int{0, 1}))
		})

		It("returns an empty slice, not nil, when nothing is newer", func() {
			list, err := dbStore.ListMessagesAfter("s1", 9, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(list).NotTo(BeNil())
			Expect(list).To(BeEmpty())
		})

		It("matches ListMessages when started from the beginning without a limit", func() {
			all, err := dbStore.ListMessages("s1")
			Expect(err).NotTo(HaveOccurred())
			after, err := dbStore.ListMessagesAfter("s1", -1, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(all))
		})

		It("returns ErrNotFound for an unknown session", func() {
			_, err := dbStore.ListMessagesAfter("missing", -1, 0)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})
	})

	Describe("AppendMessage", func() {
		It("assigns strictly increasing IDs within a session and returns the stored message", func() {
			first, err := dbStore.AppendMessage(msg("live", 0, "one"))
			Expect(err).NotTo(HaveOccurred())
			second, err := dbStore.AppendMessage(msg("live", 0, "two"))
			Expect(err).NotTo(HaveOccurred())

			Expect(second.MessageID).To(BeNumerically(">", first.MessageID))

			list, err := dbStore.ListMessages("live")
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(2))
			Expect(list[0].Message).To(Equal("one"))
			Expect(list[1].Message).To(Equal("two"))
		})

		It("ignores a caller-supplied MessageID", func() {
			first, err := dbStore.AppendMessage(msg("live", 500, "one"))
			Expect(err).NotTo(HaveOccurred())
			Expect(first.MessageID).NotTo(Equal(500))
		})

		It("never collides with IDs previously written by PutMessage", func() {
			Expect(dbStore.PutMessage(msg("mixed", 41, "from log"))).To(Succeed())

			appended, err := dbStore.AppendMessage(msg("mixed", 0, "from api"))
			Expect(err).NotTo(HaveOccurred())
			Expect(appended.MessageID).To(BeNumerically(">", 41))

			list, err := dbStore.ListMessages("mixed")
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(2))
		})

		It("keeps per-session counters independent", func() {
			a1, err := dbStore.AppendMessage(msg("a", 0, "x"))
			Expect(err).NotTo(HaveOccurred())
			b1, err := dbStore.AppendMessage(msg("b", 0, "y"))
			Expect(err).NotTo(HaveOccurred())
			Expect(a1.MessageID).To(Equal(b1.MessageID))
		})

		It("survives reopening the database", func() {
			first, err := dbStore.AppendMessage(msg("live", 0, "one"))
			Expect(err).NotTo(HaveOccurred())
			Expect(dbStore.Close()).To(Succeed())

			reopened, err := storage.NewBoltStorage(dbPath)
			Expect(err).NotTo(HaveOccurred())
			dbStore = reopened

			second, err := dbStore.AppendMessage(msg("live", 0, "two"))
			Expect(err).NotTo(HaveOccurred())
			Expect(second.MessageID).To(BeNumerically(">", first.MessageID))

			list, err := dbStore.ListMessages("live")
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(2))
		})
	})

	Describe("ListSessions", func() {
		It("is empty for a fresh database", func() {
			sessions, err := dbStore.ListSessions()
			Expect(err).NotTo(HaveOccurred())
			Expect(sessions).To(BeEmpty())
		})

		It("lists each session with its message count, ordered by session ID", func() {
			Expect(dbStore.PutMessage(msg("zeta", 0, "z"))).To(Succeed())
			Expect(dbStore.PutMessage(msg("alpha", 0, "a"))).To(Succeed())
			Expect(dbStore.PutMessage(msg("alpha", 1, "a"))).To(Succeed())
			// A plain key in the ChatLogs bucket must not be mistaken for a session.
			Expect(dbStore.Put(storage.ChatBucket, "not-a-session", []byte("x"))).To(Succeed())

			sessions, err := dbStore.ListSessions()
			Expect(err).NotTo(HaveOccurred())
			Expect(sessions).To(Equal([]storage.SessionInfo{
				{SessionID: "alpha", MessageCount: 2},
				{SessionID: "zeta", MessageCount: 1},
			}))
		})
	})
})
