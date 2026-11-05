package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

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
	exportCompact bool

	sessionsDB string
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import an Antigravity JSON chat log into the datastore",
	Long: `Reads an Antigravity JSON chat log and stores every message under its
sessionId and messageId. Importing the same log twice is idempotent: an
existing message with the same identity is overwritten, not duplicated.

Use --session to place every imported message in one session, overriding
whatever sessionId the log carries. This is required for logs whose
messages have no sessionId.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		messages, err := parser.ParseChatLog(importLog)
		if err != nil {
			return fmt.Errorf("parse %s: %w", importLog, err)
		}

		store, err := storage.NewBoltStorage(importDB)
		if err != nil {
			return err
		}
		defer store.Close()

		sessions := map[string]int{}
		for i, msg := range messages {
			if importSession != "" {
				msg.SessionID = importSession
			}
			if err := store.PutMessage(msg); err != nil {
				return fmt.Errorf("message %d (session %q, id %d): %w", i, msg.SessionID, msg.MessageID, err)
			}
			sessions[msg.SessionID]++
		}

		fmt.Fprintf(cmd.OutOrStdout(), "imported %d messages into %d session(s) in %s\n", len(messages), len(sessions), importDB)
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export stored messages as an Antigravity-shaped JSON array",
	Long: `Writes messages from the datastore to stdout as a JSON array in the
Antigravity chat-log shape, ordered by session and then message ID. The
output can be fed back to import, tail, or parser.ParseChatLog.

With --session only that session is exported; otherwise every session is.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		enc := json.NewEncoder(cmd.OutOrStdout())
		if !exportCompact {
			enc.SetIndent("", "  ")
		}
		return enc.Encode(all)
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
	exportCmd.Flags().BoolVar(&exportCompact, "compact", false, "Emit single-line JSON instead of indented")

	sessionsCmd.Flags().StringVar(&sessionsDB, "db", "enkente.db", "Path to the BoltDB datastore")
}
