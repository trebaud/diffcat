#!/bin/bash
set -euo pipefail

APP_NAME="sashi"
BUILD_DIR="./dist"

# Resolve version: explicit arg > current tag > git describe > "dev"
VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"

PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
)

rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

echo "Building $APP_NAME $VERSION..."

for PLATFORM in "${PLATFORMS[@]}"; do
    IFS="/" read -r -a split <<< "$PLATFORM"
    GOOS=${split[0]}
    GOARCH=${split[1]}

    BASE="${APP_NAME}-${VERSION}-${GOOS}-${GOARCH}"
    STAGE="${BUILD_DIR}/${BASE}"
    mkdir -p "$STAGE"

    echo "  -> $GOOS/$GOARCH"
    env GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 go build \
        -trimpath \
        -ldflags "-s -w -X main.ldflagsVersion=${VERSION}" \
        -o "${STAGE}/${APP_NAME}" \
        ./cmd/sashi

    tar -czf "${BUILD_DIR}/${BASE}.tar.gz" -C "$BUILD_DIR" "$BASE"
    rm -rf "$STAGE"
done

# Per-archive checksum file (portable: shasum on macOS, sha256sum on linux).
# One .sha256 per archive so each can be uploaded as a separate GitHub release asset.
pushd "$BUILD_DIR" > /dev/null
if command -v sha256sum > /dev/null; then
    SHA_CMD="sha256sum"
else
    SHA_CMD="shasum -a 256"
fi
for archive in *.tar.gz; do
    $SHA_CMD "$archive" > "${archive}.sha256"
done
popd > /dev/null

echo "Done. Artifacts in $BUILD_DIR:"
ls -1 "$BUILD_DIR"

echo "Creating and pushing git tag $VERSION..."
git tag "$VERSION"
git push origin "$VERSION"
echo "Tagged $VERSION"
