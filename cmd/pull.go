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

// pullCmd represents the pull command
var pullCmd = &cobra.Command{
	Use:     "pull IMAGE_REF",
	Short:   "pulls a remote image into containerd store",
	Args:    cobra.ExactArgs(1),
	PreRunE: initCS,
	RunE: func(cmd *cobra.Command, args []string) error {
		log := cs.Logger()
		flags := cmd.Flags()
		unpack, _ := flags.GetBool("unpack")
		img, err := cs.Pull(args[0])
		if err == nil {
			log.Infof("Pulled image '%s' with digest '%s'", img.Name(), img.Metadata().Target.Digest)
		}
		if unpack {
			err = cs.Unpack(img)
			if err != nil {
				return err
			}
			log.Infof("Unpacked image '%s'", img.Name())
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pullCmd)

	pullCmd.Flags().Bool("unpack", false, "Unpacks the pulled image")
}
