# Directory of Makefile
export ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

LDFLAGS:=-w -s

.PHONY: build
build:
	go build -ldflags '$(LDFLAGS)' -mod=vendor -o build/snapshotter
