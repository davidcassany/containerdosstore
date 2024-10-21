package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/diff"
	"github.com/containerd/containerd/v2/core/diff/apply"
	"github.com/containerd/containerd/v2/core/metadata"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/plugins/content/local"
	"github.com/containerd/containerd/v2/plugins/diff/walking"
	"github.com/containerd/containerd/v2/plugins/snapshots/overlay"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"
	"github.com/opencontainers/image-spec/identity"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	bolt "go.etcd.io/bbolt"
)

const snapshotterName = "overlay"

// Define a local (grpcless) implementation of the diffservice
type diffService struct {
	walkDiff  diff.Comparer
	applyDiff diff.Applier
}

func (d *diffService) Compare(ctx context.Context, lower, upper []mount.Mount, opts ...diff.Opt) (ocispec.Descriptor, error) {
	return d.walkDiff.Compare(ctx, lower, upper, opts...)
}

func (d *diffService) Apply(ctx context.Context, desc ocispec.Descriptor, mount []mount.Mount, opts ...diff.ApplyOpt) (ocispec.Descriptor, error) {
	return d.applyDiff.Apply(ctx, desc, mount, opts...)
}

func newDiffService(store content.Store) client.DiffService {
	return &diffService{
		walkDiff:  walking.NewWalkingDiff(store),
		applyDiff: apply.NewFileSystemApplier(store),
	}
}

func main() {
	// Provide a unix address to listen to, this will be the `address`
	// in the `proxy_plugin` configuration.
	// The root will be used to store the snapshots.
	if len(os.Args) < 4 {
		fmt.Printf("invalid args: usage: %s <root> <image file> <mount target>\n", os.Args[0])
		os.Exit(1)
	}

	ctx := namespaces.WithNamespace(context.TODO(), "testing")
	in := os.Args[2]
	target := os.Args[3]

	var err error
	var r io.ReadCloser
	if in == "-" {
		r = os.Stdin
	} else {
		r, err = os.Open(in)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			os.Exit(1)
		}
	}

	sn, err := overlay.NewSnapshotter(filepath.Join(os.Args[1], "snapshotter"))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	snapshotters := map[string]snapshots.Snapshotter{
		snapshotterName: sn,
	}

	bdb, err := bolt.Open(filepath.Join(os.Args[1], "metadata.db"), 0644, nil)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	store, err := local.NewStore(filepath.Join(os.Args[1], "content"))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	db := metadata.NewDB(bdb, store, snapshotters)
	err = db.Init(ctx)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	ist := metadata.NewImageStore(db)

	// TODO figure out how to make this connection an explicit mock.
	/*grpcCli, err := grpc.NewClient("unix:///tmp/dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}*/

	leaseMngr := metadata.NewLeaseManager(db)

	diffService := newDiffService(db.ContentStore())

	cli, err := client.NewWithConn(nil, client.WithServices(
		client.WithContentStore(db.ContentStore()),
		client.WithImageStore(ist),
		client.WithLeasesService(leaseMngr),
		client.WithDiffService(diffService),
		client.WithSnapshotters(snapshotters),
	))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	// Using the db's content store we get 'store' with the added labelstore
	imgs, err := cli.Import(ctx, r)
	nErr := r.Close()
	if nErr != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	for _, img := range imgs {
		fmt.Printf("Imported '%s' image\n", img.Name)

		nimg := client.NewImageWithPlatform(cli, img, platforms.Default())
		if ok, err := nimg.IsUnpacked(ctx, snapshotterName); !ok {
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			err = nimg.Unpack(ctx, snapshotterName)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("image '%s' unpacked\n", img.Name)
		} else {
			fmt.Printf("image '%s' is already unpacked\n", img.Name)
		}
		diffIDs, err := nimg.RootFS(ctx)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			os.Exit(1)
		}
		chainID := identity.ChainID(diffIDs).String()
		fmt.Println(chainID)

		var mounts []mount.Mount
		mounts, err = sn.Prepare(ctx, target, chainID)

		if err != nil {
			if errdefs.IsAlreadyExists(err) {
				mounts, err = sn.Mounts(ctx, target)
			}
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
		}

		fmt.Println(mounts)

		if err := mount.All(mounts, target); err != nil {
			if err := sn.Remove(ctx, target); err != nil && !errdefs.IsNotFound(err) {
				fmt.Printf("error cleaning up snapshot after mount error: %v", err)
				os.Exit(1)

			}
			fmt.Printf("error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Image '%s' mounted at '%s'\n", nimg.Name(), target)
	}
}
