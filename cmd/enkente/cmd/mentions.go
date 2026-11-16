package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

var (
	mentionsDB    string
	mentionsKind  string
	mentionsLimit int
)

// splitMentionArg turns "#graphdb" or "@alice" into a kind and a name.
func splitMentionArg(arg string) (parser.MentionKind, string, error) {
	if len(arg) > 1 {
		if kind, ok := parser.ParseMentionKind(arg[:1]); ok {
			return kind, arg[1:], nil
		}
	}
	return "", "", fmt.Errorf("%q is not a mention: write #tag or @user (quote it so the shell keeps the #)", arg)
}

var mentionsCmd = &cobra.Command{
	Use:   "mentions [#tag|@user]",
	Short: "Summarize #tag and @user mentions, or print the messages that mention one",
	Long: `With no argument, list every #tag and @user referenced in stored messages,
with how many messages and sessions mention it and who introduced it.

With one argument such as '#graphdb' or '@alice', print the messages that
mention it, oldest first, as a JSON chat-log array.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var filter parser.MentionKind
		if mentionsKind != "" {
			k, ok := parser.ParseMentionKind(mentionsKind)
			if !ok {
				return fmt.Errorf("--kind must be tag or user, got %q", mentionsKind)
			}
			filter = k
		}
		var kind parser.MentionKind
		var name string
		if len(args) == 1 {
			var err error
			if kind, name, err = splitMentionArg(args[0]); err != nil {
				return err
			}
		}
		if mentionsLimit < 0 {
			return fmt.Errorf("--limit must be non-negative")
		}

		store, err := storage.NewBoltStorage(mentionsDB)
		if err != nil {
			return err
		}
		defer store.Close()

		if name != "" {
			msgs, err := store.MessagesMentioning(kind, name, mentionsLimit)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(msgs)
		}

		summaries, err := store.ListMentions(filter)
		if err != nil {
			return err
		}
		if mentionsLimit > 0 && len(summaries) > mentionsLimit {
			summaries = summaries[:mentionsLimit]
		}
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "MENTION\tMESSAGES\tSESSIONS\tFIRST BY\tFIRST AT")
		for _, m := range summaries {
			sigil := "#"
			if m.Kind == parser.MentionUser {
				sigil = "@"
			}
			by := strings.TrimSpace(m.FirstSeen.User)
			if by == "" {
				by = "-"
			}
			fmt.Fprintf(tw, "%s%s\t%d\t%d\t%s\t%s\n", sigil, m.Name, m.Messages, m.Sessions, by,
				m.FirstSeen.Timestamp.UTC().Format("2006-01-02T15:04:05Z"))
		}
		return tw.Flush()
	},
}

func init() {
	rootCmd.AddCommand(mentionsCmd)
	mentionsCmd.Flags().StringVar(&mentionsDB, "db", "enkente.db", "Path to the BoltDB datastore")
	mentionsCmd.Flags().StringVar(&mentionsKind, "kind", "", "List only tag or user mentions")
	mentionsCmd.Flags().IntVar(&mentionsLimit, "limit", 0, "Show at most this many rows or messages (0 = all)")
}
