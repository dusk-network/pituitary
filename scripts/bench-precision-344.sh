#!/usr/bin/env bash
#
# Run the #344 precision@k benchmark across three chunking configs and emit a
# consolidated markdown report.
#
# Required env:
#   BENCH_BASE_CONFIG   path to a pituitary.toml that indexes a representative
#                       corpus with a live embedder. The config must NOT include
#                       a [runtime.chunking] block — overlays supply it.
#
# Optional env:
#   BENCH_CASES         path to the labeled cases JSON
#                       (default: testdata/retrieval-bench/ccd-guide-cases.json)
#   BENCH_OUT_DIR       where to drop per-variant JSON reports
#                       (default: /tmp/pituitary-bench-344)
#   BENCH_REPORT_MD     consolidated markdown output path
#                       (default: docs/development/retrieval-precision-344.md)

set -euo pipefail

if [[ -z "${BENCH_BASE_CONFIG:-}" ]]; then
  echo "bench-precision-344: BENCH_BASE_CONFIG is required" >&2
  exit 2
fi
if [[ ! -f "${BENCH_BASE_CONFIG}" ]]; then
  echo "bench-precision-344: base config not found: ${BENCH_BASE_CONFIG}" >&2
  exit 2
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CASES_DEFAULT="${REPO_ROOT}/testdata/retrieval-bench/ccd-guide-cases.json"
OVERLAY_DIR="${REPO_ROOT}/testdata/retrieval-bench/chunking-overlays"
BENCH_CASES="${BENCH_CASES:-${CASES_DEFAULT}}"
BENCH_OUT_DIR="${BENCH_OUT_DIR:-/tmp/pituitary-bench-344}"
BENCH_REPORT_MD="${BENCH_REPORT_MD:-${REPO_ROOT}/docs/development/retrieval-precision-344.md}"

mkdir -p "${BENCH_OUT_DIR}"

VARIANTS=(pre338 p338 p344)
BASE_INDEX_PATH="$(awk -F'=' '/^index_path/ { gsub(/[ "]/, "", $2); print $2; exit }' "${BENCH_BASE_CONFIG}")"
if [[ -z "${BASE_INDEX_PATH}" ]]; then
  echo "bench-precision-344: could not read [workspace] index_path from ${BENCH_BASE_CONFIG}" >&2
  exit 2
fi
BASE_INDEX_EXT="${BASE_INDEX_PATH##*.}"
BASE_INDEX_STEM="${BASE_INDEX_PATH%.*}"

run_variant() {
  local variant="$1"
  local overlay="${OVERLAY_DIR}/${variant}.toml"
  local effective_cfg="${BENCH_OUT_DIR}/${variant}.toml"
  local variant_index="${BASE_INDEX_STEM}.${variant}.${BASE_INDEX_EXT}"
  local report="${BENCH_OUT_DIR}/${variant}.json"

  # Rewrite the base config's index_path so the three variants write distinct
  # snapshots, then append the chunking overlay.
  awk -v new_path="${variant_index}" '
    BEGIN { in_ws = 0 }
    /^\[workspace\][[:space:]]*$/ { in_ws = 1; print; next }
    in_ws && /^\[/ && !/^\[workspace\]/ { in_ws = 0 }
    in_ws && /^[[:space:]]*index_path[[:space:]]*=/ {
      printf "index_path = \"%s\"\n", new_path
      next
    }
    { print }
  ' "${BENCH_BASE_CONFIG}" > "${effective_cfg}"
  if [[ -s "${overlay}" ]]; then
    printf '\n' >> "${effective_cfg}"
    cat "${overlay}" >> "${effective_cfg}"
  fi

  echo "==> ${variant}: rebuilding + benching (config: ${effective_cfg}, db: ${variant_index})"
  PITUITARY_PRECISION_CONFIG="${effective_cfg}" \
    PITUITARY_PRECISION_CASES="${BENCH_CASES}" \
    PITUITARY_PRECISION_REPORT="${report}" \
    PITUITARY_PRECISION_LABEL="${variant}" \
    go test -tags=precision_bench -run TestRetrievalPrecisionBench -count=1 \
      -timeout=30m -v ./internal/index/
}

for v in "${VARIANTS[@]}"; do
  run_variant "${v}"
done

# Render a consolidated markdown summary.
python3 - "${BENCH_REPORT_MD}" "${BENCH_OUT_DIR}" "${VARIANTS[@]}" <<'PY'
import json, os, sys, datetime

out_path = sys.argv[1]
out_dir = sys.argv[2]
variants = sys.argv[3:]

rows = []
for v in variants:
    path = os.path.join(out_dir, f"{v}.json")
    with open(path) as f:
        r = json.load(f)
    rows.append((v, r))

os.makedirs(os.path.dirname(out_path), exist_ok=True)
with open(out_path, "w") as f:
    f.write("# Retrieval precision benchmark — #344\n\n")
    f.write(f"Generated: {datetime.datetime.utcnow().isoformat()}Z\n\n")
    f.write(f"Cases file: `{rows[0][1]['cases_path']}`\n\n")
    f.write(f"Case count: {rows[0][1]['case_count']}\n\n")
    f.write("| variant | p@5   | p@10  | recall@10 | MRR   |\n")
    f.write("|---------|-------|-------|-----------|-------|\n")
    for v, r in rows:
        f.write(
            f"| {v} "
            f"| {r['mean_precision_at_5']:.3f} "
            f"| {r['mean_precision_at_10']:.3f} "
            f"| {r['mean_recall_at_10']:.3f} "
            f"| {r['mean_reciprocal_rank']:.3f} |\n"
        )
    baseline = rows[0][1]
    f.write("\n## Deltas vs pre-#338 baseline\n\n")
    f.write("| variant | Δp@5  | Δp@10 | Δrecall@10 | ΔMRR  |\n")
    f.write("|---------|-------|-------|------------|-------|\n")
    for v, r in rows:
        f.write(
            f"| {v} "
            f"| {r['mean_precision_at_5']-baseline['mean_precision_at_5']:+.3f} "
            f"| {r['mean_precision_at_10']-baseline['mean_precision_at_10']:+.3f} "
            f"| {r['mean_recall_at_10']-baseline['mean_recall_at_10']:+.3f} "
            f"| {r['mean_reciprocal_rank']-baseline['mean_reciprocal_rank']:+.3f} |\n"
        )
    f.write("\n## Per-variant raw reports\n\n")
    for v, r in rows:
        f.write(f"- `{v}`: `{os.path.join(out_dir, v + '.json')}`\n")
PY

echo "==> wrote consolidated markdown: ${BENCH_REPORT_MD}"
