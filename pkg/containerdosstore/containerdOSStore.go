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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/metadata"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/plugins/content/local"
	"github.com/containerd/containerd/v2/plugins/snapshots/overlay"
	"github.com/containerd/platforms"
	"github.com/davidcassany/containerdosstore/pkg/logger"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	bolt "go.etcd.io/bbolt"
)

const (
	overlayDriver = "overlayfs"
	snapshotsDir  = "snapshots"
	boltDbFile    = "metadata.db"
	contentDir    = "content"
	namespace     = "elemental-system"

	DefaultRoot         = "/tmp/contentstore"
	LabelSnapshotImgRef = "containerd.io/snapshot/image.ref"

	missInitErrMsg = "uninitiated containerdstore instance"
)

type ContainerdOSStore struct {
	log  logger.Logger
	root string

	// TODO create options to provide those
	driver    string
	namespace string
	platform  platforms.MatchComparer

	ctx context.Context
	db  *metadata.DB
	cli *client.Client
}

func NewContainerdOSStore(log logger.Logger, root string) ContainerdOSStore {
	return ContainerdOSStore{
		root: root, driver: overlayDriver, namespace: namespace,
		log: log, platform: platforms.Default(),
	}
}

func (c ContainerdOSStore) Logger() logger.Logger {
	return c.log
}

func (c *ContainerdOSStore) Init() error {
	var err error
	snapshotters := map[string]snapshots.Snapshotter{}

	ctx := namespaces.WithNamespace(context.TODO(), "testing")

	switch c.driver {
	case overlayDriver:
		//TODO make overlay opts configurable
		sn, err := overlay.NewSnapshotter(filepath.Join(c.root, snapshotsDir))
		if err != nil {
			return err
		}
		snapshotters[overlayDriver] = sn
	default:
		return fmt.Errorf("unsupported containerd driver '%s'", c.driver)
	}

	bdb, err := bolt.Open(filepath.Join(c.root, boltDbFile), 0644, nil)
	if err != nil {
		return err
	}

	store, err := local.NewStore(filepath.Join(c.root, contentDir))
	if err != nil {
		return err
	}

	db := metadata.NewDB(bdb, store, snapshotters)
	err = db.Init(ctx)
	if err != nil {
		return err
	}

	// TODO make client opts configurable
	cli, err := client.NewWithConn(nil, client.WithServices(
		client.WithContentStore(db.ContentStore()),
		client.WithImageStore(metadata.NewImageStore(db)),
		client.WithLeasesService(metadata.NewLeaseManager(db)),
		client.WithDiffService(NewDiffService(db.ContentStore())),
		client.WithSnapshotters(snapshotters),
	), client.WithDefaultPlatform(c.platform))
	if err != nil {
		return err
	}

	c.ctx = ctx
	c.db = db
	c.cli = cli
	return nil
}

func (c *ContainerdOSStore) IsInitiated() bool {
	return c.ctx != nil
}

func (c *ContainerdOSStore) GetClient() *client.Client {
	if !c.IsInitiated() {
		return nil
	}
	return c.cli
}

func (c *ContainerdOSStore) GetDriver() string {
	if !c.IsInitiated() {
		return ""
	}
	return c.driver
}

// Methods copied from nerdctl imgutils package, adding a dependency to nerdctl could be considered

// ReadImageConfig reads the config spec (`application/vnd.oci.image.config.v1+json`) for img.platform from content store.
func ReadImageConfig(ctx context.Context, img client.Image) (ocispec.Image, ocispec.Descriptor, error) {
	var config ocispec.Image

	configDesc, err := img.Config(ctx) // aware of img.platform
	if err != nil {
		return config, configDesc, err
	}
	p, err := content.ReadBlob(ctx, img.ContentStore(), configDesc)
	if err != nil {
		return config, configDesc, err
	}
	if err := json.Unmarshal(p, &config); err != nil {
		return config, configDesc, err
	}
	return config, configDesc, nil
}

// ReadIndex returns image index, or nil for non-indexed image.
func ReadIndex(ctx context.Context, img client.Image) (*ocispec.Index, *ocispec.Descriptor, error) {
	desc := img.Target()
	if !images.IsIndexType(desc.MediaType) {
		return nil, nil, nil
	}
	b, err := content.ReadBlob(ctx, img.ContentStore(), desc)
	if err != nil {
		return nil, &desc, err
	}
	var idx ocispec.Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, &desc, err
	}

	return &idx, &desc, nil
}

// ReadManifest returns the manifest for img.platform, or nil if no manifest was found.
func ReadManifest(ctx context.Context, img client.Image) (*ocispec.Manifest, *ocispec.Descriptor, error) {
	cs := img.ContentStore()
	targetDesc := img.Target()
	if images.IsManifestType(targetDesc.MediaType) {
		b, err := content.ReadBlob(ctx, img.ContentStore(), targetDesc)
		if err != nil {
			return nil, &targetDesc, err
		}
		var mani ocispec.Manifest
		if err := json.Unmarshal(b, &mani); err != nil {
			return nil, &targetDesc, err
		}
		return &mani, &targetDesc, nil
	}
	if images.IsIndexType(targetDesc.MediaType) {
		idx, _, err := ReadIndex(ctx, img)
		if err != nil {
			return nil, nil, err
		}
		configDesc, err := img.Config(ctx) // aware of img.platform
		if err != nil {
			return nil, nil, err
		}
		// We can't access the private `img.platform` variable.
		// So, we find the manifest object by comparing the config desc.
		for _, maniDesc := range idx.Manifests {
			maniDesc := maniDesc
			// ignore non-nil err
			if b, err := content.ReadBlob(ctx, cs, maniDesc); err == nil {
				var mani ocispec.Manifest
				if err := json.Unmarshal(b, &mani); err != nil {
					return nil, nil, err
				}
				if reflect.DeepEqual(configDesc, mani.Config) {
					return &mani, &maniDesc, nil
				}
			}
		}
	}
	// no manifest was found
	return nil, nil, nil
}

// copied from github.com/containerd/containerd/rootfs/apply.go
func uniquePart() string {
	t := time.Now()
	var b [3]byte
	// Ignore read failures, just decreases uniqueness
	rand.Read(b[:])
	return fmt.Sprintf("%d-%s", t.Nanosecond(), base64.URLEncoding.EncodeToString(b[:]))
}
