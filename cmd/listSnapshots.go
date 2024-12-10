/*
Copyright © 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
