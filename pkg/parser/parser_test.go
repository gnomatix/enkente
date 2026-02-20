package parser_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
)

var _ = Describe("JSON Parser", func() {
	var (
		tempFile string
	)

	BeforeEach(func() {
		testData := `[
			{
				"sessionId": "1ee3cafd-cbc4-414f-b0b4-8639c1653cd1",
				"messageId": 0,
				"type": "user",
				"message": "Are you able to retrieve and process an RSS feed?",
				"timestamp": "2025-09-04T21:27:18.909Z"
			},
			{
				"sessionId": "1ee3cafd-cbc4-414f-b0b4-8639c1653cd1",
				"messageId": 1,
				"type": "system",
				"message": "Yes, I can!",
				"timestamp": "2025-09-04T21:28:05.069Z"
			}
		]`

		tempDir := GinkgoT().TempDir()
		tempFile = filepath.Join(tempDir, "test_logs.json")
		err := os.WriteFile(tempFile, []byte(testData), 0644)
		Expect(err).NotTo(HaveOccurred())
	})

	It("successfully parses a valid Antigravity JSON log", func() {
		messages, err := parser.ParseChatLog(tempFile)

		Expect(err).NotTo(HaveOccurred())
		Expect(messages).To(HaveLen(2))

		Expect(messages[0].SessionID).To(Equal("1ee3cafd-cbc4-414f-b0b4-8639c1653cd1"))
		Expect(messages[0].MessageID).To(Equal(0))
		Expect(messages[0].Type).To(Equal("user"))
		Expect(messages[0].Message).To(Equal("Are you able to retrieve and process an RSS feed?"))

		expectedTime, _ := time.Parse(time.RFC3339Nano, "2025-09-04T21:27:18.909Z")
		Expect(messages[0].Timestamp.Equal(expectedTime)).To(BeTrue())

		Expect(messages[1].Type).To(Equal("system"))
		Expect(messages[1].Message).To(Equal("Yes, I can!"))
	})

	It("returns an error for a non-existent file", func() {
		_, err := parser.ParseChatLog("does_not_exist.json")
		Expect(err).To(HaveOccurred())
	})
})
