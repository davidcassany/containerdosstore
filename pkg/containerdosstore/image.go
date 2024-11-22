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
	"github.com/containerd/containerd/v2/core/images"
	"github.com/opencontainers/image-spec/identity"
)

func (c *ContainerdOSStore) Get(ref string) (client.Image, error) {
	if !c.IsInitiated() {
		return nil, errors.New(missInitErrMsg)
	}

	img, err := c.cli.GetImage(c.ctx, ref)
	if err != nil {
		return nil, err
	}

	return img, nil
}

func (c *ContainerdOSStore) List(filters ...string) ([]client.Image, error) {
	if !c.IsInitiated() {
		return nil, errors.New(missInitErrMsg)
	}

	images, err := c.cli.ListImages(c.ctx, filters...)
	if err != nil {
		return nil, err
	}

	return images, nil
}

func (c *ContainerdOSStore) Delete(name string, opts ...images.DeleteOpt) error {
	if !c.IsInitiated() {
		return errors.New(missInitErrMsg)
	}

	//TODO handle lease
	ctx := c.ctx

	err := c.delete(ctx, name, opts...)
	if err != nil {
		c.log.Errorf("failed deleting image '%s': %v", name, err)
		return err
	}

	c.log.Infof("Successfully deleted image '%s'", name)
	return nil
}

func (c *ContainerdOSStore) Update(img images.Image, fieldpaths ...string) (client.Image, error) {
	if !c.IsInitiated() {
		return nil, errors.New(missInitErrMsg)
	}

	//TODO handle lease

	i, err := c.cli.ImageService().Update(c.ctx, img, fieldpaths...)
	if err != nil {
		return nil, err
	}

	return client.NewImage(c.cli, i), nil
}

func (c *ContainerdOSStore) Create(img images.Image) (client.Image, error) {
	if !c.IsInitiated() {
		return nil, errors.New(missInitErrMsg)
	}

	//TODO handle lease

	i, err := c.cli.ImageService().Create(c.ctx, img)
	if err != nil {
		return nil, err
	}

	return client.NewImage(c.cli, i), nil
}

func (c *ContainerdOSStore) delete(ctx context.Context, name string, opts ...images.DeleteOpt) error {
	img, err := c.cli.GetImage(ctx, name)
	if err != nil {
		return err
	}
	if ok, err := img.IsUnpacked(ctx, c.driver); ok {
		diffIDs, err := img.RootFS(ctx)
		if err != nil {
			return err
		}
		chainID := identity.ChainID(diffIDs).String()
		sn := c.cli.SnapshotService(c.driver)
		err = removeSnapshotsChain(ctx, sn, chainID, -1)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return c.cli.ImageService().Delete(ctx, name, opts...)
}
