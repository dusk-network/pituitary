#!/usr/bin/env bash
#
# #358 next-iteration retrieval precision benchmark: doc-level AND
# chunk-level metrics across three chunking configs.
#
# Required env:
#   BENCH_BASE_CONFIG   path to workspace config toml (MUST NOT include
#                       a [runtime.chunking] block — overlays supply it).
#
# Optional env:
#   BENCH_CASES         path to labeled cases JSON
#                       (default: testdata/retrieval-bench/next-iteration-cases.json)
#   BENCH_CORPUS_PINS   path to corpus pin TOML ([[corpus.repo]] entries)
#                       (default: testdata/retrieval-bench/next-iteration-corpus.toml)
#                       Runner refuses to run unless every pinned repo's
#                       HEAD matches its pin.
#   BENCH_OUT_DIR       per-variant JSON report dir
#                       (default: /tmp/pituitary-bench-358)
#   BENCH_REPORT_MD     consolidated markdown output path
#                       (default: docs/development/retrieval-precision-358.md)
#   BENCH_SKIP_PINS     if "1", skip corpus pin enforcement (NEVER use for
#                       published runs; intended for iteration-only).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/bench-precision.sh
source "${REPO_ROOT}/scripts/lib/bench-precision.sh"

if [[ -z "${BENCH_BASE_CONFIG:-}" ]]; then
  echo "bench-precision-358: BENCH_BASE_CONFIG is required" >&2
  exit 2
fi
if [[ ! -f "${BENCH_BASE_CONFIG}" ]]; then
  echo "bench-precision-358: base config not found: ${BENCH_BASE_CONFIG}" >&2
  exit 2
fi

CASES_DEFAULT="${REPO_ROOT}/testdata/retrieval-bench/next-iteration-cases.json"
PINS_DEFAULT="${REPO_ROOT}/testdata/retrieval-bench/next-iteration-corpus.toml"
OVERLAY_DIR="${REPO_ROOT}/testdata/retrieval-bench/chunking-overlays"
BENCH_CASES="${BENCH_CASES:-${CASES_DEFAULT}}"
BENCH_CORPUS_PINS="${BENCH_CORPUS_PINS:-${PINS_DEFAULT}}"
BENCH_OUT_DIR="${BENCH_OUT_DIR:-/tmp/pituitary-bench-358}"
BENCH_REPORT_MD="${BENCH_REPORT_MD:-${REPO_ROOT}/docs/development/retrieval-precision-358.md}"

if [[ "${BENCH_SKIP_PINS:-0}" != "1" ]]; then
  if [[ ! -f "${BENCH_CORPUS_PINS}" ]]; then
    echo "bench-precision-358: corpus pins file not found: ${BENCH_CORPUS_PINS}" >&2
    echo "  set BENCH_SKIP_PINS=1 to skip enforcement (iteration-only)." >&2
    exit 2
  fi
  bench_enforce_corpus_pins "${BENCH_CORPUS_PINS}"
fi

mkdir -p "${BENCH_OUT_DIR}"

BASE_INDEX_PATH="$(bench_read_workspace_index_path "${BENCH_BASE_CONFIG}")"
if [[ -z "${BASE_INDEX_PATH}" ]]; then
  echo "bench-precision-358: could not read [workspace] index_path from ${BENCH_BASE_CONFIG}" >&2
  exit 2
fi
BASE_INDEX_EXT="${BASE_INDEX_PATH##*.}"
BASE_INDEX_STEM="${BASE_INDEX_PATH%.*}"

VARIANTS=(pre338 p338 p344)

