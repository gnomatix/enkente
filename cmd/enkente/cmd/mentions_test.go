package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

var _ = Describe("mentions command", func() {
	var dbPath string

	BeforeEach(func() {
		dbPath = filepath.Join(GinkgoT().TempDir(), "m.db")
		store, err := storage.NewBoltStorage(dbPath)
		Expect(err).NotTo(HaveOccurred())
		base := time.Date(2025, 9, 4, 21, 0, 0, 0, time.UTC)
		Expect(store.PutMessages([]parser.AntigravityMessage{
			{SessionID: "s1", MessageID: 0, Type: "user", User: "alice", Message: "#GraphDB with @bob", Timestamp: base},
			{SessionID: "s1", MessageID: 1, Type: "user", User: "bob", Message: "#graphdb and #nlp", Timestamp: base.Add(time.Minute)},
		})).To(Succeed())
		Expect(store.Close()).To(Succeed())
	})

	It("prints a summary table, most-mentioned first", func() {
		out, err := run("mentions", "--db", dbPath)
		Expect(err).NotTo(HaveOccurred())
		lines := strings.Split(strings.TrimSpace(out), "\n")
		Expect(lines).To(HaveLen(4))
		Expect(lines[0]).To(HavePrefix("MENTION"))
		Expect(strings.Fields(lines[1])).To(Equal([]string{"#graphdb", "2", "1", "alice", "2025-09-04T21:00:00Z"}))
	})

	It("filters the summary by kind and limit", func() {
		out, err := run("mentions", "--db", dbPath, "--kind", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("@bob"))
		Expect(out).NotTo(ContainSubstring("#graphdb"))

		out, err = run("mentions", "--db", dbPath, "--limit", "1")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSpace(out), "\n")).To(HaveLen(2))
	})

	It("prints the messages mentioning one entity as JSON", func() {
		out, err := run("mentions", "--db", dbPath, "#GRAPHDB")
		Expect(err).NotTo(HaveOccurred())
		var msgs []parser.AntigravityMessage
		Expect(json.Unmarshal([]byte(out), &msgs)).To(Succeed())
		Expect(msgs).To(HaveLen(2))
		Expect(msgs[0].User).To(Equal("alice"))
	})

	It("rejects malformed arguments", func() {
		_, err := run("mentions", "--db", dbPath, "graphdb")
		Expect(err).To(MatchError(ContainSubstring("not a mention")))
		_, err = run("mentions", "--db", dbPath, "#42")
		Expect(err).To(MatchError(storage.ErrInvalidMention))
		_, err = run("mentions", "--db", dbPath, "--kind", "topic")
		Expect(err).To(HaveOccurred())
		_, err = run("mentions", "--db", dbPath, "--limit", "-1")
		Expect(err).To(HaveOccurred())
	})
})
