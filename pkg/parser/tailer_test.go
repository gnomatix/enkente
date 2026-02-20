package parser_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
)

var _ = Describe("Live Tailer", func() {
	var (
		tempFile string
		done     chan struct{}
	)

	BeforeEach(func() {
		tempDir := GinkgoT().TempDir()
		tempFile = filepath.Join(tempDir, "tail_test_logs.json")
		done = make(chan struct{})
	})

	AfterEach(func() {
		close(done)
	})

	It("streams new messages as the file is updated processing them via a worker pool", func() {
		// Initial state: empty array
		err := os.WriteFile(tempFile, []byte("[]"), 0644)
		Expect(err).NotTo(HaveOccurred())

		processedCount := 0
		handler := func(workerID int, msg parser.AntigravityMessage) {
			processedCount++
		}

		err = parser.TailChatLog(tempFile, 50*time.Millisecond, 2, handler, done)
		Expect(err).NotTo(HaveOccurred())

		// Write a single message
		testData1 := `[
			{
				"sessionId": "123",
				"messageId": 0,
				"type": "user",
				"message": "First message"
			}
		]`
		time.Sleep(100 * time.Millisecond) // Ensure modtime differs
		err = os.WriteFile(tempFile, []byte(testData1), 0644)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int { return processedCount }, "1s").Should(Equal(1))

		// Write a second message (extending the array)
		testData2 := `[
			{
				"sessionId": "123",
				"messageId": 0,
				"type": "user",
				"message": "First message"
			},
			{
				"sessionId": "123",
				"messageId": 1,
				"type": "system",
				"message": "Second message"
			}
		]`
		time.Sleep(100 * time.Millisecond)
		err = os.WriteFile(tempFile, []byte(testData2), 0644)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int { return processedCount }, "1s").Should(Equal(2))

		// Ensure no more messages are ready right now
		Consistently(func() int { return processedCount }, "200ms").Should(Equal(2))
	})
})