run_variant() {
  local variant="$1"
  local overlay="${OVERLAY_DIR}/${variant}.toml"
  local effective_cfg="${BENCH_OUT_DIR}/${variant}.toml"
  local variant_index="${BASE_INDEX_STEM}.${variant}.${BASE_INDEX_EXT}"
  local report="${BENCH_OUT_DIR}/${variant}.json"

  bench_rewrite_workspace_index_path "${BENCH_BASE_CONFIG}" "${effective_cfg}" "${variant_index}"
  if [[ -s "${overlay}" ]]; then
    printf '\n' >> "${effective_cfg}"
    cat "${overlay}" >> "${effective_cfg}"
  fi

  echo "==> ${variant}: rebuilding + benching (cfg=${effective_cfg}, db=${variant_index})"
  PITUITARY_PRECISION_CONFIG="${effective_cfg}" \
    PITUITARY_PRECISION_CASES="${BENCH_CASES}" \
    PITUITARY_PRECISION_REPORT="${report}" \
    PITUITARY_PRECISION_LABEL="${variant}" \
    go test -tags=precision_bench -run TestRetrievalPrecisionBench -count=1 \
      -timeout=60m -v ./internal/index/

  # Fold the per-variant snapshot size into the JSON report.
  local size_bytes
  size_bytes="$(bench_file_size_bytes "${variant_index}")"
  python3 -c "
import json, sys
path, size = sys.argv[1], int(sys.argv[2])
with open(path) as f:
    r = json.load(f)
r['snapshot_size_bytes'] = size
with open(path, 'w') as f:
    json.dump(r, f, indent=2)
" "${report}" "${size_bytes}"
}

for v in "${VARIANTS[@]}"; do
  run_variant "${v}"
done

# Render consolidated markdown with both doc-level and chunk-level tables.
python3 - "${BENCH_REPORT_MD}" "${BENCH_OUT_DIR}" "${VARIANTS[@]}" <<'PY'
import json, os, sys, datetime

out_path = sys.argv[1]
out_dir = sys.argv[2]
variants = sys.argv[3:]

rows = []
for v in variants:
    with open(os.path.join(out_dir, f"{v}.json")) as f:
        rows.append((v, json.load(f)))

os.makedirs(os.path.dirname(out_path), exist_ok=True)
with open(out_path, "w") as f:
    f.write("# Retrieval precision benchmark — #358 (chunk-level)\n\n")
    f.write(f"Generated: {datetime.datetime.utcnow().isoformat()}Z\n\n")
    f.write(f"Cases file: `{rows[0][1]['cases_path']}`\n\n")
    f.write(f"Case count: {rows[0][1]['case_count']} "
            f"(chunk-eligible: {rows[0][1].get('chunk_case_count', 0)})\n\n")

    f.write("## Doc-level (for continuity with #344)\n\n")
    f.write("| variant | p@5   | p@10  | recall@10 | MRR   | snapshot bytes |\n")
    f.write("|---------|-------|-------|-----------|-------|----------------|\n")
    for v, r in rows:
        f.write(
            f"| {v} "
            f"| {r['mean_precision_at_5']:.3f} "
            f"| {r['mean_precision_at_10']:.3f} "
            f"| {r['mean_recall_at_10']:.3f} "
            f"| {r['mean_reciprocal_rank']:.3f} "
            f"| {r.get('snapshot_size_bytes', 0)} |\n"
        )

    f.write("\n## Chunk-level (#358 Arm A)\n\n")
    f.write("| variant | p@5   | p@10  | recall@10 | MRR   |\n")
    f.write("|---------|-------|-------|-----------|-------|\n")
    for v, r in rows:
        if r.get("chunk_case_count", 0) == 0:
            f.write(f"| {v} | — | — | — | — |\n")
            continue
        f.write(
            f"| {v} "
            f"| {r.get('mean_chunk_precision_at_5', 0):.3f} "
            f"| {r.get('mean_chunk_precision_at_10', 0):.3f} "
            f"| {r.get('mean_chunk_recall_at_10', 0):.3f} "
            f"| {r.get('mean_chunk_reciprocal_rank', 0):.3f} |\n"
        )

    f.write("\n## Arm B (parent-inclusion, LLM-graded RAG)\n\n")
    f.write("_Reserved — closes on the upstream ExpandContext issue; not measured here._\n")
PY

echo "==> wrote consolidated markdown: ${BENCH_REPORT_MD}"
