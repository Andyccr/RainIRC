#!/bin/sh
# One-shot install for p2pirc. No Docker, no root required by default.
# Installs a single static binary to $PREFIX/bin (default: ~/.local/bin).
#
#   curl -fsSL https://raw.githubusercontent.com/Andyccr/RainIRC/main/scripts/install.sh | sh
#   PREFIX=/usr/local sh scripts/install.sh
#
set -e

REPO="${P2PIRC_REPO:-Andyccr/RainIRC}"
VERSION="${P2PIRC_VERSION:-latest}"
PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="${BINDIR:-$PREFIX/bin}"

module="github.com/${REPO}/cmd/p2pirc"

os_arch() {
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	arch=$(uname -m)
	case "$os" in
	linux) os=linux ;;
	darwin) os=darwin ;;
	mingw*|msys*|cygwin*) os=windows ;;
	*)
		echo "p2pirc-install: unsupported OS $os (install Go 1.22+ and run: go install ${module}@${VERSION})" >&2
		exit 1
		;;
	esac
	case "$arch" in
	x86_64|amd64) arch=amd64 ;;
	aarch64|arm64) arch=arm64 ;;
	*)
		echo "p2pirc-install: unsupported arch $arch" >&2
		exit 1
		;;
	esac
	echo "${os} ${arch}"
}

fetch() {
	url=$1
	out=$2
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$out"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$out" "$url"
	else
		echo "p2pirc-install: need curl or wget to download a release binary" >&2
		exit 1
	fi
}

mkdir -p "$BINDIR"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/p2pirc"

if command -v go >/dev/null 2>&1; then
	echo "p2pirc-install: go install ${module}@${VERSION}"
	GOBIN="$tmp" go install "${module}@${VERSION}"
	if [ -f "$tmp/p2pirc.exe" ] && [ ! -f "$tmp/p2pirc" ]; then
		mv "$tmp/p2pirc.exe" "$bin"
	fi
else
	set -- $(os_arch)
	os=$1
	arch=$2
	ext=
	if [ "$os" = windows ]; then
		ext=.exe
	fi
	name="p2pirc-${os}-${arch}${ext}"
	if [ "$VERSION" = latest ]; then
		url="https://github.com/${REPO}/releases/latest/download/${name}"
	else
		url="https://github.com/${REPO}/releases/download/${VERSION}/${name}"
	fi
	echo "p2pirc-install: download ${url}"
	fetch "$url" "$bin"
fi

if [ ! -f "$bin" ]; then
	echo "p2pirc-install: binary missing after install" >&2
	exit 1
fi
chmod 755 "$bin"
cp "$bin" "${BINDIR}/p2pirc"
chmod 755 "${BINDIR}/p2pirc"
echo "p2pirc-install: ${BINDIR}/p2pirc"
"${BINDIR}/p2pirc" --version

case ":$PATH:" in
*":${BINDIR}:"*) ;;
*)
	echo "p2pirc-install: add ${BINDIR} to PATH, then run: p2pirc --lan --nickname Alice" >&2
	;;
esac
echo "p2pirc-install: LAN one-shot: p2pirc --lan --nickname Alice"
echo "p2pirc-install: persist flags in ~/.p2pirc/config  (see docs/deploy.md)"
