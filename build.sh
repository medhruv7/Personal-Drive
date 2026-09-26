#!/usr/bin/env sh
# Cross-compile Local Drive for every supported platform into dist/.
# Pure Go (no cgo), so this works from any OS with Go installed.
set -eu
cd "$(dirname "$0")"
mkdir -p dist
for target in darwin/arm64 darwin/amd64 windows/amd64 windows/arm64 linux/amd64 linux/arm64; do
  os=${target%/*}; arch=${target#*/}
  out="dist/localdrive-$os-$arch"
  [ "$os" = windows ] && out="$out.exe"
  echo "building $out"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w" -o "$out" .
done
