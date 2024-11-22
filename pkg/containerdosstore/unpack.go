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
	"github.com/containerd/containerd/v2/core/diff"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/core/unpack"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func (c *ContainerdOSStore) Unpack(img client.Image, opts ...client.UnpackOpt) error {
	if !c.IsInitiated() {
		return errors.New(missInitErrMsg)
	}

	//TODO handle lease
	ctx := c.ctx

	return c.unpack(ctx, img, opts...)
}

func (c *ContainerdOSStore) unpack(ctx context.Context, img client.Image, opts ...client.UnpackOpt) error {
	if ok, err := img.IsUnpacked(ctx, c.driver); !ok {
		if err != nil {
			return err
		}

		uPlat := unpack.Platform{
			Platform:       c.platform,
			SnapshotterKey: c.driver,
			Snapshotter:    c.cli.SnapshotService(c.driver),
			SnapshotOpts:   []snapshots.Opt{},
			Applier:        c.cli.DiffService(),
			ApplyOpts:      []diff.ApplyOpt{},
		}

		unpacker, err := unpack.NewUnpacker(ctx, c.cli.ContentStore(), unpack.WithUnpackPlatform(uPlat))
		if err != nil {
			return err
		}

		desc := img.Target()

		var handlerFunc images.HandlerFunc = func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			return images.Children(ctx, c.cli.ContentStore(), desc)
		}
		var handler images.Handler
		handler = images.Handlers(handlerFunc)

		handler = unpacker.Unpack(handler)

		if err := images.WalkNotEmpty(ctx, handler, desc); err != nil {
			if unpacker != nil {
				// wait for unpacker to cleanup
				unpacker.Wait()
			}
			// TODO: Handle Not Empty as a special case on the input
			return err
		}

		if unpacker != nil {
			if _, err = unpacker.Wait(); err != nil {
				return err
			}
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
