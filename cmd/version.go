package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/output"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print tsk version",
	Run: func(cmd *cobra.Command, args []string) {
		output.Print(output.Result{
			Data: map[string]interface{}{
				"version": appVersion,
				"commit":  appCommit,
				"date":    appDate,
			},
			Pretty: func() {
				fmt.Printf("tsk %s%s%s\n", output.Bold, appVersion, output.Reset)
				if appVersion != "dev" {
					fmt.Printf("%scommit: %s%s\n", output.Dim, appCommit, output.Reset)
					fmt.Printf("%sbuilt:  %s%s\n", output.Dim, appDate, output.Reset)
				}
			},
		})
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
