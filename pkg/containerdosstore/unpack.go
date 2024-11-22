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

func (c *ContainerdOSStore) Unpack(img client.Image, opts ...ApplyCommitOpt) error {
	if !c.IsInitiated() {
		return errors.New(missInitErrMsg)
	}

	//TODO handle lease
	ctx := c.ctx

	err := c.unpack(ctx, img, opts...)
	if err != nil {
		c.log.Errorf("failed to unpack image '%s': %v", img.Name(), err)
		return err
	}

	c.log.Infof("Successfully unpacked image '%s'", img.Name())
	return nil
}

func (c *ContainerdOSStore) unpack(ctx context.Context, img client.Image, opts ...ApplyCommitOpt) error {
	if ok, err := img.IsUnpacked(ctx, c.driver); !ok {
		if err != nil {
			return err
		}

		cOpt := &ApplyCommitOpts{
			sOpts: []snapshots.Opt{},
			aOpts: []diff.ApplyOpt{},
		}
		for _, o := range opts {
			err := o(cOpt)
			if err != nil {
				return err
			}
		}

		uPlat := unpack.Platform{
			Platform:       c.platform,
			SnapshotterKey: c.driver,
			Snapshotter:    c.cli.SnapshotService(c.driver),
			SnapshotOpts:   cOpt.sOpts,
			Applier:        c.cli.DiffService(),
			ApplyOpts:      cOpt.aOpts,
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
		handler = images.Handlers(images.FilterPlatforms(handlerFunc, c.platform))

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
