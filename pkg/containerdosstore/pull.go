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
	"reflect"

	"github.com/containerd/containerd/v2/client"
)

type PullOpts struct {
	aOpts  []ApplyCommitOpt
	rOpts  []client.RemoteOpt
	unpack bool
}

type PullOpt func(*PullOpts) error

func WithPullClientOpts(opts ...client.RemoteOpt) PullOpt {
	return func(pOpts *PullOpts) error {
		pOpts.rOpts = append(pOpts.rOpts, opts...)
		return nil
	}
}

func WithPullUnpack() PullOpt {
	return func(pOpts *PullOpts) error {
		pOpts.unpack = true
		return nil
	}
}

func WithPullApplyCommitOpts(opts ...ApplyCommitOpt) PullOpt {
	return func(pOpts *PullOpts) error {
		pOpts.aOpts = append(pOpts.aOpts, opts...)
		return nil
	}
}

func (c *ContainerdOSStore) Pull(ref string, opts ...PullOpt) (_ client.Image, retErr error) {
	if !c.IsInitiated() {
		return nil, errors.New(missInitErrMsg)
	}

	pOpt := &PullOpts{
		aOpts: []ApplyCommitOpt{},
		rOpts: []client.RemoteOpt{},
	}
	for _, o := range opts {
		err := o(pOpt)
		if err != nil {
			return nil, err
		}
	}

	// Ugly hack to filter out WithPullUnpack option
	// Unpack must be always performed manually by us to ensure consistency
	var remove bool
	var i int
	var o client.RemoteOpt
	rOpts := pOpt.rOpts
	for i, o = range rOpts {
		if reflect.ValueOf(o).Pointer() == reflect.ValueOf(client.WithPullUnpack).Pointer() {
			c.log.Warn("Requested 'WithPullUnpack' option, ignoring it")
			remove = true
			break
		}
	}
	if remove {
		rOpts = append(rOpts[:i], rOpts[i+1:]...)
	}

	ctx, done, err := c.cli.WithLease(c.ctx)
	if err != nil {
		c.log.Errorf("failed to create lease to pull image: %v", err)
		return nil, err
	}
	defer func() {
		err = done(ctx)
		if err != nil && retErr == nil {
			c.log.Warnf("could not remove lease on pull operation")
		}
	}()

	img, err := c.cli.Pull(ctx, ref, rOpts...)
	if err != nil {
		c.log.Errorf("failed to pull image '%s': %v", ref, err)
		return nil, err
	}
	c.log.Infof("Successfully pulled image '%s'", img.Name())

	if pOpt.unpack {
		err = c.unpack(ctx, img, pOpt.aOpts...)
		if err != nil {
			c.log.Errorf("failed to unpack image '%s': %v", img.Name(), err)
		} else {
			c.log.Infof("Successfully unpacked image '%s'", img.Name())
		}
	}
	return img, err
}
