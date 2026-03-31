# CI Recipes

Pituitary should run in CI as a consumer, not as a separate CI product. The CLI already exposes the right primitives; these recipes show how to compose them in a normal pipeline.

These examples assume your repo already has a committed config (`.pituitary/pituitary.toml`). If it does not, run `pituitary init --path .` locally first and commit the generated config before you wire CI around it.

## GitHub Action: comment `review-spec` output on spec pull requests

This action comments on pull requests that touch configured spec bundles. It uses `pituitary explain-file` to map changed files back to indexed `spec_bundle` sources, runs `pituitary review-spec` for each affected bundle, and updates one sticky PR comment with the markdown report.

```yaml
name: Pituitary Review Spec

on:
  pull_request:
    paths:
      - "specs/**"
      - ".pituitary/pituitary.toml"
      - "pituitary.toml"

jobs:
  review-spec:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read
      issues: write
    steps:
      - uses: dusk-network/pituitary@v1.0.0-beta.3
        with:
          fail-on: error
          # Set this when your repo keeps config at the root instead.
          # config-path: pituitary.toml
```

`fail-on` is an action-level severity threshold:

- `error`: fail only when a touched spec review finds deterministic doc-drift items
- `warning`: fail on `error` plus `review-spec` warnings or `possible_drift` assessments
- `none`: never fail; comment only

The action updates one comment per PR by default and deletes the stale comment automatically if the PR no longer touches indexed spec bundles.
By default it reads `.pituitary/pituitary.toml`; if your repo keeps `pituitary.toml` at the root, set `config-path: pituitary.toml`.

## GitHub Actions: diff compliance plus doc drift

This recipe installs the released Linux binary, rebuilds the local index, checks the PR diff against accepted specs, and then runs a workspace-wide doc-drift pass.

```yaml
name: Pituitary Spec Hygiene

on:
  pull_request:

jobs:
  pituitary:
    runs-on: ubuntu-latest
    env:
      PITUITARY_VERSION: v1.0.0-beta.3
    steps:
      - name: Check out repository
        uses: actions/checkout@v5
        with:
          fetch-depth: 0

      - name: Install Pituitary release binary
        run: |
          version_no_v="${PITUITARY_VERSION#v}"
          archive="pituitary_${version_no_v}_linux_amd64.tar.gz"
          curl -fsSL -o "/tmp/${archive}" "https://github.com/dusk-network/pituitary/releases/download/${PITUITARY_VERSION}/${archive}"
          tar -xzf "/tmp/${archive}" -C /tmp
          install -m 0755 /tmp/pituitary "$HOME/.local/bin/pituitary"
          echo "$HOME/.local/bin" >> "$GITHUB_PATH"

      - name: Rebuild index
        run: pituitary index --rebuild

      - name: Check changed code against accepted specs
        run: git diff origin/main...HEAD | pituitary check-compliance --diff-file -

      - name: Check doc drift from the PR diff
        run: git diff origin/main...HEAD | pituitary check-doc-drift --diff-file -

      - name: Check doc drift across the full workspace
        run: pituitary check-doc-drift --scope all
```

## Optional runtime preflight for real embeddings

If your checked-in config uses `runtime.embedder.provider = "openai_compatible"` or provider-backed analysis, add a preflight before the rebuild:

```yaml
      - name: Check runtime readiness
        run: pituitary status --check-runtime all
```

Keep that step out of deterministic fixture-only CI. In fixture mode, there is no live runtime dependency to validate.

## Notes

- Prefer the release binary in consumer CI. This repo's own `main` CI builds from source because it is testing Pituitary itself, not consuming it.
- Pin `PITUITARY_VERSION` to the exact release you want your repo to consume rather than floating on latest.
- `check-compliance --diff-file` is best for change-scoped policy.
- `check-doc-drift --diff-file` is best for change-scoped stale-doc detection on the files under review.
- `check-doc-drift --scope all` is still the broader workspace-wide hygiene sweep.
- If you only want a deterministic CI baseline, keep the default fixture embedder and skip runtime preflight entirely.
