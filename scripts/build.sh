#!/bin/bash

script_dir=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
src_dir=$script_dir/..

mkdir -p $HOME/go/pkg

podman build -f $script_dir/Dockerfile -t localhost/lsdc2-pilot:build-image $src_dir
podman run \
    --rm \
    -v $src_dir:/go/src \
    -v $HOME/go/pkg:/go/pkg \
    --workdir /go/src \
    localhost/lsdc2-pilot:build-image \
    /bin/bash -c 'go get ./... && go build --ldflags  "\
        -X main.Version=$(git describe --tags --always --dirty) \
        -X main.Commit=$(git rev-parse HEAD) \
        -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
        -w \
        -s \
        -extldflags \"-static\"\
    " \
    -o lsdc2-pilot \
    cmd/main.go '

