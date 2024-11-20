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
	"errors"
	"fmt"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/image-spec/identity"
)

func (c *ContainerdOSStore) MountFromScratch(target string, key string) (snapshotKey string, retErr error) {
	return c.Mount(nil, target, key, false)
}

func (c *ContainerdOSStore) Mount(img client.Image, target string, key string, readonly bool) (snapshotKey string, retErr error) {
	if !c.IsInitiated() {
		return "", errors.New(missInitErrMsg)
	}

	if key == "" {
		key = target
	}

	//TODO handle lease properly, whats the purpose of it?
	ctx, done, err := c.cli.WithLease(c.ctx,
		leases.WithID(key),
		leases.WithExpiration(24*time.Hour),
		leases.WithLabel("containerd.io/gc.ref.snapshot."+c.driver, key),
	)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return "", err
	}

	defer func() {
		if retErr != nil && done != nil {
			done(ctx)
		}
	}()

	// TODO create and/or check target existence?

	var parent string
	var labels map[string]string

	// TODO properly name labels
	if img == nil {
		parent = ""
		labels = map[string]string{}
	} else {
		diffIDs, err := img.RootFS(ctx)
		if err != nil {
			return "", err
		}
		parent = identity.ChainID(diffIDs).String()
		labels = map[string]string{
			"containerd.io/gc.ref.image": img.Name(),
		}
	}

	sn := c.cli.SnapshotService(c.driver)
	opts := []snapshots.Opt{
		snapshots.WithLabels(labels),
	}

	var mounts []mount.Mount
	if readonly {
		mounts, err = sn.View(ctx, key, parent, opts...)
	} else {
		mounts, err = sn.Prepare(ctx, key, parent, opts...)
	}

	if err != nil {
		if errdefs.IsAlreadyExists(err) {
			mounts, err = sn.Mounts(ctx, key)
		}
		if err != nil {
			return "", err
		}
	}

	if err := mount.All(mounts, target); err != nil {
		if err := sn.Remove(ctx, key); err != nil && !errdefs.IsNotFound(err) {
			return "", fmt.Errorf("error cleaning up snapshot after mount error: %v", err)
		}
		return "", err
	}

	return key, nil
}

func (c *ContainerdOSStore) Umount(target string, key string, removeSnap int) error {
	if !c.IsInitiated() {
		return errors.New(missInitErrMsg)
	}

	if err := mount.UnmountAll(target, 0); err != nil {
		return err
	}

	// Do not remove any snapshot
	if removeSnap == 0 {
		return nil
	}

	if err := c.cli.LeasesService().Delete(c.ctx, leases.Lease{ID: key}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("error deleting lease: %w", err)
	}
	s := c.cli.SnapshotService(c.driver)

	// Remove up to a certain level of childs
	if removeSnap > 0 {
		removeSnapshotsChain(c.ctx, s, key, removeSnap-1)
	}

	// Remove the entire chain
	return removeSnapshotsChain(c.ctx, s, key, -1)
}
