# scripts/lib/leak-guard.sh
# Fails if any tracked file contains a token from the LEAK_GUARD_TOKENS
# denylist. The denylist lives in the env, not in this file — the token
# set itself is private. Usage:
#   export LEAK_GUARD_TOKENS='tokA|tokB|host\.example\.com'
#   bash scripts/lib/leak-guard.sh .

leak_guard_scan() {
  local root="${1:-.}"
  if [[ -z "${LEAK_GUARD_TOKENS:-}" ]]; then
    echo "leak-guard: LEAK_GUARD_TOKENS env var is required" >&2
    echo "  export it in a private rc (e.g. ~/.envrc.private) as a" >&2
    echo "  pipe-separated alternation of tokens to denylist." >&2
    return 2
  fi
  # git grep scans only tracked + untracked-non-ignored files.
  # Word boundaries so a token like 'foo' matches 'foo' but not 'foobar'.
  if git -C "${root}" grep -InE "\\b(${LEAK_GUARD_TOKENS})\\b" 2>/dev/null; then
    echo "" >&2
    echo "leak-guard: denylisted tokens found in tracked content; commit BLOCKED" >&2
    return 1
  fi
  return 0
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  set -euo pipefail
  leak_guard_scan "${1:-.}"
fi
