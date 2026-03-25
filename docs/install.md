# Install and Quickstart

Install Pituitary from a release binary for normal use. Build from source only if you are contributing to Pituitary itself.

## Homebrew

```sh
export HOMEBREW_GITHUB_API_TOKEN="$(gh auth token)"
brew install dusk-network/tap/pituitary
```

## One-Line Installer

```sh
gh api repos/dusk-network/pituitary/contents/scripts/install.sh?ref=main \
  -H 'Accept: application/vnd.github.raw' \
  | sh
```

The installer downloads the latest released archive for the current platform through `gh release download`, verifies it against the published checksum manifest, and installs `pituitary` to `/usr/local/bin` when that path is writable or `~/.local/bin` otherwise.

You can also pin the release or install directory:

```sh
gh api repos/dusk-network/pituitary/contents/scripts/install.sh?ref=main \
  -H 'Accept: application/vnd.github.raw' \
  | PITUITARY_VERSION=v1.0.0-alpha PITUITARY_INSTALL_DIR="$HOME/.local/bin" sh
```

## Manual Releases

Prebuilt archives are published on [GitHub Releases](https://github.com/dusk-network/pituitary/releases) for:

- `linux/amd64`
- `darwin/arm64`
- `windows/amd64`

If you need a different platform or want full manual control, download and extract the matching archive from Releases directly.

## Evaluate on an Existing Repo

```sh
pituitary init --path .
pituitary status
pituitary check-doc-drift --scope all

# Optional pre-merge guardrail
git diff --cached | pituitary check-compliance --diff-file -
```

If your repo already has a config, skip `init` and go straight to `status`, `index --rebuild`, or the analysis commands.

## Build from Source

If you are contributing to Pituitary itself or want to try the bundled example workspace in this repo:

```sh
git clone https://github.com/dusk-network/pituitary.git
cd pituitary
go build -o pituitary .

./pituitary index --rebuild
./pituitary review-spec --path specs/rate-limit-v2
./pituitary analyze-impact --path specs/rate-limit-v2/body.md
./pituitary check-doc-drift --scope all
```

The repo ships with a small example workspace under `specs/` and curated fixture docs under `docs/guides/` and `docs/runbooks/`.
