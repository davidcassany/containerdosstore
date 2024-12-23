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
	"github.com/davidcassany/ocistore/pkg/ocistore"
	"github.com/spf13/cobra"
)

// importCmd represents the import command
var importCmd = &cobra.Command{
	Use:     "import FILE",
	Short:   "Imports the given OCI archive",
	Long:    `Let's rock`,
	Args:    cobra.ExactArgs(1),
	PreRunE: initCS,
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()
		unpack, _ := flags.GetBool("unpack")
		file := args[0]

		opts := []ocistore.ImportOpt{}
		if unpack {
			opts = append(opts, ocistore.WithImportUnpack())
		}

		_, err := cs.ImportFile(file, opts...)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(importCmd)

	importCmd.Flags().Bool("unpack", false, "Unpacks imported images")
}
