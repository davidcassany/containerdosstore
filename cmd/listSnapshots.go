/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// listSnapshotsCmd represents the listSnapshots command
var listSnapshotsCmd = &cobra.Command{
	Use:     "list-snapshots",
	Short:   "Lists all available snapshots",
	PreRunE: initCS,
	RunE: func(cmd *cobra.Command, args []string) error {
		snaps, err := cs.ListSnapshots()
		if err != nil {
			return err
		}

		jsonStr, err := json.MarshalIndent(snaps, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(jsonStr))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listSnapshotsCmd)
}
