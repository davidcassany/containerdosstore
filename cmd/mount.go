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
	"github.com/containerd/containerd/v2/client"
	"github.com/spf13/cobra"
)

// mountCmd represents the mount command
var mountCmd = &cobra.Command{
	Use:     "mount MOUNTPOINT",
	Short:   "Mounts the given image name to the given target mountpoint",
	Args:    cobra.ExactArgs(1),
	PreRunE: initCS,
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()
		readOnly, _ := flags.GetBool("read-only")
		unpack, _ := flags.GetBool("unpack")
		key, _ := flags.GetString("snapshot-key")
		name, _ := flags.GetString("image")
		scratch, _ := flags.GetBool("from-scratch")
		target := args[0]

		// TODO create/check target?
		var img client.Image
		var err error
		log := cs.Logger()

		if !scratch {
			img, err = cs.Get(name)
			if err != nil {
				return err
			}

			if unpack {
				err = cs.Unpack(img)
				if err != nil {
					return err
				}
			}
		}

		key, err = cs.Mount(img, target, key, readOnly)
		if err != nil {
			return err
		}
		log.Infof("Snapshot '%s' mounted at '%s'", key, target)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(mountCmd)

	mountCmd.Flags().String("snapshot-key", "", "Sets a custom key for the active snapshot")
	mountCmd.Flags().Bool("unpack", false, "Unpacks the image if not done already")
	mountCmd.Flags().Bool("read-only", false, "Set the mount as a read-only mount")
	mountCmd.Flags().Bool("from-scratch", false, "Sets the mount of a new snapshot without a base image, used to create images from scratch")
	mountCmd.Flags().String("image", "", "Name of the image to mount")

	mountCmd.MarkFlagsMutuallyExclusive("from-scratch", "image")
	mountCmd.MarkFlagsMutuallyExclusive("from-scratch", "unpack")
	mountCmd.MarkFlagsMutuallyExclusive("from-scratch", "read-only")
	mountCmd.MarkFlagsOneRequired("from-scratch", "image")
}
