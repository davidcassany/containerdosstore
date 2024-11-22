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
	"fmt"

	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/errdefs"
)

func (c *ContainerdOSStore) ListSnapshots(filters ...string) ([]snapshots.Info, error) {
	if !c.IsInitiated() {
		return nil, errors.New(missInitErrMsg)
	}

	//TODO handle lease

	sn := c.cli.SnapshotService(c.driver)

	return listSnapshots(c.ctx, sn, filters...)
}

func (c *ContainerdOSStore) GetSnapshot(key string) (snapshots.Info, error) {
	var info snapshots.Info
	if !c.IsInitiated() {
		return info, errors.New(missInitErrMsg)
	}

	//TODO handle lease

	sn := c.cli.SnapshotService(c.driver)
	return sn.Stat(c.ctx, key)
}

func (c *ContainerdOSStore) UpdateSnapshot(info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	if !c.IsInitiated() {
		return info, errors.New(missInitErrMsg)
	}

	//TODO handle lease

	return c.updateSnapshot(c.ctx, info, fieldpaths...)
}

func (c *ContainerdOSStore) LabelSnapshot(name string, labels map[string]string) error {
	if !c.IsInitiated() {
		return errors.New(missInitErrMsg)
	}

	//TODO handle lease
	ctx := c.ctx

	sn := c.cli.SnapshotService(c.driver)
	info, err := sn.Stat(ctx, name)
	if err != nil {
		return err
	}

	_, err = c.labelSnapshot(ctx, info, labels)
	return err
}

func (c *ContainerdOSStore) RemoveSnapshotLabels(name string, labelKeys ...string) error {
	if !c.IsInitiated() {
		return errors.New(missInitErrMsg)
	}

	//TODO handle lease
	ctx := c.ctx

	sn := c.cli.SnapshotService(c.driver)
	info, err := sn.Stat(ctx, name)
	if err != nil {
		return err
	}

	_, err = c.removeSnapshotLabels(ctx, info, labelKeys...)
	return err
}

func listSnapshots(ctx context.Context, sn snapshots.Snapshotter, filters ...string) ([]snapshots.Info, error) {
	var snaps []snapshots.Info
	walkFunc := func(ctx context.Context, info snapshots.Info) error {
		snaps = append(snaps, info)
		return nil
	}

	err := sn.Walk(ctx, walkFunc, filters...)
	if err != nil {
		return nil, err
	}

	return snaps, nil
}

func removeSnapshotsChain(ctx context.Context, s snapshots.Snapshotter, key string, depth int) error {
	var walkFunc func(ctx context.Context, s snapshots.Snapshotter, key string, step int) error

	walkFunc = func(ctx context.Context, s snapshots.Snapshotter, key string, step int) error {
		sInfo, err := s.Stat(ctx, key)
		if err != nil {
			if errdefs.IsNotFound(err) {
				// TODO add warning logs here
				return nil
			}
			return err
		}
		if err := s.Remove(ctx, key); err != nil {
			// We can't remove snapshots having childs, attempting so returns a failed precondition
			// We only consider it an error if the very first one fails
			if errdefs.IsFailedPrecondition(err) && step != 0 {
				return nil
			}
			return fmt.Errorf("error removing snapshot: %w", err)
		}
		if sInfo.Parent == "" || depth == 0 {
			return nil
		} else if depth > 0 {
			depth--
		}
		return walkFunc(ctx, s, sInfo.Parent, step+1)
	}
	return walkFunc(ctx, s, key, 0)
}

func (c *ContainerdOSStore) updateSnapshot(ctx context.Context, info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	sn := c.cli.SnapshotService(c.driver)
	return sn.Update(ctx, info, fieldpaths...)
}

func (c *ContainerdOSStore) removeSnapshotLabels(ctx context.Context, info snapshots.Info, labelKeys ...string) (snapshots.Info, error) {
	sLabels := info.Labels
	if sLabels == nil {
		sLabels = map[string]string{}
	}
	for _, k := range labelKeys {
		delete(sLabels, k)
	}
	info.Labels = sLabels

	return c.updateSnapshot(ctx, info)
}

func (c *ContainerdOSStore) labelSnapshot(ctx context.Context, info snapshots.Info, labels map[string]string) (snapshots.Info, error) {
	sLabels := info.Labels
	if sLabels == nil {
		sLabels = map[string]string{}
	}
	for k, v := range labels {
		sLabels[k] = v
	}
	info.Labels = sLabels

	return c.updateSnapshot(ctx, info)
}
