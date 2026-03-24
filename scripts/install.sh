#!/bin/sh

set -eu

REPO="dusk-network/pituitary"

log() {
  printf '%s\n' "$*" >&2
}

fail() {
  log "pituitary install: $*"
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

resolve_version() {
  version_input="${PITUITARY_VERSION:-${1:-latest}}"
  if [ "$version_input" = "latest" ]; then
    tag="$(gh api "repos/${REPO}/releases/latest" -q .tag_name)"
    [ -n "$tag" ] || fail "failed to resolve the latest release tag"
  else
    case "$version_input" in
      v*) tag="$version_input" ;;
      *) tag="v$version_input" ;;
    esac
  fi

  version="${tag#v}"
}

detect_platform() {
  os_name="$(uname -s)"
  arch_name="$(uname -m)"

  case "$os_name" in
    Linux) os_slug="linux" ;;
    Darwin) os_slug="macOS" ;;
    *) fail "unsupported operating system: $os_name" ;;
  esac

  case "$arch_name" in
    x86_64|amd64) arch_slug="amd64" ;;
    arm64|aarch64) arch_slug="arm64" ;;
    *) fail "unsupported architecture: $arch_name" ;;
  esac

  case "${os_slug}/${arch_slug}" in
    linux/amd64|macOS/arm64) ;;
    *) fail "unsupported release target: ${os_slug}/${arch_slug}" ;;
  esac

  archive="pituitary_${version}_${os_slug}_${arch_slug}.tar.gz"
  checksums="pituitary_${tag}_checksums.txt"
}

choose_install_dir() {
  install_dir="${PITUITARY_INSTALL_DIR:-}"
  if [ -n "$install_dir" ]; then
    return
  fi

  if [ "$(id -u)" -eq 0 ] || [ -w /usr/local/bin ]; then
    install_dir="/usr/local/bin"
  else
    install_dir="${HOME}/.local/bin"
  fi
}

sha256_file() {
  file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return
  fi

  fail "missing required command: sha256sum or shasum"
}

verify_checksum() {
  archive_path="$1"
  checksum_path="$2"

  expected="$(
    awk -v name="$archive" '$2 == name { print $1 }' "$checksum_path"
  )"
  [ -n "$expected" ] || fail "no checksum found for ${archive}"

  actual="$(sha256_file "$archive_path")"
  [ "$actual" = "$expected" ] || fail "checksum mismatch for ${archive}"
}

install_binary() {
  mkdir -p "$install_dir"
  if command -v install >/dev/null 2>&1; then
    install -m 0755 "$tmpdir/pituitary" "$install_dir/pituitary"
  else
    cp "$tmpdir/pituitary" "$install_dir/pituitary"
    chmod 0755 "$install_dir/pituitary"
  fi
}

main() {
  need_cmd gh
  need_cmd tar
  need_cmd awk
  need_cmd uname
  need_cmd mktemp
  need_cmd id
  gh auth status >/dev/null 2>&1 || fail "GitHub CLI authentication is required; run 'gh auth login' first"

  resolve_version "${1:-latest}"
  detect_platform
  choose_install_dir

  tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/pituitary-install.XXXXXX")"
  trap 'rm -rf "$tmpdir"' EXIT INT TERM

  archive_path="${tmpdir}/${archive}"
  checksum_path="${tmpdir}/${checksums}"

  log "pituitary install: downloading ${tag} for ${os_slug}/${arch_slug}"
  gh release download "$tag" \
    --repo "$REPO" \
    --pattern "$archive" \
    --pattern "$checksums" \
    --dir "$tmpdir" \
    --clobber
  verify_checksum "$archive_path" "$checksum_path"

  tar -xzf "$archive_path" -C "$tmpdir"
  [ -f "$tmpdir/pituitary" ] || fail "archive did not contain a pituitary binary"

  install_binary

  log "pituitary install: installed to ${install_dir}/pituitary"
  "${install_dir}/pituitary" version

  case ":${PATH:-}:" in
    *:"$install_dir":*) ;;
    *)
      log "pituitary install: add ${install_dir} to PATH if you want to invoke pituitary without a full path"
      ;;
  esac
}

main "$@"
