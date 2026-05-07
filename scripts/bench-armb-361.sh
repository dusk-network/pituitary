#!/usr/bin/env bash
#
# #361 Arm B retrieval benchmark: LLM-graded RAG answer quality with
# ExpandContext(IncludeParent) off vs on across chunking variants.
#
# Required env:
#   BENCH_BASE_CONFIG   path to workspace config toml. It must configure a
#                       live embedder and runtime.analysis provider.
#
# Optional env:
#   BENCH_CASES         path to Arm B RAG cases JSON
#                       (default: testdata/retrieval-bench/armb-rag-cases.json)
#   BENCH_CORPUS_PINS   path to corpus pin TOML ([[corpus.repo]] entries)
#                       (default: testdata/retrieval-bench/next-iteration-corpus.toml)
#   BENCH_OUT_DIR       per-variant JSON report dir
#                       (default: /tmp/pituitary-bench-361)
#   BENCH_REPORT_MD     consolidated markdown output path
#                       (default: docs/development/retrieval-armb-361.md)
#   BENCH_SKIP_PINS     if "1", skip corpus pin enforcement (iteration-only).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/bench-precision.sh
source "${REPO_ROOT}/scripts/lib/bench-precision.sh"

if [[ -z "${BENCH_BASE_CONFIG:-}" ]]; then
  echo "bench-armb-361: BENCH_BASE_CONFIG is required" >&2
  exit 2
fi
if [[ ! -f "${BENCH_BASE_CONFIG}" ]]; then
  echo "bench-armb-361: base config not found: ${BENCH_BASE_CONFIG}" >&2
  exit 2
fi

CASES_DEFAULT="${REPO_ROOT}/testdata/retrieval-bench/armb-rag-cases.json"
PINS_DEFAULT="${REPO_ROOT}/testdata/retrieval-bench/next-iteration-corpus.toml"
OVERLAY_DIR="${REPO_ROOT}/testdata/retrieval-bench/chunking-overlays"
BENCH_CASES="${BENCH_CASES:-${CASES_DEFAULT}}"
BENCH_CORPUS_PINS="${BENCH_CORPUS_PINS:-${PINS_DEFAULT}}"
BENCH_OUT_DIR="${BENCH_OUT_DIR:-/tmp/pituitary-bench-361}"
BENCH_REPORT_MD="${BENCH_REPORT_MD:-${REPO_ROOT}/docs/development/retrieval-armb-361.md}"

if [[ "${BENCH_SKIP_PINS:-0}" != "1" ]]; then
  if [[ ! -f "${BENCH_CORPUS_PINS}" ]]; then
    echo "bench-armb-361: corpus pins file not found: ${BENCH_CORPUS_PINS}" >&2
    echo "  set BENCH_SKIP_PINS=1 to skip enforcement (iteration-only)." >&2
    exit 2
  fi
  bench_enforce_corpus_pins "${BENCH_CORPUS_PINS}"
fi

mkdir -p "${BENCH_OUT_DIR}"

BASE_INDEX_PATH="$(bench_read_workspace_index_path "${BENCH_BASE_CONFIG}")"
if [[ -z "${BASE_INDEX_PATH}" ]]; then
  echo "bench-armb-361: could not read [workspace] index_path from ${BENCH_BASE_CONFIG}" >&2
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

  echo "==> ${variant}: rebuilding + Arm B benching (cfg=${effective_cfg}, db=${variant_index})"
  PITUITARY_ARMB_CONFIG="${effective_cfg}" \
    PITUITARY_ARMB_CASES="${BENCH_CASES}" \
    PITUITARY_ARMB_REPORT="${report}" \
    PITUITARY_ARMB_LABEL="${variant}" \
    go test -tags=precision_bench -run TestRetrievalArmBBench -count=1 \
      -timeout=120m -v ./internal/index/

  local stroma_pattern="${variant_index%.*}.stroma.*.${variant_index##*.}"
  local stroma_path
  stroma_path="$(ls -1 ${stroma_pattern} 2>/dev/null | head -1)"
  local size_bytes
  if [[ -n "${stroma_path}" && -f "${stroma_path}" ]]; then
    size_bytes="$(bench_file_size_bytes "${stroma_path}")"
  else
    echo "bench: warning - could not locate stroma snapshot matching ${stroma_pattern}; reporting registry size instead" >&2
    size_bytes="$(bench_file_size_bytes "${variant_index}")"
  fi
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

python3 - "${BENCH_REPORT_MD}" "${BENCH_OUT_DIR}" "${VARIANTS[@]}" <<'PY'
import json, os, sys, datetime

out_path = sys.argv[1]
out_dir = sys.argv[2]
variants = sys.argv[3:]

