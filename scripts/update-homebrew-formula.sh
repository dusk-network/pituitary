#!/bin/sh

set -eu

REPO="dusk-network/pituitary"
RELEASES_URL="https://github.com/${REPO}/releases"

log() {
  printf '%s\n' "$*" >&2
}

fail() {
  log "pituitary tap update: $*"
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

resolve_tag() {
  if [ $# -lt 1 ] || [ -z "$1" ]; then
    fail "usage: ./scripts/update-homebrew-formula.sh <tag> [tap-path]"
  fi

  case "$1" in
    v*) tag="$1" ;;
    *) tag="v$1" ;;
  esac
  version="${tag#v}"
}

resolve_tap_path() {
  tap_path="${2:-${PITUITARY_HOMEBREW_TAP_PATH:-}}"
  [ -n "$tap_path" ] || fail "missing tap path; pass it as the second argument or PITUITARY_HOMEBREW_TAP_PATH"
  [ -d "$tap_path/.git" ] || fail "tap path is not a git repository: $tap_path"
  mkdir -p "$tap_path/Formula"
}

read_checksum() {
  name="$1"
  awk -v target="$name" '$2 == target { print $1 }' "$checksum_path"
}

write_formula() {
  formula_path="$tap_path/Formula/pituitary.rb"
  cat >"$formula_path" <<EOF
class Pituitary < Formula
  desc "Spec management tool for keeping specifications and documentation aligned"
  homepage "https://github.com/${REPO}"
  license "MIT"
  version "${version}"

  on_macos do
    on_arm do
      url "https://github.com/${REPO}/releases/download/${tag}/pituitary_${version}_macOS_arm64.tar.gz"
      sha256 "${darwin_sha}"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/${REPO}/releases/download/${tag}/pituitary_${version}_linux_amd64.tar.gz"
      sha256 "${linux_sha}"
    end
  end

  def install
    bin.install "pituitary"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/pituitary version")
  end
end
EOF
}

main() {
  need_cmd curl
  need_cmd awk
  need_cmd mktemp

  resolve_tag "$@"
  resolve_tap_path "$@"

  tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/pituitary-tap.XXXXXX")"
  trap 'rm -rf "$tmpdir"' EXIT INT TERM

  checksum_path="${tmpdir}/pituitary_${tag}_checksums.txt"
  curl -fsSL -o "$checksum_path" \
    "${RELEASES_URL}/download/${tag}/pituitary_${tag}_checksums.txt" \
    || fail "failed to download checksums for ${tag}"

  linux_sha="$(read_checksum "pituitary_${version}_linux_amd64.tar.gz")"
  darwin_sha="$(read_checksum "pituitary_${version}_macOS_arm64.tar.gz")"

  [ -n "$linux_sha" ] || fail "missing linux checksum for ${tag}"
  [ -n "$darwin_sha" ] || fail "missing macOS checksum for ${tag}"

  write_formula

  log "pituitary tap update: wrote ${tap_path}/Formula/pituitary.rb for ${tag}"
  log "pituitary tap update: next step:"
  log "  git -C ${tap_path} add Formula/pituitary.rb && git -C ${tap_path} commit -m \"Update pituitary to ${tag}\" && git -C ${tap_path} push"
}

main "$@"
