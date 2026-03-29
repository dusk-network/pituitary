#!/bin/bash
export TERM=${TERM:-xterm-256color}

R=$'\033[1;31m'
G=$'\033[1;32m'
Y=$'\033[1;33m'
C=$'\033[0;36m'
B=$'\033[1m'
D=$'\033[2m'
RST=$'\033[0m'

clear

# ============================================================
# V2: LESS INFO, MORE ACTION
# ============================================================

printf "\n"
printf "  ${D}Design: less info per finding, actionable remediations${RST}\n"
printf "\n"
sleep 1

# --- check-doc-drift: compact ---
printf "${B}━━◈ check-doc-drift${RST}\n"
printf "\n"
printf "  ${C}docs/guides/api-rate-limits.md${RST}                        ${R}██ DRIFT${RST}\n"
printf "\n"
printf "    ${R}✗${RST} ${B}wrong window model${RST}           ${Y}expected${RST} sliding  ${Y}got${RST} fixed\n"
printf "    ${R}✗${RST} ${B}wrong default limit${RST}           ${Y}expected${RST} 200/min  ${Y}got${RST} 100/min\n"
printf "    ${R}✗${RST} ${B}tenant overrides unsupported${RST}  ${Y}expected${RST} yes      ${Y}got${RST} no\n"
printf "\n"
printf "    ${G}fix:${RST} pituitary fix --path docs/guides/api-rate-limits.md ${D}(3 edits)${RST}\n"
printf "    ${D}ℹ  run review-spec --format html for the full evidence report${RST}\n"
printf "\n"
printf "  ${C}docs/runbooks/rate-limit-rollout.md${RST}                   ${G}██ OK${RST}\n"
printf "\n"
sleep 3

# --- what `fix` would do ---
printf "${B}━━◈ fix${RST} ${D}--path docs/guides/api-rate-limits.md${RST}\n"
printf "\n"
printf "  ${D}docs/guides/api-rate-limits.md${RST}\n"
printf "\n"
printf "    ${R}- The public API uses a fixed-window rate limiter.${RST}\n"
printf "    ${G}+ The public API uses a sliding-window rate limiter.${RST}\n"
printf "\n"
printf "    ${R}- The default limit is 100 requests per minute for each API key.${RST}\n"
printf "    ${G}+ The default limit is 200 requests per minute per tenant.${RST}\n"
printf "\n"
printf "    ${R}- tenant-specific overrides are not supported.${RST}\n"
printf "    ${G}+ tenant-specific overrides are supported through configuration.${RST}\n"
printf "\n"
printf "  ${Y}apply these edits?${RST} ${B}[y/n/diff]${RST} "
printf "\n"
printf "\n"
sleep 3

# --- check-overlap: compact ---
printf "${B}━━◈ check-overlap${RST} · ${C}SPEC-042${RST}\n"
printf "\n"
printf "  ${R}██${RST} ${C}SPEC-008${RST}  ${B}.955${RST}  ${D}Legacy Rate Limiting${RST}        extends this spec\n"
printf "  ${R}██${RST} ${C}SPEC-055${RST}  ${B}.929${RST}  ${D}Burst Handling${RST}              adjacent scope\n"
printf "  ${Y}▒▒${RST} ${C}DOG-001${RST}   ${B}.692${RST}  ${D}Product Scope Contract${RST}      adjacent scope\n"
printf "  ${Y}▒▒${RST} ${C}DOG-002${RST}   ${B}.687${RST}  ${D}Contributor Workflow${RST}        adjacent scope\n"
printf "\n"
printf "  ${G}✓${RST} SPEC-042 already supersedes SPEC-008 — no action needed\n"
printf "\n"
sleep 3

# --- review-spec: full tree (v1 style) ---
printf "${B}━━◈ review-spec${RST} · ${C}SPEC-042${RST}\n"
printf "    ${D}Per-Tenant Rate Limiting for Public API Endpoints${RST}\n"
printf "\n"
printf "  ${B}OVERLAP${RST}   4 specs · recommendation: proceed with supersedes\n"
printf "  ${D}├─${RST} ${C}SPEC-008${RST}  0.955  extends\n"
printf "  ${D}├─${RST} ${C}SPEC-055${RST}  0.929  adjacent\n"
printf "  ${D}├─${RST} ${C}DOG-001${RST}   0.692  adjacent\n"
printf "  ${D}└─${RST} ${C}DOG-002${RST}   0.687  adjacent\n"
printf "\n"
printf "  ${B}IMPACT${RST}    2 specs · 2 refs · 2 docs\n"
printf "  ${D}├─${RST} ${C}SPEC-055${RST}  Burst Handling · depends_on\n"
printf "  ${D}├─${RST} ${C}SPEC-008${RST}  Legacy Rate Limiting · supersedes · ${D}historical${RST}\n"
printf "  ${D}├─${RST} ${C}doc://runbooks/rate-limit-rollout${RST}  0.956\n"
printf "  ${D}└─${RST} ${C}doc://guides/api-rate-limits${RST}       0.902\n"
printf "\n"
printf "  ${B}DOC DRIFT${RST} 1 item · 3 remediations\n"
printf "  ${D}└─${RST} ${C}doc://guides/api-rate-limits${RST}  ${R}██ DRIFT${RST}\n"
printf "     ${Y}→${RST} 3 suggested edits ${D}(see check-doc-drift for detail)${RST}\n"
printf "\n"
printf "  ${B}COMPARISON${RST}  prefer ${C}SPEC-042${RST} as the primary reference\n"
printf "\n"
printf "  ${D}ℹ  run review-spec --format html for the full evidence report${RST}\n"
printf "\n"
sleep 3

# --- status: minimal ---
printf "${B}━━◈ status${RST}\n"
printf "\n"
printf "  ${B}5${RST} specs  ${B}2${RST} docs  ${B}23${RST} chunks  ${G}fresh${RST}  ${D}fixture embedder${RST}\n"
printf "\n"
sleep 2

printf "${D}--- end of preview ---${RST}\n\n"
