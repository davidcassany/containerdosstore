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
	"os"

	"github.com/davidcassany/containerdosstore/pkg/containerdosstore"
	"github.com/davidcassany/containerdosstore/pkg/logger"
	"github.com/spf13/cobra"
)

var cs containerdosstore.ContainerdOSStore

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "containerdosstore",
	Short: "A daememon less client for a local image containerd store",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func initCS(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	root, _ := flags.GetString("root")
	llvl, _ := flags.GetString("loglevel")
	debug, _ := flags.GetBool("debug")

	var log logger.Logger
	var err error

	if debug {
		log, _ = logger.NewLogger(logger.DebugLevel)
	} else if llvl != "" {
		log, err = logger.NewLogger(llvl)
		if err != nil {
			log, _ = logger.NewLogger(logger.DebugLevel)
		}
		log.Warnf("could parse log level '%s', setting log to '%s' level", llvl, logger.DebugLevel)
	} else {
		log, _ = logger.NewLogger(logger.InfoLevel)
	}

	cs = containerdosstore.NewContainerdOSStore(log, root)
	return cs.Init()
}

func init() {
	rootCmd.PersistentFlags().String("root", containerdosstore.DefaultRoot, "path for the containerd local store")
	rootCmd.PersistentFlags().Bool("debug", false, "set log ouput to debug level")
	rootCmd.PersistentFlags().String("loglevel", "", "set log ouput level")
	rootCmd.MarkFlagsMutuallyExclusive("debug", "loglevel")

	cobra.OnFinalize(
		func() {
			if cs.IsInitiated() {
				err := cs.GetClient().SnapshotService(cs.GetDriver()).Close()
				if err != nil {
					cs.Logger().Debugf("failed closing snapshotter: %v", err)
				}
			}
		}, func() {
			//TODO handle garbage collector
		},
	)
}
