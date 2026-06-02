#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
VERSION="${VERSION:-0.1.0-dev}"
DIST_DIR="${DIST_DIR:-"$ROOT/dist"}"
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"
BINARY_NAME="ouro"

case "$DIST_DIR" in
	/*) ;;
	*) DIST_DIR="$ROOT/$DIST_DIR" ;;
esac

case "$VERSION" in
	""|*/*)
		printf 'invalid VERSION %s\n' "$VERSION" >&2
		exit 1
		;;
esac

if [ "$GOOS" = "windows" ]; then
	BINARY_NAME="ouro.exe"
fi

PACKAGE="ouro_${VERSION}_${GOOS}_${GOARCH}"
RELEASE_DIR="$DIST_DIR/$PACKAGE"
ARCHIVE="$DIST_DIR/$PACKAGE.tar.gz"
SUMS="$DIST_DIR/SHA256SUMS"

mkdir -p "$DIST_DIR"
rm -rf "$RELEASE_DIR" "$ARCHIVE"
mkdir -p "$RELEASE_DIR"

cd "$ROOT"
GOPROXY="$GOPROXY" GOOS="$GOOS" GOARCH="$GOARCH" go build -o "$RELEASE_DIR/$BINARY_NAME" ./cmd/ouro
cp "$ROOT/scripts/install.sh" "$RELEASE_DIR/install.sh"
chmod 0755 "$RELEASE_DIR/install.sh"
cat > "$RELEASE_DIR/README.md" <<EOF
# Ouroboros $VERSION

This archive contains the Ouroboros CLI for $GOOS/$GOARCH.

Install from this local archive or extracted directory:

\`\`\`sh
PREFIX="\${HOME}/.local" sh install.sh .
\`\`\`

No network access is required by the installer.
EOF

(cd "$DIST_DIR" && tar -czf "$PACKAGE.tar.gz" "$PACKAGE")
if command -v sha256sum >/dev/null 2>&1; then
	(cd "$DIST_DIR" && sha256sum "$PACKAGE.tar.gz" > "$SUMS")
elif command -v shasum >/dev/null 2>&1; then
	(cd "$DIST_DIR" && shasum -a 256 "$PACKAGE.tar.gz" > "$SUMS")
else
	printf 'sha256sum or shasum is required to write checksums\n' >&2
	exit 1
fi

printf 'release: %s\n' "$ARCHIVE"
printf 'checksums: %s\n' "$SUMS"
