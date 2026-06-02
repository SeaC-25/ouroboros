#!/usr/bin/env sh
set -eu

usage() {
	printf 'usage: %s <local-release-archive-or-directory>\n' "$0" >&2
}

if [ "$#" -ne 1 ]; then
	usage
	exit 2
fi

SOURCE="$1"
PREFIX="${PREFIX:-"$HOME/.local"}"
BIN_DIR="${BIN_DIR:-"$PREFIX/bin"}"
TMP_DIR=""

cleanup() {
	if [ -n "$TMP_DIR" ]; then
		rm -rf "$TMP_DIR"
	fi
}
trap cleanup EXIT INT TERM

find_binary() {
	root="$1"
	for candidate in "$root/ouro" "$root"/ouro_*/ouro "$root/ouro.exe" "$root"/ouro_*/ouro.exe; do
		if [ -f "$candidate" ]; then
			printf '%s\n' "$candidate"
			return 0
		fi
	done
	return 1
}

case "$SOURCE" in
	http://*|https://*)
		printf 'install source must be a local archive or directory, got URL: %s\n' "$SOURCE" >&2
		exit 2
		;;
esac

if [ -d "$SOURCE" ]; then
	BINARY="$(find_binary "$SOURCE")" || {
		printf 'could not find ouro binary in %s\n' "$SOURCE" >&2
		exit 1
	}
elif [ -f "$SOURCE" ]; then
	case "$SOURCE" in
		*.tar.gz|*.tgz)
			TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ouro-install-XXXXXX")"
			tar -xzf "$SOURCE" -C "$TMP_DIR"
			BINARY="$(find_binary "$TMP_DIR")" || {
				printf 'could not find ouro binary in archive %s\n' "$SOURCE" >&2
				exit 1
			}
			;;
		*)
			printf 'unsupported install source: %s\n' "$SOURCE" >&2
			exit 2
			;;
	esac
else
	printf 'install source does not exist: %s\n' "$SOURCE" >&2
	exit 1
fi

mkdir -p "$BIN_DIR"
cp "$BINARY" "$BIN_DIR/ouro"
chmod 0755 "$BIN_DIR/ouro"
printf 'installed %s\n' "$BIN_DIR/ouro"
