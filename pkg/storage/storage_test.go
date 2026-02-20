package storage_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/storage"
)

var _ = Describe("Embedded Storage Engine", func() {
	var (
		dbPath  string
		dbStore *storage.BoltStorage
	)

	BeforeEach(func() {
		tempDir := GinkgoT().TempDir()
		dbPath = filepath.Join(tempDir, "test.db")

		store, err := storage.NewBoltStorage(dbPath)
		Expect(err).NotTo(HaveOccurred())
		dbStore = store
	})

	AfterEach(func() {
		if dbStore != nil {
			err := dbStore.Close()
			Expect(err).NotTo(HaveOccurred())
		}
		_ = os.Remove(dbPath)
	})

	It("starts up and creates the necessary buckets", func() {
		// Verify we can put data into the ChatBucket
		err := dbStore.Put(storage.ChatBucket, "test-id", []byte("hello world"))
		Expect(err).NotTo(HaveOccurred())

		data, err := dbStore.Get(storage.ChatBucket, "test-id")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("hello world"))
	})

	It("handles missing keys gracefully", func() {
		data, err := dbStore.Get(storage.ConceptBucket, "does-not-exist")
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(BeNil())
	})

	It("returns errors for invalid buckets", func() {
		err := dbStore.Put("InvalidBucket", "key", []byte("data"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))

		_, err = dbStore.Get("InvalidBucket", "key")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})
})