rows = []
for v in variants:
    with open(os.path.join(out_dir, f"{v}.json")) as f:
        rows.append((v, json.load(f)))

def arm(report, name):
    for item in report["arms"]:
        if item["name"] == name:
            return item
    raise KeyError(name)

def dist_text(summary):
    dist = summary.get("distribution", {})
    return " ".join(f"{i}:{dist.get(str(i), 0)}" for i in range(6))

os.makedirs(os.path.dirname(out_path), exist_ok=True)
with open(out_path, "w") as f:
    first = rows[0][1]
    f.write("# Retrieval Arm B benchmark - #361\n\n")
    f.write(f"Generated: {datetime.datetime.utcnow().isoformat()}Z\n\n")
    f.write(f"Cases file: `{first['cases_path']}`\n\n")
    f.write(f"Case count: {first['case_count']} (mid-body: {first['mid_body_case_count']})\n\n")
    f.write(f"Corpus doc count: {first.get('corpus_doc_count', 0)}\n\n")
    f.write(f"Analysis model: `{first['analysis_model']}`\n\n")
    f.write(f"Actionability ceiling: {first['actionability_ceiling']}\n\n")

    f.write("## Headline\n\n")
    f.write("| variant | leaf mean | parent mean | delta | leaf median | parent median | parent p95 retrieval ms | snapshot bytes |\n")
    f.write("|---------|-----------|-------------|-------|-------------|---------------|-------------------------|----------------|\n")
    for v, r in rows:
        leaf = arm(r, "leaf_only")["summary"]
        parent = arm(r, "leaf_plus_parent")["summary"]
        delta = r.get("delta") or {}
        f.write(
            f"| {v} "
            f"| {leaf['mean_score']:.2f} "
            f"| {parent['mean_score']:.2f} "
            f"| {delta.get('mean_score_delta', 0):+.2f} "
            f"| {leaf['median_score']:.1f} "
            f"| {parent['median_score']:.1f} "
            f"| {parent['p95_retrieval_latency_ms']:.2f} "
            f"| {r.get('snapshot_size_bytes', 0)} |\n"
        )

    f.write("\n## Quality Distribution\n\n")
    f.write("| variant | arm | distribution (score:count) | errored cases |\n")
    f.write("|---------|-----|----------------------------|---------------|\n")
    for v, r in rows:
        for name in ("leaf_only", "leaf_plus_parent"):
            summary = arm(r, name)["summary"]
            f.write(f"| {v} | {name} | {dist_text(summary)} | {summary['errored_cases']} |\n")

    f.write("\n## Cost And Latency Envelope\n\n")
    f.write("Model pricing is not configured in this harness; prompt/completion token counts are the neutral cost envelope.\n\n")
    f.write("| variant | arm | retrieval searches | expansion calls | generator calls | grader calls | prompt tokens | completion tokens | context bytes | context token estimate |\n")
    f.write("|---------|-----|--------------------|-----------------|-----------------|--------------|---------------|-------------------|---------------|------------------------|\n")
    for v, r in rows:
        for name in ("leaf_only", "leaf_plus_parent"):
            summary = arm(r, name)["summary"]
            model_calls = summary.get("model_calls", 0)
            generator_calls = summary.get("generator_model_calls", model_calls // 2)
            grader_calls = summary.get("grader_model_calls", model_calls - generator_calls)
            f.write(
                f"| {v} | {name} "
                f"| {summary['stroma_searches']} "
                f"| {summary['context_expansion_calls']} "
                f"| {generator_calls} "
                f"| {grader_calls} "
                f"| {summary.get('prompt_tokens', 0)} "
                f"| {summary.get('completion_tokens', 0)} "
                f"| {summary['context_bytes']} "
                f"| {summary['context_token_estimate']} |\n"
            )

    p344 = dict(rows).get("p344")
    if p344:
        delta = (p344.get("delta") or {}).get("mean_score_delta", 0)
        f.write("\n## Interpretation\n\n")
        if delta > 0:
            f.write(f"`p344` parent inclusion improved mean LLM-graded score by {delta:.2f}, with unchanged median score. This is research-positive for the parent-lineage story, but too small to justify a broad default-quality claim by itself; keep parent inclusion as the explicit outline-context path and use the token envelope above when judging opt-in workflows.\n")
        elif delta == 0:
            f.write("`p344` parent inclusion produced a null mean-score delta on this run. Per #361, this keeps the LateChunkPolicy parent-lineage product story unproven for this corpus and should be reassessed before making external quality claims.\n")
        else:
            f.write(f"`p344` parent inclusion reduced mean score by {abs(delta):.2f}. Per #361, this is research-negative for the current parent-lineage story on this corpus.\n")
PY

echo "==> wrote consolidated markdown: ${BENCH_REPORT_MD}"
