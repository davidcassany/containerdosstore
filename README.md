# OCIStore

This is a proof of concept to build a storage library for OCI images based on containerd v2 stack using pure
local services, so no containerd daemon is required. This PoC is in the scope of
[Elemental Toolkit](https://github.com/rancher/elemental-toolkit) project, the actual goal would be to built
a library based on containerd that can be used to implement the Elemental Toolkit snapshotter interface.
Having such a library would allow Elemental Toolkit to manage host OS as a regular OCI image using the same
actual stack to store, unpack and mount OCI artifacts as any Containerd based K8s distro.

## Build

```bash
make build
```

## Run

```bash
$ ocistore --help
A daememon less client for a local image containerd store

Usage:
  ocistore [command]

Available Commands:
  commit         Commit given active snapshot as a new image
  delete         Deletes the given image
  help           Help about any command
  import         Imports the given OCI archive
  list           Lists all images
  list-snapshots Lists all available snapshots
  mount          Mounts the given image name to the given target mountpoint
  pull           pulls a remote image into containerd store
  umount         Unmounts the given mountpoint
  unpack         Unpacks the given image

Flags:
      --debug             set log ouput to debug level
  -h, --help              help for ocistore
      --loglevel string   set log ouput level
      --root string       path for the containerd local store (default "/tmp/contentstore")
```
