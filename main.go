package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"google.golang.org/grpc"

	contentapi "github.com/containerd/containerd/api/services/content/v1"
	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/v2/contrib/snapshotservice"
	"github.com/containerd/containerd/v2/plugins/content/local"
	"github.com/containerd/containerd/v2/plugins/services/content/contentserver"
	"github.com/containerd/containerd/v2/plugins/snapshots/overlay"
)

func main() {
	// Provide a unix address to listen to, this will be the `address`
	// in the `proxy_plugin` configuration.
	// The root will be used to store the snapshots.
	if len(os.Args) < 3 {
		fmt.Printf("invalid args: usage: %s <unix addr> <root>\n", os.Args[0])
		os.Exit(1)
	}

	// Create a gRPC server
	rpc := grpc.NewServer()

	sn, err := overlay.NewSnapshotter(filepath.Join(os.Args[2], "snapshotter"))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	st, err := local.NewStore(filepath.Join(os.Args[2], "store"))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	// Convert the snapshotter to a gRPC service,
	// example in github.com/containerd/containerd/contrib/snapshotservice
	snService := snapshotservice.FromSnapshotter(sn)

	// Convert the content store to a gRPC service,
	stService := contentserver.New(st)

	// Register the services with the gRPC server
	snapshotsapi.RegisterSnapshotsServer(rpc, snService)
	contentapi.RegisterContentServer(rpc, stService)

	// Listen and serve
	l, err := net.Listen("unix", os.Args[1])
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if err := rpc.Serve(l); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}
