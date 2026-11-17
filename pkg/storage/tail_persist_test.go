package storage_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

// This spec drives the real log tailer into the store, mirroring what
// `enkente tail --db` does, to verify the ingestion-to-storage path end to
// end: ordering is preserved, concurrent workers do not corrupt the session,
// and replaying a log (which the tailer does after truncation or restart)
// is idempotent rather than duplicating rows.
var _ = Describe("Tailer to storage persistence", func() {
	var (
		dbPath  string
		logPath string
		dbStore *storage.BoltStorage
		done    chan struct{}
	)

	const logV1 = `[
		{"sessionId":"s","messageId":0,"type":"user","user":"alice","message":"zero","timestamp":"2025-09-04T21:27:18.909Z"},
		{"sessionId":"s","messageId":1,"type":"system","message":"one","timestamp":"2025-09-04T21:28:05.069Z"}
	]`
	const logV2 = `[
		{"sessionId":"s","messageId":0,"type":"user","user":"alice","message":"zero","timestamp":"2025-09-04T21:27:18.909Z"},
		{"sessionId":"s","messageId":1,"type":"system","message":"one","timestamp":"2025-09-04T21:28:05.069Z"},
		{"sessionId":"s","messageId":2,"type":"user","user":"alice","message":"two","timestamp":"2025-09-04T21:29:00Z"}
	]`

	BeforeEach(func() {
		dir := GinkgoT().TempDir()
		dbPath = filepath.Join(dir, "tail.db")
		logPath = filepath.Join(dir, "logs.json")
		done = make(chan struct{})

		store, err := storage.NewBoltStorage(dbPath)
		Expect(err).NotTo(HaveOccurred())
		dbStore = store
	})

	AfterEach(func() {
		close(done)
		Expect(dbStore.Close()).To(Succeed())
	})

	countMessages := func() int {
		msgs, err := dbStore.ListMessages("s")
		if err != nil {
			return 0
		}
		return len(msgs)
	}

	It("persists tailed messages in order and tolerates replay without duplicating", func() {
		Expect(os.WriteFile(logPath, []byte(logV1), 0o644)).To(Succeed())

		handler := func(_ int, msg parser.AntigravityMessage) {
			Expect(dbStore.PutMessage(msg)).To(Succeed())
		}
		Expect(parser.TailChatLog(logPath, 20*time.Millisecond, 3, handler, done)).To(Succeed())

		Eventually(countMessages, "2s").Should(Equal(2))

		// Append a message to the log: only it should be added.
		time.Sleep(50 * time.Millisecond) // ensure a distinct modtime
		Expect(os.WriteFile(logPath, []byte(logV2), 0o644)).To(Succeed())
		Eventually(countMessages, "2s").Should(Equal(3))

		// Simulate rotation: truncate then rewrite the full log. The tailer
		// replays every message; the store must end up with the same three.
		time.Sleep(50 * time.Millisecond)
		Expect(os.WriteFile(logPath, []byte("[]"), 0o644)).To(Succeed())
		time.Sleep(50 * time.Millisecond)
		Expect(os.WriteFile(logPath, []byte(logV2), 0o644)).To(Succeed())
		Consistently(countMessages, "300ms").Should(Equal(3))

		msgs, err := dbStore.ListMessages("s")
		Expect(err).NotTo(HaveOccurred())
		Expect(msgs[0].Message).To(Equal("zero"))
		Expect(msgs[1].Message).To(Equal("one"))
		Expect(msgs[2].Message).To(Equal("two"))
		Expect(msgs[0].Timestamp.Equal(time.Date(2025, 9, 4, 21, 27, 18, 909000000, time.UTC))).To(BeTrue())

		// A later API-style append must land after the log-imported IDs.
		appended, err := dbStore.AppendMessage(parser.AntigravityMessage{SessionID: "s", Type: "user", Message: "api"})
		Expect(err).NotTo(HaveOccurred())
		Expect(appended.MessageID).To(Equal(3))
	})
})
