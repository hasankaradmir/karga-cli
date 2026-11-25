#!/bin/sh
# Installs a karga CLI release (Linux or macOS) into $KARGA_BIN_DIR (default /usr/local/bin) after
# checking it against the release's checksums.txt.
#
#   KARGA_VERSION       tag to install (default: the latest published release)
#   KARGA_BIN_DIR       where to put the binary
#   KARGA_DOWNLOAD_URL  download from here instead of GitHub (a mirror, or file:// for a local build)
#   KARGA_SKIP_SIGNATURE=1  do not verify the signature (a local build has none)
#
# karga needs karga-server on the host it manages: https://github.com/hasankaradmir/karga-server
set -eu

repo=hasankaradmir/karga-cli
bin_dir=${KARGA_BIN_DIR:-/usr/local/bin}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $os in linux|darwin) ;; *) echo "unsupported OS: $os" >&2; exit 1 ;; esac
arch=$(uname -m)
case $arch in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo "unsupported architecture: $arch" >&2; exit 1 ;; esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

version=${KARGA_VERSION:-}
if [ -z "$version" ]; then
  [ -z "${KARGA_DOWNLOAD_URL:-}" ] || { echo "set KARGA_VERSION with KARGA_DOWNLOAD_URL" >&2; exit 1; }
  # GitHub redirects /releases/latest to the tag of the latest published release.
  version=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest" | sed -n 's#.*/tag/##p')
  [ -n "$version" ] || { echo "no published release found" >&2; exit 1; }
fi
url=${KARGA_DOWNLOAD_URL:-https://github.com/$repo/releases/download/$version}
archive="karga_${version#v}_${os}_${arch}.tar.gz"

for f in "$archive" checksums.txt; do
  curl -fsSL -o "$tmp/$f" "$url/$f"
done
want=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null; then got=$(sha256sum "$tmp/$archive" | cut -d' ' -f1); else got=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1); fi
[ -n "$want" ] && [ "$want" = "$got" ] || { echo "checksum mismatch for $archive" >&2; exit 1; }

# checksums.txt is signed by the release workflow (Sigstore, no key): with cosign installed, the
# archive is proven to come from this repository's release, not only to be intact.
if [ "${KARGA_SKIP_SIGNATURE:-0}" != 1 ] && command -v cosign >/dev/null; then
  curl -fsSL -o "$tmp/checksums.txt.sig" "$url/checksums.txt.sig"
  curl -fsSL -o "$tmp/checksums.txt.pem" "$url/checksums.txt.pem"
  cosign verify-blob --certificate "$tmp/checksums.txt.pem" --signature "$tmp/checksums.txt.sig" \
    --certificate-identity-regexp "^https://github.com/$repo/.github/workflows/release.yml@refs/tags/" \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com "$tmp/checksums.txt" >/dev/null 2>&1 \
    || { echo "the signature of checksums.txt does not verify" >&2; exit 1; }
elif [ "${KARGA_SKIP_SIGNATURE:-0}" != 1 ]; then
  echo "note: install cosign to also verify the release signature" >&2
fi

tar -xzf "$tmp/$archive" -C "$tmp"
# Under a temporary name next to it, then renamed: nothing ever runs half a file. A directory
# only root may write gets a binary only root may write (0755), not the installing user's.
if [ -w "$bin_dir" ]; then
  install -m 0755 "$tmp/karga" "$bin_dir/.karga.new"
  mv -f "$bin_dir/.karga.new" "$bin_dir/karga"
else
  as_root() { if [ "$(id -u)" = 0 ]; then "$@"; else sudo "$@"; fi; }
  as_root install -o root -g root -m 0755 "$tmp/karga" "$bin_dir/.karga.new"
  as_root mv -f "$bin_dir/.karga.new" "$bin_dir/karga"
fi
echo "karga $version installed to $bin_dir/karga"
