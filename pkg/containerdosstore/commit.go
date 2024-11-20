/*
Copyright The containerd Authors
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/diff"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/rootfs"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// The entire code of this file is porting the code from nerdctl imgutil/commit package

var (
	emptyGZLayer = digest.Digest("sha256:4f4fb700ef54461cfa02571ae0db9a0dc1e0cdb5577484a6d75e68dc38e8acc1")
	emptyDigest  = digest.Digest("")
)

type Changes struct {
	CMD, Entrypoint []string
}

type Opts struct {
	Author  string
	Message string
	Ref     string
	Changes Changes
}

func (c *ContainerdOSStore) Commit(snapshotKey string, opts *Opts) (client.Image, error) {
	if !c.IsInitiated() {
		return nil, errors.New(missInitErrMsg)
	}

	sn := c.cli.SnapshotService(c.driver)
	differ := c.cli.DiffService()
	cs := c.cli.ContentStore()

	info, err := sn.Stat(c.ctx, snapshotKey)
	if err != nil {
		return nil, err
	}

	var baseImgConfig ocispec.Image
	var baseMfst *ocispec.Manifest

	if imgRef, ok := info.Labels["containerd.io/gc.ref.image"]; ok {
		baseImage, err := c.Get(imgRef)
		if err != nil {
			return nil, err
		}

		baseImgConfig, _, err = ReadImageConfig(c.ctx, baseImage)
		if err != nil {
			return nil, err
		}

		baseMfst, _, err = ReadManifest(c.ctx, baseImage)
		if err != nil {
			return nil, err
		}
	}

	// TODO ensure all content for baseImage

	// TODO which is the dirty data to clean?
	// Don't gc me and clean the dirty data after 1 hour!
	ctx, done, err := c.cli.WithLease(c.ctx, leases.WithRandomID(), leases.WithExpiration(1*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("failed to create lease for commit: %w", err)
	}
	defer done(ctx)

	diffLayerDesc, diffID, err := createDiff(ctx, snapshotKey, sn, c.cli.ContentStore(), differ)
	if err != nil {
		return nil, fmt.Errorf("failed to export layer: %w", err)
	}

	imageConfig, err := generateCommitImageConfig(baseImgConfig, diffID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to generate commit image config: %w", err)
	}

	rootfsID := identity.ChainID(imageConfig.RootFS.DiffIDs).String()
	if err := applyDiffLayer(ctx, rootfsID, baseImgConfig, sn, differ, diffLayerDesc); err != nil {
		return nil, fmt.Errorf("failed to apply diff: %w", err)
	}

	// TODO shall we keep the configDigest for something?
	commitManifestDesc, _, err := writeContentsForImage(ctx, cs, c.driver, baseMfst, imageConfig, diffLayerDesc)
	if err != nil {
		return nil, err
	}

	// image create
	img := images.Image{
		Name:      opts.Ref,
		Target:    commitManifestDesc,
		CreatedAt: time.Now(),
	}

	if _, err := c.cli.ImageService().Update(ctx, img); err != nil {
		if !errdefs.IsNotFound(err) {
			return nil, err
		}

		if _, err := c.cli.ImageService().Create(ctx, img); err != nil {
			return nil, fmt.Errorf("failed to create new image %s: %w", opts.Ref, err)
		}
	}

	// unpack the image to snapshotter
	cimg := client.NewImage(c.cli, img)
	if err := cimg.Unpack(ctx, c.driver); err != nil {
		return nil, err
	}

	return cimg, nil
}

// createDiff creates a layer diff into containerd's content store.
func createDiff(ctx context.Context, name string, sn snapshots.Snapshotter, cs content.Store, comparer diff.Comparer) (ocispec.Descriptor, digest.Digest, error) {
	newDesc, err := rootfs.CreateDiff(ctx, name, sn, comparer)
	if err != nil {
		return ocispec.Descriptor{}, digest.Digest(""), err
	}

	info, err := cs.Info(ctx, newDesc.Digest)
	if err != nil {
		return ocispec.Descriptor{}, digest.Digest(""), err
	}

	diffIDStr, ok := info.Labels["containerd.io/uncompressed"]
	if !ok {
		return ocispec.Descriptor{}, digest.Digest(""), fmt.Errorf("invalid differ response with no diffID")
	}

	diffID, err := digest.Parse(diffIDStr)
	if err != nil {
		return ocispec.Descriptor{}, digest.Digest(""), err
	}

	return ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2LayerGzip,
		Digest:    newDesc.Digest,
		Size:      info.Size,
	}, diffID, nil
}

// generateCommitImageConfig returns commit oci image config based on the container's image.
func generateCommitImageConfig(baseConfig ocispec.Image, diffID digest.Digest, opts *Opts) (ocispec.Image, error) {
	// TODO(fuweid): support updating the USER/ENV/... fields?
	if opts.Changes.CMD != nil {
		baseConfig.Config.Cmd = opts.Changes.CMD
	}
	if opts.Changes.Entrypoint != nil {
		baseConfig.Config.Entrypoint = opts.Changes.Entrypoint
	}
	if opts.Author == "" {
		opts.Author = baseConfig.Author
	}

	createdBy := ""
	createdTime := time.Now()
	arch := baseConfig.Architecture
	if arch == "" {
		arch = runtime.GOARCH
		//TODO log warning assuming ARCH
	}
	os := baseConfig.OS
	if os == "" {
		os = runtime.GOOS
		// TODO log warning assuming OS
	}

	return ocispec.Image{
		Platform: ocispec.Platform{
			Architecture: arch,
			OS:           os,
		},

		Created: &createdTime,
		Author:  opts.Author,
		Config:  baseConfig.Config,
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: append(baseConfig.RootFS.DiffIDs, diffID),
		},
		History: append(baseConfig.History, ocispec.History{
			Created:    &createdTime,
			CreatedBy:  createdBy,
			Author:     opts.Author,
			Comment:    opts.Message,
			EmptyLayer: (diffID == emptyGZLayer),
		}),
	}, nil
}

// writeContentsForImage will commit oci image config and manifest into containerd's content store.
func writeContentsForImage(ctx context.Context, cs content.Store, snName string, baseMfst *ocispec.Manifest, newConfig ocispec.Image, diffLayerDesc ocispec.Descriptor) (ocispec.Descriptor, digest.Digest, error) {
	newConfigJSON, err := json.Marshal(newConfig)
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	configDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2Config,
		Digest:    digest.FromBytes(newConfigJSON),
		Size:      int64(len(newConfigJSON)),
	}

	layers := []ocispec.Descriptor{diffLayerDesc}
	if baseMfst != nil {
		layers = append(baseMfst.Layers, layers...)
	}

	newMfst := struct {
		MediaType string `json:"mediaType,omitempty"`
		ocispec.Manifest
	}{
		MediaType: images.MediaTypeDockerSchema2Manifest,
		Manifest: ocispec.Manifest{
			Versioned: specs.Versioned{
				SchemaVersion: 2,
			},
			Config: configDesc,
			Layers: layers,
		},
	}

	newMfstJSON, err := json.MarshalIndent(newMfst, "", "    ")
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	newMfstDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2Manifest,
		Digest:    digest.FromBytes(newMfstJSON),
		Size:      int64(len(newMfstJSON)),
	}

	// new manifest should reference the layers and config content
	labels := map[string]string{
		"containerd.io/gc.ref.content.0": configDesc.Digest.String(),
	}
	for i, l := range layers {
		labels[fmt.Sprintf("containerd.io/gc.ref.content.%d", i+1)] = l.Digest.String()
	}

	err = content.WriteBlob(ctx, cs, newMfstDesc.Digest.String(), bytes.NewReader(newMfstJSON), newMfstDesc, content.WithLabels(labels))
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	// config should reference to snapshotter
	labelOpt := content.WithLabels(map[string]string{
		fmt.Sprintf("containerd.io/gc.ref.snapshot.%s", snName): identity.ChainID(newConfig.RootFS.DiffIDs).String(),
	})
	err = content.WriteBlob(ctx, cs, configDesc.Digest.String(), bytes.NewReader(newConfigJSON), configDesc, labelOpt)
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	return newMfstDesc, configDesc.Digest, nil
}

// applyDiffLayer will apply diff layer content created by createDiff into the snapshotter.
func applyDiffLayer(ctx context.Context, name string, baseImg ocispec.Image, sn snapshots.Snapshotter, differ diff.Applier, diffDesc ocispec.Descriptor) (retErr error) {
	var (
		key    = uniquePart() + "-" + name
		parent = identity.ChainID(baseImg.RootFS.DiffIDs).String()
	)

	mount, err := sn.Prepare(ctx, key, parent)
	if err != nil {
		return err
	}

	defer func() {
		if retErr != nil {
			// NOTE: the snapshotter should be hold by lease. Even
			// if the cleanup fails, the containerd gc can delete it.
			if err := sn.Remove(ctx, key); err != nil {
				// TODO log warn failed to cleanup aborted apply key: err
			}
		}
	}()

	if _, err = differ.Apply(ctx, diffDesc, mount); err != nil {
		return err
	}

	if err = sn.Commit(ctx, name, key); err != nil {
		if errdefs.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}
