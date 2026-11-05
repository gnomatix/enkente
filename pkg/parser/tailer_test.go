package parser_test

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

		// The handler runs on several worker goroutines, so the counter must
		// be atomic for the test to be race-free.
		var processed atomic.Int32
		processedCount := func() int { return int(processed.Load()) }
		handler := func(workerID int, msg parser.AntigravityMessage) {
			processed.Add(1)
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

		Eventually(processedCount, "1s").Should(Equal(1))

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

		Eventually(processedCount, "1s").Should(Equal(2))

		// Ensure no more messages are ready right now
		Consistently(processedCount, "200ms").Should(Equal(2))
	})

	It("re-emits everything when the file is replaced by a different log of the same length", func() {
		logA := `[
			{"sessionId":"A","messageId":0,"type":"user","message":"a0"},
			{"sessionId":"A","messageId":1,"type":"system","message":"a1"}
		]`
		logB := `[
			{"sessionId":"B","messageId":0,"type":"user","message":"b0"},
			{"sessionId":"B","messageId":1,"type":"system","message":"b1"}
		]`
		Expect(os.WriteFile(tempFile, []byte(logA), 0644)).To(Succeed())

		var mu sync.Mutex
		var seen []string
		handler := func(workerID int, msg parser.AntigravityMessage) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, msg.SessionID+"/"+msg.Message)
		}
		count := func() int { mu.Lock(); defer mu.Unlock(); return len(seen) }

		Expect(parser.TailChatLog(tempFile, 20*time.Millisecond, 1, handler, done)).To(Succeed())
		Eventually(count, "1s").Should(Equal(2))

		// Same byte length, same message count, different session: the old
		// length-only check would have emitted nothing here.
		time.Sleep(50 * time.Millisecond)
		Expect(os.WriteFile(tempFile, []byte(logB), 0644)).To(Succeed())
		Eventually(count, "1s").Should(Equal(4))
		Consistently(count, "200ms").Should(Equal(4))

		mu.Lock()
		defer mu.Unlock()
		Expect(seen).To(Equal([]string{"A/a0", "A/a1", "B/b0", "B/b1"}))
	})

	It("emits only the appended messages when a log grows", func() {
		Expect(os.WriteFile(tempFile, []byte(`[{"sessionId":"A","messageId":0,"type":"user","message":"a0"}]`), 0644)).To(Succeed())

		var processed atomic.Int32
		handler := func(workerID int, msg parser.AntigravityMessage) { processed.Add(1) }
		count := func() int { return int(processed.Load()) }

		Expect(parser.TailChatLog(tempFile, 20*time.Millisecond, 2, handler, done)).To(Succeed())
		Eventually(count, "1s").Should(Equal(1))

		time.Sleep(50 * time.Millisecond)
		Expect(os.WriteFile(tempFile, []byte(`[
			{"sessionId":"A","messageId":0,"type":"user","message":"a0"},
			{"sessionId":"A","messageId":1,"type":"user","message":"a1"},
			{"sessionId":"A","messageId":2,"type":"user","message":"a2"}
		]`), 0644)).To(Succeed())
		Eventually(count, "1s").Should(Equal(3))
		Consistently(count, "200ms").Should(Equal(3))
	})
})
