package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

// resetFlags restores every subcommand flag to its default and clears its
// Changed bit. cobra keeps flag state in package-level variables across
// Execute calls, so without this a value (or a satisfied "required" mark)
// from one spec would leak into the next.
func resetFlags() {
	for _, c := range rootCmd.Commands() {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
}

// run executes the root command with args, capturing stdout and stderr.
func run(args ...string) (string, error) {
	resetFlags()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

var _ = Describe("Headless datastore commands", func() {
	var (
		dir     string
		dbPath  string
		logPath string
	)

	const antigravityLog = `[
		{"sessionId":"1ee3cafd","messageId":0,"type":"user","user":"brett","message":"Are you able to retrieve and process an RSS feed?","timestamp":"2025-09-04T21:27:18.909Z"},
		{"sessionId":"1ee3cafd","messageId":1,"type":"system","message":"Yes, I can!","timestamp":"2025-09-04T21:28:05.069Z"},
		{"sessionId":"other","messageId":0,"type":"user","user":"brett","message":"Different session","timestamp":"2025-09-05T10:00:00Z"}
	]`

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		dbPath = filepath.Join(dir, "cli.db")
		logPath = filepath.Join(dir, "logs.json")
		Expect(os.WriteFile(logPath, []byte(antigravityLog), 0o644)).To(Succeed())
	})

	Describe("import", func() {
		It("stores every message under its own session and reports counts", func() {
			out, err := run("import", "--log", logPath, "--db", dbPath, "--session", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("imported 3 messages into 2 session(s)"))

			store, err := storage.NewBoltStorage(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer store.Close()

			sessions, err := store.ListSessions()
			Expect(err).NotTo(HaveOccurred())
			Expect(sessions).To(Equal([]storage.SessionInfo{
				{SessionID: "1ee3cafd", MessageCount: 2},
				{SessionID: "other", MessageCount: 1},
			}))

			msg, err := store.GetMessage("1ee3cafd", 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Message).To(Equal("Yes, I can!"))
		})

		It("is idempotent when the same log is imported twice", func() {
			_, err := run("import", "--log", logPath, "--db", dbPath, "--session", "")
			Expect(err).NotTo(HaveOccurred())
			_, err = run("import", "--log", logPath, "--db", dbPath, "--session", "")
			Expect(err).NotTo(HaveOccurred())

			out, err := run("sessions", "--db", dbPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(MatchRegexp(`1ee3cafd\s+2\n`))
			Expect(out).To(MatchRegexp(`other\s+1\n`))
		})

		It("assigns --session to a log whose messages carry no sessionId", func() {
			noSession := filepath.Join(dir, "nosession.json")
			Expect(os.WriteFile(noSession, []byte(`[
				{"messageId":0,"type":"user","message":"a"},
				{"messageId":1,"type":"system","message":"b"}
			]`), 0o644)).To(Succeed())

			out, err := run("import", "--log", noSession, "--db", dbPath, "--session", "assigned")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("imported 2 messages into 1 session(s)"))

			store, err := storage.NewBoltStorage(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer store.Close()
			sessions, err := store.ListSessions()
			Expect(err).NotTo(HaveOccurred())
			Expect(sessions).To(Equal([]storage.SessionInfo{{SessionID: "assigned", MessageCount: 2}}))
		})

		It("refuses a log without sessionIds when --session is not given, creating no datastore", func() {
			noSession := filepath.Join(dir, "nosession.json")
			Expect(os.WriteFile(noSession, []byte(`[
				{"sessionId":"s","messageId":0,"type":"user","message":"a"},
				{"messageId":1,"type":"system","message":"b"}
			]`), 0o644)).To(Succeed())

			_, err := run("import", "--log", noSession, "--db", dbPath)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("message 1"))
			Expect(err.Error()).To(ContainSubstring("--session"))
			_, statErr := os.Stat(dbPath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It("refuses to collapse several sessions into one when messageIds would collide", func() {
			_, err := run("import", "--log", logPath, "--db", dbPath, "--session", "merged")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("would both be stored as session \"merged\" messageId 0"))
			_, statErr := os.Stat(dbPath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It("fails with a parse error for a malformed log and writes nothing", func() {
			bad := filepath.Join(dir, "bad.json")
			Expect(os.WriteFile(bad, []byte(`{not json`), 0o644)).To(Succeed())

			_, err := run("import", "--log", bad, "--db", dbPath)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse"))
			_, statErr := os.Stat(dbPath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It("treats an empty log as a no-op without creating a datastore", func() {
			empty := filepath.Join(dir, "empty.json")
			Expect(os.WriteFile(empty, []byte(`[]`), 0o644)).To(Succeed())

			out, err := run("import", "--log", empty, "--db", dbPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("nothing imported"))
			_, statErr := os.Stat(dbPath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It("requires --log even after an earlier spec supplied it", func() {
			_, err := run("import", "--log", logPath, "--db", dbPath)
			Expect(err).NotTo(HaveOccurred())

			_, err = run("import", "--db", filepath.Join(dir, "other.db"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`required flag(s) "log" not set`))
			_, statErr := os.Stat(filepath.Join(dir, "other.db"))
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})
	})

	Describe("export", func() {
		BeforeEach(func() {
			_, err := run("import", "--log", logPath, "--db", dbPath, "--session", "")
			Expect(err).NotTo(HaveOccurred())
		})

		It("exports one session as a re-ingestable Antigravity array", func() {
			out, err := run("export", "--db", dbPath, "--session", "1ee3cafd", "--compact=false")
			Expect(err).NotTo(HaveOccurred())

			var msgs []parser.AntigravityMessage
			Expect(json.Unmarshal([]byte(out), &msgs)).To(Succeed())
			Expect(msgs).To(HaveLen(2))
			Expect(msgs[0].MessageID).To(Equal(0))
			Expect(msgs[1].MessageID).To(Equal(1))
			Expect(msgs[1].Message).To(Equal("Yes, I can!"))

			// Round trip: the export parses with the log parser and re-imports.
			exported := filepath.Join(dir, "export.json")
			Expect(os.WriteFile(exported, []byte(out), 0o644)).To(Succeed())
			parsed, err := parser.ParseChatLog(exported)
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed).To(HaveLen(2))

			second := filepath.Join(dir, "second.db")
			res, err := run("import", "--log", exported, "--db", second, "--session", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(ContainSubstring("imported 2 messages into 1 session(s)"))
		})

		It("exports every session, ordered by session then message ID, compactly on request", func() {
			out, err := run("export", "--db", dbPath, "--session", "", "--compact")
			Expect(err).NotTo(HaveOccurred())
			Expect(bytes.Count([]byte(out), []byte("\n"))).To(Equal(1))

			var msgs []parser.AntigravityMessage
			Expect(json.Unmarshal([]byte(out), &msgs)).To(Succeed())
			Expect(msgs).To(HaveLen(3))
			Expect(msgs[0].SessionID).To(Equal("1ee3cafd"))
			Expect(msgs[2].SessionID).To(Equal("other"))
		})

		It("fails for an unknown session", func() {
			_, err := run("export", "--db", dbPath, "--session", "missing", "--compact=false")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing"))
		})
	})

	Describe("sessions", func() {
		It("prints a header only for an empty datastore", func() {
			out, err := run("sessions", "--db", dbPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(MatchRegexp(`^SESSION\s+MESSAGES\n$`))
		})
	})
})
