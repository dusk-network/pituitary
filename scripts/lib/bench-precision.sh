# scripts/lib/bench-precision.sh
# Shared helpers for retrieval precision benches. Consumers are expected to
# `set -euo pipefail` themselves — sourcing this file must not change the
# caller's global shell behavior.
#
# Exported helpers:
#   bench_read_workspace_index_path <cfg.toml>
#   bench_rewrite_workspace_index_path <src.toml> <dst.toml> <new_path>
#   bench_read_corpus_pins <pinfile.toml>          -> tab-separated path<TAB>sha
#   bench_enforce_corpus_pins <pinfile.toml>       -> exit 2 on drift
#   bench_file_size_bytes <path>                   -> size in bytes, 0 if missing

# Read [workspace].index_path from a TOML file. Scoped to the [workspace] table.
bench_read_workspace_index_path() {
  local cfg="$1"
  awk '
    BEGIN { in_ws = 0 }
    /^\[workspace\][[:space:]]*$/ { in_ws = 1; next }
    in_ws && /^\[/ { in_ws = 0 }
    in_ws && /^[[:space:]]*index_path[[:space:]]*=/ {
      sub(/^[^=]*=[[:space:]]*/, "")
      gsub(/^"|"[[:space:]]*$|^'\''|'\''[[:space:]]*$/, "")
      sub(/[[:space:]]+$/, "")
      print
      exit
    }
  ' "${cfg}"
}

# Rewrite [workspace].index_path in a TOML file and write the result to dst.
bench_rewrite_workspace_index_path() {
  local src="$1" dst="$2" new_path="$3"
  awk -v new_path="${new_path}" '
    BEGIN { in_ws = 0 }
    /^\[workspace\][[:space:]]*$/ { in_ws = 1; print; next }
    in_ws && /^\[/ && !/^\[workspace\]/ { in_ws = 0 }
    in_ws && /^[[:space:]]*index_path[[:space:]]*=/ {
      printf "index_path = \"%s\"\n", new_path
      next
    }
    { print }
  ' "${src}" > "${dst}"
}

# Read [[corpus.repo]] blocks from a pin file. Each block has path and sha keys.
# Emits TAB-separated lines `path<TAB>sha` to stdout.
bench_read_corpus_pins() {
  local pinfile="$1"
  awk '
    BEGIN { in_repo = 0; path = ""; sha = "" }
    /^\[\[corpus\.repo\]\]/ {
      if (in_repo && path != "" && sha != "") print path "\t" sha
      in_repo = 1; path = ""; sha = ""; next
    }
    /^\[/ && !/^\[\[corpus\.repo\]\]/ {
      if (in_repo && path != "" && sha != "") print path "\t" sha
      in_repo = 0; path = ""; sha = ""
    }
    in_repo && /^[[:space:]]*path[[:space:]]*=/ {
      sub(/^[^=]*=[[:space:]]*/, ""); gsub(/^"|"$/, ""); path = $0
    }
    in_repo && /^[[:space:]]*sha[[:space:]]*=/ {
      sub(/^[^=]*=[[:space:]]*/, ""); gsub(/^"|"$/, ""); sha = $0
    }
    END {
      if (in_repo && path != "" && sha != "") print path "\t" sha
    }
  ' "${pinfile}"
}

# Check each pinned repo's HEAD against the pin. Returns 0 if all match.
# Returns 2 on any mismatch and prints details to stderr. Prefix-matches
# the SHA so pin files may record short SHAs.
bench_enforce_corpus_pins() {
  local pinfile="$1"
  local failed=0
  while IFS=$'\t' read -r repo_path pin_sha; do
    if [[ -z "${repo_path}" || -z "${pin_sha}" ]]; then
      continue
    fi
    # Accept both classic repos (.git dir) and worktrees (.git file pointer).
    if [[ ! -e "${repo_path}/.git" ]]; then
      echo "bench: pin target is not a git repo: ${repo_path}" >&2
      failed=1; continue
    fi
    local head_sha
    head_sha="$(git -C "${repo_path}" rev-parse HEAD 2>/dev/null || echo '?')"
    if [[ "${head_sha}" != "${pin_sha}"* ]]; then
      echo "bench: pin drift for ${repo_path}: HEAD=${head_sha} pin=${pin_sha}" >&2
      failed=1
    fi
  done < <(bench_read_corpus_pins "${pinfile}")
  if (( failed != 0 )); then
    echo "bench: corpus pins are not honored; refusing to run" >&2
    return 2
  fi
  return 0
}

# Emit the size in bytes of the given file (0 if missing).
bench_file_size_bytes() {
  local path="$1"
  if [[ -f "${path}" ]]; then
    if stat -f '%z' "${path}" 2>/dev/null; then return 0; fi
    stat -c '%s' "${path}"
  else
    echo 0
  fi
}
