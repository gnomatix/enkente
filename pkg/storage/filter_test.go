package storage_test

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

var _ = Describe("ListMessagesFiltered", func() {
	var (
		dbStore *storage.BoltStorage
		base    time.Time
	)

	BeforeEach(func() {
		store, err := storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "filter.db"))
		Expect(err).NotTo(HaveOccurred())
		dbStore = store
		base = time.Date(2025, 9, 4, 21, 0, 0, 0, time.UTC)
		for i, user := range []string{"alice", "Bob", "alice", "carol", "alice"} {
			Expect(dbStore.PutMessage(parser.AntigravityMessage{
				SessionID: "s1", MessageID: i, Type: "user", User: user,
				Message: "m", Timestamp: base.Add(time.Duration(i) * time.Minute),
			})).To(Succeed())
		}
	})

	AfterEach(func() {
		Expect(dbStore.Close()).To(Succeed())
	})

	list := func(after, limit int, f storage.MessageFilter) []int {
		msgs, err := dbStore.ListMessagesFiltered("s1", after, limit, f)
		Expect(err).NotTo(HaveOccurred())
		out := []int{}
		for _, m := range msgs {
			out = append(out, m.MessageID)
		}
		return out
	}

	It("matches everything with a zero filter", func() {
		Expect(list(-1, 0, storage.MessageFilter{})).To(Equal([]int{0, 1, 2, 3, 4}))
	})

	It("filters by user case-insensitively", func() {
		Expect(list(-1, 0, storage.MessageFilter{User: "ALICE"})).To(Equal([]int{0, 2, 4}))
		Expect(list(-1, 0, storage.MessageFilter{User: "bob"})).To(Equal([]int{1}))
	})

	It("treats since as inclusive and until as exclusive", func() {
		f := storage.MessageFilter{Since: base.Add(1 * time.Minute), Until: base.Add(3 * time.Minute)}
		Expect(list(-1, 0, f)).To(Equal([]int{1, 2}))
	})

	It("splits a range into adjacent windows without overlap", func() {
		mid := base.Add(2 * time.Minute)
		a := list(-1, 0, storage.MessageFilter{Until: mid})
		b := list(-1, 0, storage.MessageFilter{Since: mid})
		Expect(append(a, b...)).To(Equal([]int{0, 1, 2, 3, 4}))
	})

	It("counts the limit over matching messages only", func() {
		Expect(list(-1, 2, storage.MessageFilter{User: "alice"})).To(Equal([]int{0, 2}))
	})

	It("combines with afterID for incremental polling", func() {
		Expect(list(0, 0, storage.MessageFilter{User: "alice"})).To(Equal([]int{2, 4}))
	})

	It("still reports ErrNotFound for an unknown session", func() {
		_, err := dbStore.ListMessagesFiltered("missing", -1, 0, storage.MessageFilter{User: "alice"})
		Expect(err).To(MatchError(storage.ErrNotFound))
	})
})
