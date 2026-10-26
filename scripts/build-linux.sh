#!/bin/sh
# Compila build/bin/leggio-linux-<arch> dentro Docker (amd64 di default).
#   scripts/build-linux.sh [amd64|arm64]
set -e
arch=${1:-amd64}
cd "$(dirname "$0")/.."
host=$(docker info -f '{{.Architecture}}')
case "$arch/$host" in
  amd64/x86_64)
    docker build --build-arg ARCH=amd64 --build-arg BASE=amd64/ubuntu:22.04 -t leggio-build:amd64 build/linux ;;
  arm64/aarch64)
    docker build --build-arg ARCH=arm64 --build-arg BASE=arm64v8/ubuntu:22.04 -t leggio-build:arm64 build/linux ;;
  amd64/aarch64)
    # Mac Apple Silicon: compilazione incrociata, vedi Dockerfile.cross.
    docker build -f build/linux/Dockerfile.cross -t leggio-build:amd64 build/linux ;;
  *)
    echo "da $host non so compilare per $arch" >&2
    exit 1 ;;
esac
docker run --rm -v "$PWD:/src" -v "leggio-gocache-$arch:/root/.cache" -v leggio-gomod:/root/go/pkg/mod \
  "leggio-build:$arch" \
  go build -tags desktop,production,webkit2_41 -trimpath -ldflags "-s -w" -o "build/bin/leggio-linux-$arch" .
ls -lh "build/bin/leggio-linux-$arch"
