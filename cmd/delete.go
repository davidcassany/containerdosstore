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
	"github.com/spf13/cobra"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:     "delete IMAGE_NAME",
	Short:   "Deletes the given image",
	Long:    `Be sensitive`,
	Args:    cobra.ExactArgs(1),
	PreRunE: initCS,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		log := cs.Logger()

		img, err := cs.Get(name)
		if err != nil {
			return err
		}
		err = cs.Delete(name)
		if err != nil {
			return err
		}
		log.Infof("Image '%s' with digest '%s' deleted\n", img.Name(), img.Metadata().Target.Digest)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
