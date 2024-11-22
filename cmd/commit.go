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
	"github.com/davidcassany/containerdosstore/pkg/containerdosstore"
	"github.com/spf13/cobra"
)

// commitCmd represents the commit command
var commitCmd = &cobra.Command{
	Use:     "commit SNAPSHOT_KEY IMAGE_NAME",
	Short:   "Commit given active snapshot as a new image",
	Args:    cobra.ExactArgs(1),
	PreRunE: initCS,
	RunE: func(cmd *cobra.Command, args []string) error {
		snapshotkey := args[0]
		newRef := args[1]
		log := cs.Logger()

		iOpts := containerdosstore.ImgOpts{Ref: newRef}

		newImg, err := cs.Commit(snapshotkey, containerdosstore.WithImgOpts(iOpts))
		if err != nil {
			return err
		}

		log.Infof("New image '%s' created\n", newImg.Name())

		return nil
	},
}

func init() {
	rootCmd.AddCommand(commitCmd)

	commitCmd.Flags().String("image", "", "Name of the new image to commit")
	commitCmd.MarkFlagRequired("image")
}
