# Skill Package Evaluation

This directory contains the evaluation framework used to validate and optimize the `SKILL.md` instruction set before publication.

## Methodology

The evaluation follows the "prompt as artifact under test" approach:

1. **Define representative tasks** that cover the full range of Pituitary agent workflows.
2. **Define a scoring rubric** that checks whether the agent follows the intended operating defaults.
3. **Run each task through the SKILL.md instructions** and score the expected agent behavior.
4. **Iterate** on instruction wording until scores stabilize.

This is the same general approach described in Karpathy's autoresearch methodology: treat the skill/prompt as the artifact under test, score outputs against a rubric, iterate until stable.

## Files

- `test-suite.json` — 10 representative tasks with expected agent behavior
- `rubric.json` — scoring dimensions with pass/fail criteria
- `scores.json` — evaluation results for the current SKILL.md version

## Running the evaluation

The evaluation is designed to be run manually or by an LLM judge. Each test case describes a user intent, and the rubric defines what correct agent behavior looks like. An evaluator (human or LLM) reads the SKILL.md, simulates the agent's response to each test case, and scores against the rubric.

For automated evaluation with an LLM judge:

1. Load `SKILL.md` as the system prompt.
2. For each task in `test-suite.json`, present the `user_intent` as user input.
3. Score the LLM's response against `rubric.json`.
4. Record pass/fail for each dimension.

## Score history

| Version | Date | Overall | Notes |
|---------|------|---------|-------|
| v8 (autoskiller) | 2026-03-29 | 0.930 | Structural optimization pass |
| structural baseline | 2026-03-29 | 0.717 | Pre-optimization structural assessment |
| current (post-eval) | 2026-04-05 | 0.95 | All dimensions pass — see `scores.json` for per-dimension totals |

Per-dimension applicability and pass totals are maintained in `scores.json`. The full dimension set is defined in `rubric.json`.
