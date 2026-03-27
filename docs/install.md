# Install and Quickstart

Install Pituitary from a release binary for normal use. Build from source only if you are contributing to Pituitary itself.

## Homebrew (macOS)

```sh
brew install dusk-network/tap/pituitary
```

## Linux / macOS (binary)

Download the latest release from [GitHub Releases](https://github.com/dusk-network/pituitary/releases) for your platform (`linux_amd64` or `macOS_arm64`), extract, and install:

```sh
tar xzf pituitary_*_*.tar.gz
sudo install pituitary /usr/local/bin/
```

If you prefer a user-level install: `install pituitary ~/.local/bin/` (make sure `~/.local/bin` is in your PATH).

## Windows

Download `pituitary_<version>_windows_amd64.zip` from [GitHub Releases](https://github.com/dusk-network/pituitary/releases), extract `pituitary.exe`, and add its location to your PATH.

## Manual Releases

Prebuilt archives are published on [GitHub Releases](https://github.com/dusk-network/pituitary/releases) for:

- `pituitary_<version>_linux_amd64.tar.gz`
- `pituitary_<version>_macOS_arm64.tar.gz`
- `pituitary_<version>_windows_amd64.zip`

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

**Prerequisites:** Go 1.25.8+, a C toolchain (required for the sqlite-vec extension). For platform-specific setup, see [prerequisites.md](development/prerequisites.md).

```sh
git clone https://github.com/dusk-network/pituitary.git
cd pituitary
go build -o pituitary .

./pituitary index --rebuild
./pituitary review-spec --path specs/rate-limit-v2
./pituitary check-doc-drift --scope all
```

The repo ships with a small example workspace under `specs/` and curated fixture docs under `docs/guides/` and `docs/runbooks/`.
