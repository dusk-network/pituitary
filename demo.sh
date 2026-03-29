#!/bin/bash
set -e
export TERM=${TERM:-xterm-256color}

B=$'\033[1m'
D=$'\033[2m'
RST=$'\033[0m'
PROMPT=$'\033[0;32m'

type_cmd() {
    printf "\n"
    printf "${PROMPT}>${RST} "
    for (( i=0; i<${#1}; i++ )); do
        printf "${B}%s${RST}" "${1:$i:1}"
        sleep 0.02
    done
    printf "\n"
    sleep 0.3
}

run_cmd() {
    type_cmd "$1"
    eval "$1"
    sleep 2.5
}

clear
printf "\n  ${B}Pituitary${RST} ${D}— catch spec drift before it catches you${RST}\n"
sleep 1.5

# 1. check-doc-drift
run_cmd "pituitary check-doc-drift --scope all"

# 2. fix (dry-run so it doesn't modify files)
run_cmd "pituitary fix --scope all --dry-run"

# 3. check-overlap
run_cmd "pituitary check-overlap --spec-ref SPEC-042"

# 4. review-spec
run_cmd "pituitary review-spec --spec-ref SPEC-042"

# 5. status (just the summary line, not the full config dump)
type_cmd "pituitary status"
pituitary status 2>&1 | head -3
sleep 2

printf "\n  ${D}github.com/dusk-network/pituitary${RST}\n\n"
sleep 2
