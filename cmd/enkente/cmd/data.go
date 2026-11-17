package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

// Headless datastore commands. Unlike serve and tail these need no terminal,
// so they are usable from scripts and CI.

var (
	importLog     string
	importDB      string
	importSession string

	exportDB      string
	exportSession string
	exportFormat  string
	exportCompact bool

	sessionsDB string
)

// exportFormats lists the values accepted by export --format, in the order
// they are named in the usage error.
var exportFormats = []string{"json", "jsonl", "csv"}

// exportCSVHeader is the first row of a csv export. Its column names match
// the JSON field names of parser.AntigravityMessage.
var exportCSVHeader = []string{"sessionId", "messageId", "type", "user", "message", "timestamp", "project"}

// writeExport writes msgs to w in the given format. compact only affects
// json; jsonl is always one compact object per line and csv has no
// alternative layout.
func writeExport(w io.Writer, format string, msgs []parser.AntigravityMessage, compact bool) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		if !compact {
			enc.SetIndent("", "  ")
		}
		return enc.Encode(msgs)
	case "jsonl":
		enc := json.NewEncoder(w)
		for _, msg := range msgs {
			if err := enc.Encode(msg); err != nil {
				return err
			}
		}
		return nil
	case "csv":
		cw := csv.NewWriter(w)
		if err := cw.Write(exportCSVHeader); err != nil {
			return err
		}
		for _, msg := range msgs {
			row := []string{
				msg.SessionID,
				strconv.Itoa(msg.MessageID),
				msg.Type,
				msg.User,
				msg.Message,
				msg.Timestamp.Format(time.RFC3339Nano),
				msg.Project,
			}
			if err := cw.Write(row); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	default:
		return fmt.Errorf("unknown export format %q: must be one of %s", format, strings.Join(exportFormats, ", "))
	}
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import an Antigravity JSON chat log into the datastore",
	Long: `Reads an Antigravity JSON chat log and stores every message under its
sessionId and messageId. Importing the same log twice is idempotent: an
existing message with the same identity is overwritten, not duplicated.

Use --session to place every imported message in one session, overriding
whatever sessionId the log carries. This is required for logs whose
messages have no sessionId. The import is refused if that would make two
messages share a messageId, so logs from several sessions cannot be
collapsed into one.

The import is all-or-nothing: the log is validated before the datastore
is opened, and all messages are written in a single transaction.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		messages, err := parser.ParseChatLog(importLog)
		if err != nil {
			return fmt.Errorf("parse %s: %w", importLog, err)
		}
		if len(messages) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "no messages in %s; nothing imported\n", importLog)
			return nil
		}

		type identity struct {
			session string
			id      int
		}
		seen := map[identity]int{}
		sessions := map[string]int{}
		for i := range messages {
			if importSession != "" {
				messages[i].SessionID = importSession
			}
			if err := storage.ValidateMessage(messages[i]); err != nil {
				return fmt.Errorf("message %d in %s: %w (use --session to assign one)", i, importLog, err)
			}
			key := identity{messages[i].SessionID, messages[i].MessageID}
			if prev, dup := seen[key]; dup {
				return fmt.Errorf("messages %d and %d in %s would both be stored as session %q messageId %d; refusing to import", prev, i, importLog, key.session, key.id)
			}
			seen[key] = i
			sessions[key.session]++
		}

		store, err := storage.NewBoltStorage(importDB)
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.PutMessages(messages); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "imported %d messages into %d session(s) in %s\n", len(messages), len(sessions), importDB)
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export stored messages as JSON, JSON Lines or CSV",
	Long: `Writes messages from the datastore to stdout, ordered by session and then
message ID, in one of three formats chosen with --format:

  json   (default) a JSON array in the Antigravity chat-log shape. The
         output can be fed back to import, tail, or parser.ParseChatLog.
         --compact emits it on a single line instead of indented.
  jsonl  one compact JSON object per line, with the same field names as
         json, for streaming tools and line-oriented NoSQL loaders.
  csv    a header row (sessionId,messageId,type,user,message,timestamp,project)
         followed by one row per message, quoted as encoding/csv requires;
         timestamps are RFC 3339 with nanoseconds, as stored.

--compact only affects json. With --session only that session is exported;
otherwise every session is.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Reject a bad format before touching the datastore so a typo never
		// creates an empty database file.
		if !slices.Contains(exportFormats, exportFormat) {
			return fmt.Errorf("unknown export format %q: must be one of %s", exportFormat, strings.Join(exportFormats, ", "))
		}

		store, err := storage.NewBoltStorage(exportDB)
		if err != nil {
			return err
		}
		defer store.Close()

		var sessionIDs []string
		if exportSession != "" {
			sessionIDs = []string{exportSession}
		} else {
			infos, err := store.ListSessions()
			if err != nil {
				return err
			}
			for _, info := range infos {
				sessionIDs = append(sessionIDs, info.SessionID)
			}
		}

		all := []parser.AntigravityMessage{}
		for _, id := range sessionIDs {
			msgs, err := store.ListMessages(id)
			if err != nil {
				return fmt.Errorf("session %q: %w", id, err)
			}
			all = append(all, msgs...)
		}

		return writeExport(cmd.OutOrStdout(), exportFormat, all, exportCompact)
	},
}

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List the sessions held in the datastore with their message counts",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := storage.NewBoltStorage(sessionsDB)
		if err != nil {
			return err
		}
		defer store.Close()

		infos, err := store.ListSessions()
		if err != nil {
			return err
		}

		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "SESSION\tMESSAGES")
		for _, info := range infos {
			fmt.Fprintf(tw, "%s\t%d\n", info.SessionID, info.MessageCount)
		}
		return tw.Flush()
	},
}

func init() {
	rootCmd.AddCommand(importCmd, exportCmd, sessionsCmd)

	importCmd.Flags().StringVarP(&importLog, "log", "l", "", "Path to the Antigravity JSON chat log to import")
	importCmd.Flags().StringVar(&importDB, "db", "enkente.db", "Path to the BoltDB datastore")
	importCmd.Flags().StringVar(&importSession, "session", "", "Store every imported message under this sessionId")
	_ = importCmd.MarkFlagRequired("log")

	exportCmd.Flags().StringVar(&exportDB, "db", "enkente.db", "Path to the BoltDB datastore")
	exportCmd.Flags().StringVar(&exportSession, "session", "", "Export only this sessionId")
	exportCmd.Flags().StringVar(&exportFormat, "format", "json", "Output format: json (array), jsonl (one object per line) or csv")
	exportCmd.Flags().BoolVar(&exportCompact, "compact", false, "Emit single-line JSON instead of indented (json format only; ignored for jsonl and csv)")

	sessionsCmd.Flags().StringVar(&sessionsDB, "db", "enkente.db", "Path to the BoltDB datastore")
}
