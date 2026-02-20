package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "enkente",
	Short: "enkente is a multi-faceted mind-mapping datastore",
	Long: `enkente is a real-time collaborative platform designed to ingest 
multi-user chat streams and continuously encode semantic and contextual relationships.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Usage()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
