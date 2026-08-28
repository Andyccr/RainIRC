#!/bin/sh
# One-shot install for p2pirc. No Docker, no root required by default.
# Prefers a GitHub Release binary (checksum verified), then go install.
# Installs to $PREFIX/bin (default: ~/.local/bin).
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
		echo "p2pirc-install: unsupported OS $os" >&2
		return 1
		;;
	esac
	case "$arch" in
	x86_64|amd64) arch=amd64 ;;
	aarch64|arm64) arch=arm64 ;;
	*)
		echo "p2pirc-install: unsupported arch $arch" >&2
		return 1
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
		return 1
	fi
}

digest() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		echo "p2pirc-install: need sha256sum or shasum to verify the release" >&2
		return 1
	fi
}

release_urls() {
	name=$1
	if [ "$VERSION" = latest ]; then
		echo "https://github.com/${REPO}/releases/latest/download/${name}"
		echo "https://github.com/${REPO}/releases/latest/download/SHA256SUMS"
	else
		echo "https://github.com/${REPO}/releases/download/${VERSION}/${name}"
		echo "https://github.com/${REPO}/releases/download/${VERSION}/SHA256SUMS"
	fi
}

try_release() {
	spec=$(os_arch) || return 1
	# shellcheck disable=SC2086
	set -- $spec
	os=$1
	arch=$2
	ext=
	if [ "$os" = windows ]; then
		ext=.exe
	fi
	name="p2pirc-${os}-${arch}${ext}"
	binurl=$(release_urls "$name" | sed -n '1p')
	sumurl=$(release_urls "$name" | sed -n '2p')
	echo "p2pirc-install: download ${binurl}"
	fetch "$binurl" "$bin" || return 1
	fetch "$sumurl" "$tmp/SHA256SUMS" || return 1
	expected=$(awk -v n="$name" '$2 == n { print $1; exit }' "$tmp/SHA256SUMS")
	if [ -z "$expected" ]; then
		echo "p2pirc-install: SHA256SUMS has no entry for ${name}" >&2
		return 1
	fi
	got=$(digest "$bin") || return 1
	if [ "$got" != "$expected" ]; then
		echo "p2pirc-install: checksum mismatch for ${name}" >&2
		echo "p2pirc-install: want ${expected}" >&2
		echo "p2pirc-install: got  ${got}" >&2
		return 1
	fi
	echo "p2pirc-install: checksum ok"
	return 0
}

try_go() {
	if ! command -v go >/dev/null 2>&1; then
		return 1
	fi
	echo "p2pirc-install: go install ${module}@${VERSION}"
	GOBIN="$tmp" go install "${module}@${VERSION}"
	if [ -f "$tmp/p2pirc.exe" ] && [ ! -f "$tmp/p2pirc" ]; then
		mv "$tmp/p2pirc.exe" "$bin"
	fi
	[ -f "$bin" ]
}

mkdir -p "$BINDIR"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
bin="$tmp/p2pirc"

if ! try_release; then
	echo "p2pirc-install: release binary unavailable, trying go install"
	if ! try_go; then
		echo "p2pirc-install: failed (need a GitHub Release asset or Go 1.22+)" >&2
		exit 1
	fi
fi

chmod 755 "$bin"
cp "$bin" "${BINDIR}/p2pirc.new"
chmod 755 "${BINDIR}/p2pirc.new"
mv "${BINDIR}/p2pirc.new" "${BINDIR}/p2pirc"
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
