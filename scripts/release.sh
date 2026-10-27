#!/usr/bin/env sh
set -eu

case "$0" in
	*/*) ROOT_DIR="${0%/*}" ;;
	*) ROOT_DIR="." ;;
esac
ROOT="$(CDPATH= cd -- "$ROOT_DIR/.." && pwd)"
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
if [ -d "$ROOT/toolchains/managed-bundles" ]; then
	mkdir -p "$RELEASE_DIR/toolchains"
	cp -R "$ROOT/toolchains/managed-bundles" "$RELEASE_DIR/toolchains/managed-bundles"
fi
cat > "$RELEASE_DIR/README.md" <<EOF
# Ouroboros $VERSION

This archive contains the Ouroboros CLI for $GOOS/$GOARCH.

Install from this local archive or extracted directory:

\`\`\`sh
PREFIX="\${HOME}/.local" sh install.sh .
\`\`\`

No network access is required by the installer.

Keep SHA256SUMS next to the archive when installing from the tarball. The
installer verifies the matching archive entry before extraction when that file
is present.

If this archive includes managed tool bundles, the installer copies them to:

\`\`\`text
\${PREFIX}/share/ouroboros/toolchains/managed-bundles
\`\`\`

The bundled nuclei-templates seed carries offline corpus metadata, including
corpus_status, corpus_name, corpus_source, corpus_retrieved_at,
corpus_file_count, corpus_sha256, corpus_digest_status,
corpus_digest_algorithm, and corpus_digest_scope, so operators can verify
the packaged rule-corpus index without downloading, syncing, or executing it.

Import a bundled tool into a workspace explicitly:

\`\`\`sh
ouro toolchain release-managed-bundles "\${PREFIX}/share/ouroboros"
ouro toolchain release-managed-status <workspace> "\${PREFIX}/share/ouroboros" nuclei-templates
ouro toolchain release-managed-status <workspace> "\${PREFIX}/share/ouroboros" nuclei-templates --expected-sha256 <sha256>
ouro toolchain release-managed-status-source-set <workspace> "\${PREFIX}/share/ouroboros" --set --set-by operator --reason "current release"
ouro toolchain release-managed-status-source-set <workspace> "\${PREFIX}/share/ouroboros" --set --pin-bundle nuclei-templates --expected-sha256 <sha256> --set-by operator --reason "pinned release"
ouro toolchain release-managed-status-source <workspace>
ouro toolchain managed-update-request <workspace> "\${PREFIX}/share/ouroboros" <bundle> --request --requested-by operator --reason "review update"
ouro toolchain managed-update-requests <workspace>
ouro toolchain release-managed-install <workspace> "\${PREFIX}/share/ouroboros" web-api-pack nuclei-templates --expected-sha256 <sha256> --execute
\`\`\`

Import operator-supplied local binary or rule-bundle payloads as workspace-managed
bundles without running them or creating final install links:

\`\`\`sh
ouro toolchain managed-bundle-import <workspace> httpx ./httpx --kind binary --version 1.3.0 --source operator-local/httpx --expected-sha256 <sha256> --import
ouro toolchain managed-bundle-import <workspace> nuclei-templates ./nuclei-templates --kind rule_bundle --version 2026.06.05 --source operator-local/nuclei-templates --expected-sha256 <sha256> --import
ouro toolchain managed-bundles <workspace> httpx
ouro toolchain managed-install-plan <workspace> web-api-pack httpx
\`\`\`
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
