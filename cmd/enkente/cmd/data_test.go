package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

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
		{"sessionId":"1ee3cafd","messageId":0,"type":"user","user":"alice","message":"Are you able to retrieve and process an RSS feed?","timestamp":"2025-09-04T21:27:18.909Z"},
		{"sessionId":"1ee3cafd","messageId":1,"type":"system","message":"Yes, I can!","timestamp":"2025-09-04T21:28:05.069Z"},
		{"sessionId":"other","messageId":0,"type":"user","user":"alice","message":"Different session","timestamp":"2025-09-05T10:00:00Z"}
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

		Describe("--format", func() {
			It("defaults to the indented JSON array when --format is not given", func() {
				out, err := run("export", "--db", dbPath, "--session", "1ee3cafd")
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(HavePrefix("[\n  {\n    \"sessionId\": \"1ee3cafd\""))
				Expect(out).To(HaveSuffix("}\n]\n"))

				var msgs []parser.AntigravityMessage
				Expect(json.Unmarshal([]byte(out), &msgs)).To(Succeed())
				Expect(msgs).To(HaveLen(2))
			})

			It("still honours --compact for the explicit json format", func() {
				out, err := run("export", "--db", dbPath, "--session", "", "--format", "json", "--compact")
				Expect(err).NotTo(HaveOccurred())
				Expect(bytes.Count([]byte(out), []byte("\n"))).To(Equal(1))
				Expect(out).To(HavePrefix("[{"))
			})

			It("writes jsonl as one parseable compact object per line", func() {
				out, err := run("export", "--db", dbPath, "--session", "", "--format", "jsonl")
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(HaveSuffix("\n"))
				Expect(out).NotTo(HavePrefix("["))

				lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
				Expect(lines).To(HaveLen(3))

				var msgs []parser.AntigravityMessage
				for _, line := range lines {
					Expect(line).To(HavePrefix("{"))
					Expect(line).To(HaveSuffix("}"), "no trailing comma or array syntax on %q", line)
					var msg parser.AntigravityMessage
					Expect(json.Unmarshal([]byte(line), &msg)).To(Succeed(), "line %q", line)
					msgs = append(msgs, msg)
				}
				Expect(msgs[0].SessionID).To(Equal("1ee3cafd"))
				Expect(msgs[0].MessageID).To(Equal(0))
				Expect(msgs[1].MessageID).To(Equal(1))
				Expect(msgs[1].Message).To(Equal("Yes, I can!"))
				Expect(msgs[2].SessionID).To(Equal("other"))

				// jsonl carries the same documents as the compact json array.
				arr, err := run("export", "--db", dbPath, "--session", "", "--format", "json", "--compact")
				Expect(err).NotTo(HaveOccurred())
				var fromArray []parser.AntigravityMessage
				Expect(json.Unmarshal([]byte(arr), &fromArray)).To(Succeed())
				Expect(msgs).To(Equal(fromArray))
			})

			It("ignores --compact for jsonl", func() {
				plain, err := run("export", "--db", dbPath, "--session", "", "--format", "jsonl")
				Expect(err).NotTo(HaveOccurred())
				compact, err := run("export", "--db", dbPath, "--session", "", "--format", "jsonl", "--compact")
				Expect(err).NotTo(HaveOccurred())
				Expect(compact).To(Equal(plain))
			})

			It("writes csv with a header and one row per message, round-tripping commas and quotes", func() {
				tricky := filepath.Join(dir, "tricky.json")
				Expect(os.WriteFile(tricky, []byte(`[
					{"sessionId":"csv","messageId":0,"type":"user","user":"alice","message":"He said \"hello, world\", then\nleft","timestamp":"2025-09-04T21:27:18.909Z"},
					{"sessionId":"csv","messageId":1,"type":"system","message":"plain","timestamp":"2025-09-04T21:28:05.069123456+02:00"}
				]`), 0o644)).To(Succeed())
				trickyDB := filepath.Join(dir, "tricky.db")
				_, err := run("import", "--log", tricky, "--db", trickyDB, "--session", "")
				Expect(err).NotTo(HaveOccurred())

				out, err := run("export", "--db", trickyDB, "--session", "", "--format", "csv")
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(HavePrefix("sessionId,messageId,type,user,message,timestamp,project\n"))

				records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
				Expect(err).NotTo(HaveOccurred())
				Expect(records).To(HaveLen(3), "header plus one row per message")
				Expect(records[0]).To(Equal([]string{"sessionId", "messageId", "type", "user", "message", "timestamp", "project"}))
				Expect(records[1]).To(Equal([]string{
					"csv", "0", "user", "alice", "He said \"hello, world\", then\nleft", "2025-09-04T21:27:18.909Z", "",
				}))
				// The stored offset is preserved rather than normalised to UTC,
				// and the empty user field is an empty column.
				Expect(records[2]).To(Equal([]string{
					"csv", "1", "system", "", "plain", "2025-09-04T21:28:05.069123456+02:00", "",
				}))
			})

			It("writes only the csv header for an empty datastore", func() {
				emptyDB := filepath.Join(dir, "empty.db")
				out, err := run("export", "--db", emptyDB, "--session", "", "--format", "csv")
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(Equal("sessionId,messageId,type,user,message,timestamp,project\n"))
			})

			It("ignores --compact for csv", func() {
				plain, err := run("export", "--db", dbPath, "--session", "", "--format", "csv")
				Expect(err).NotTo(HaveOccurred())
				compact, err := run("export", "--db", dbPath, "--session", "", "--format", "csv", "--compact")
				Expect(err).NotTo(HaveOccurred())
				Expect(compact).To(Equal(plain))
				Expect(strings.Count(plain, "\n")).To(Equal(4), "header plus three rows")
			})

			It("rejects an unknown format before opening or creating the datastore", func() {
				fresh := filepath.Join(dir, "never-created.db")
				_, err := run("export", "--db", fresh, "--session", "", "--format", "xml")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`"xml"`))
				Expect(err.Error()).To(ContainSubstring("json, jsonl, csv"))
				_, statErr := os.Stat(fresh)
				Expect(os.IsNotExist(statErr)).To(BeTrue(), "a bad --format must not create the datastore")
			})

			It("rejects an unknown format even when the datastore exists", func() {
				_, err := run("export", "--db", dbPath, "--session", "", "--format", "JSON")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("json, jsonl, csv"))
			})
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
