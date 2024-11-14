package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/archive"
	"github.com/containerd/containerd/v2/core/metadata"
	"github.com/containerd/containerd/v2/core/snapshots"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/plugins/content/local"
	"github.com/containerd/containerd/v2/plugins/snapshots/overlay"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	bolt "go.etcd.io/bbolt"
)

func main() {
	// Provide a unix address to listen to, this will be the `address`
	// in the `proxy_plugin` configuration.
	// The root will be used to store the snapshots.
	if len(os.Args) < 3 {
		fmt.Printf("invalid args: usage: %s <root> <image file>\n", os.Args[0])
		os.Exit(1)
	}

	sn, err := overlay.NewSnapshotter(filepath.Join(os.Args[1], "snapshotter"))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	snapshotters := map[string]snapshots.Snapshotter{
		"overlay": sn,
	}

	st, err := local.NewStore(filepath.Join(os.Args[1], "content"))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	bdb, err := bolt.Open(filepath.Join(os.Args[1], "metadata.db"), 0644, nil)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	db := metadata.NewDB(bdb, st, snapshotters)
	ist := metadata.NewImageStore(db)
	//TODO is it needed to initiate the db?
}

type importer struct{}

type importOpts struct {
	indexName       string
	imageRefT       func(string) string
	dgstRefT        func(digest.Digest) string
	skipDgstRef     func(string) bool
	allPlatforms    bool
	platformMatcher platforms.MatchComparer
	compress        bool
	discardLayers   bool
	skipMissing     bool
	imageLabels     map[string]string
}

// ImportOpt allows the caller to specify import specific options
type ImportOpt func(*importOpts) error

// WithImageRefTranslator is used to translate the index reference
// to an image reference for the image store.
func WithImageRefTranslator(f func(string) string) ImportOpt {
	return func(c *importOpts) error {
		c.imageRefT = f
		return nil
	}
}

// WithImageLabels are the image labels to apply to a new image
func WithImageLabels(labels map[string]string) ImportOpt {
	return func(c *importOpts) error {
		c.imageLabels = labels
		return nil
	}
}

// WithDigestRef is used to create digest images for each
// manifest in the index.
func WithDigestRef(f func(digest.Digest) string) ImportOpt {
	return func(c *importOpts) error {
		c.dgstRefT = f
		return nil
	}
}

// WithSkipDigestRef is used to specify when to skip applying
// WithDigestRef. The callback receives an image reference (or an empty
// string if not specified in the image). When the callback returns true,
// the skip occurs.
func WithSkipDigestRef(f func(string) bool) ImportOpt {
	return func(c *importOpts) error {
		c.skipDgstRef = f
		return nil
	}
}

// WithIndexName creates a tag pointing to the imported index
func WithIndexName(name string) ImportOpt {
	return func(c *importOpts) error {
		c.indexName = name
		return nil
	}
}

// WithAllPlatforms is used to import content for all platforms.
func WithAllPlatforms(allPlatforms bool) ImportOpt {
	return func(c *importOpts) error {
		c.allPlatforms = allPlatforms
		return nil
	}
}

// WithImportPlatform is used to import content for specific platform.
func WithImportPlatform(platformMacher platforms.MatchComparer) ImportOpt {
	return func(c *importOpts) error {
		c.platformMatcher = platformMacher
		return nil
	}
}

// WithImportCompression compresses uncompressed layers on import.
// This is used for import formats which do not include the manifest.
func WithImportCompression() ImportOpt {
	return func(c *importOpts) error {
		c.compress = true
		return nil
	}
}

// WithDiscardUnpackedLayers allows the garbage collector to clean up
// layers from content store after unpacking.
func WithDiscardUnpackedLayers() ImportOpt {
	return func(c *importOpts) error {
		c.discardLayers = true
		return nil
	}
}

// WithSkipMissing allows to import an archive which doesn't contain all the
// referenced blobs.
func WithSkipMissing() ImportOpt {
	return func(c *importOpts) error {
		c.skipMissing = true
		return nil
	}
}

