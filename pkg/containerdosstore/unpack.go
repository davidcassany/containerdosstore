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

package containerdosstore

import (
	"context"
	"errors"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/snapshots"
)

func (c *ContainerdOSStore) Unpack(img client.Image, opts ...client.UnpackOpt) error {
	if !c.IsInitiated() {
		return errors.New(missInitErrMsg)
	}

	if ok, err := img.IsUnpacked(c.ctx, c.driver); !ok {
		if err != nil {
			return err
		}

		err = img.Unpack(c.ctx, c.driver, opts...)
		if err != nil {
			return err
		}
	}
	return nil
}

// WithUnpackSnapshotOpts appends new snapshot options on the UnpackConfig.
func WithUnpackSnapshotOpts(opts ...snapshots.Opt) client.UnpackOpt {
	return func(ctx context.Context, uc *client.UnpackConfig) error {
		uc.SnapshotOpts = append(uc.SnapshotOpts, opts...)
		return nil
	}
}
