#!/bin/sh
set -eu

REPO="samuelloranger/glim"
BIN="glim"
DEST="${GLIM_INSTALL_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*)
		echo "glim: unsupported architecture: $arch" >&2
		exit 1
		;;
esac
case "$os" in
	linux | darwin) ;;
	*)
		echo "glim: unsupported OS: $os" >&2
		exit 1
		;;
esac

if [ -n "${GLIM_VERSION:-}" ]; then
	ver="$GLIM_VERSION"
else
	ver=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
		grep '"tag_name"' | head -1 | cut -d'"' -f4)
fi
if [ -z "$ver" ]; then
	echo "glim: could not determine latest version" >&2
	exit 1
fi

url="https://github.com/$REPO/releases/download/$ver/${BIN}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "glim: downloading $ver ($os/$arch)"
curl -fsSL "$url" | tar -xz -C "$tmp"
mkdir -p "$DEST"
install -m 0755 "$tmp/$BIN" "$DEST/$BIN"
echo "glim: installed to $DEST/$BIN"

case ":$PATH:" in
	*":$DEST:"*) ;;
	*) echo "glim: add $DEST to your PATH" >&2 ;;
esac

echo "glim: run 'glim config --domain https://glim.example.com' then 'glim install claude'"