// Import imports an image from a Tar stream using reader.
// Caller needs to specify importer. Future version may use oci.v1 as the default.
// Note that unreferenced blobs may be imported to the content store as well.
func Import(ctx context.Context, reader io.Reader, st content.Store, ist images.Store, opts ...ImportOpt) ([]images.Image, error) {
	var iopts importOpts
	for _, o := range opts {
		if err := o(&iopts); err != nil {
			return nil, err
		}
	}

	// TODO shall we initiate a containerd client?
	/*ctx, done, err := c.WithLease(ctx)
	if err != nil {
		return nil, err
	}
	defer done(ctx)*/

	var aio []archive.ImportOpt
	if iopts.compress {
		aio = append(aio, archive.WithImportCompression())
	}

	index, err := archive.ImportIndex(ctx, st, reader, aio...)
	if err != nil {
		return nil, err
	}

	var (
		imgs []images.Image
		cs   = st
		is   = ist
	)

	if iopts.indexName != "" {
		imgs = append(imgs, images.Image{
			Name:   iopts.indexName,
			Target: index,
		})
	}
	var platformMatcher = c.platform
	if iopts.allPlatforms {
		platformMatcher = platforms.All
	} else if iopts.platformMatcher != nil {
		platformMatcher = iopts.platformMatcher
	}

	var handler images.HandlerFunc = func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		// Only save images at top level
		if desc.Digest != index.Digest {
			// Don't set labels on missing content.
			children, err := images.Children(ctx, cs, desc)
			if iopts.skipMissing && errdefs.IsNotFound(err) {
				return nil, images.ErrSkipDesc
			}
			return children, err
		}

		idx, err := decodeIndex(ctx, cs, desc)
		if err != nil {
			return nil, err
		}

		for _, m := range idx.Manifests {
			name := imageName(m.Annotations, iopts.imageRefT)
			if name != "" {
				imgs = append(imgs, images.Image{
					Name:   name,
					Target: m,
				})
			}
			if iopts.skipDgstRef != nil {
				if iopts.skipDgstRef(name) {
					continue
				}
			}
			if iopts.dgstRefT != nil {
				ref := iopts.dgstRefT(m.Digest)
				if ref != "" {
					imgs = append(imgs, images.Image{
						Name:   ref,
						Target: m,
					})
				}
			}
		}

		return idx.Manifests, nil
	}

	handler = images.FilterPlatforms(handler, platformMatcher)
	if iopts.discardLayers {
		handler = images.SetChildrenMappedLabels(cs, handler, images.ChildGCLabelsFilterLayers)
	} else {
		handler = images.SetChildrenLabels(cs, handler)
	}
	if err := images.WalkNotEmpty(ctx, handler, index); err != nil {
		return nil, err
	}

	for i := range imgs {
		fieldsPath := []string{"target"}
		if iopts.imageLabels != nil {
			fieldsPath = append(fieldsPath, "labels")
			imgs[i].Labels = iopts.imageLabels
		}
		img, err := is.Update(ctx, imgs[i], fieldsPath...)
		if err != nil {
			if !errdefs.IsNotFound(err) {
				return nil, err
			}

			img, err = is.Create(ctx, imgs[i])
			if err != nil {
				return nil, err
			}
		}
		imgs[i] = img
	}

	return imgs, nil
}

func imageName(annotations map[string]string, ociCleanup func(string) string) string {
	name := annotations[images.AnnotationImageName]
	if name != "" {
		return name
	}
	name = annotations[ocispec.AnnotationRefName]
	if name != "" {
		if ociCleanup != nil {
			name = ociCleanup(name)
		}
	}
	return name
}

func decodeIndex(ctx context.Context, store content.Provider, desc ocispec.Descriptor) (*ocispec.Index, error) {
	var index ocispec.Index
	p, err := content.ReadBlob(ctx, store, desc)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(p, &index); err != nil {
		return nil, err
	}

	return &index, nil
}
