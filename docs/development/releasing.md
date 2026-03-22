# Releasing Pituitary

Pituitary releases are tag-driven and produce prebuilt archives so users can evaluate the tool without building it from source.

## Automated Targets

The current release workflow publishes one archive per supported platform:

- `linux/amd64`
- `darwin/arm64`
- `windows/amd64`

Each archive is built from the checked-in [/.goreleaser.yaml](../../.goreleaser.yaml) configuration, and the workflow also uploads a combined SHA-256 checksum manifest for the tagged release.

## Release Workflow

1. Make sure the branch you want to release is already merged to `main`.
2. Pull the latest `main` locally.
3. Create and push an annotated SemVer tag such as `v0.2.0`.

```sh
git switch main
git pull --ff-only origin main
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```

Pushing the tag triggers [/.github/workflows/release.yml](../../.github/workflows/release.yml), which:

- runs GoReleaser packaging on Linux, macOS, and Windows runners
- injects the tag, commit, and build date into the binary metadata
- uploads the versioned archives to a GitHub release for that tag
- publishes `pituitary_<tag>_checksums.txt` alongside the archives

## Validation

Pull requests and pushes to `main` validate the checked-in GoReleaser configuration with `goreleaser check` in [/.github/workflows/ci.yml](../../.github/workflows/ci.yml).

That validation is intentionally lighter than a full tagged release: it checks the config shape without requiring a release tag or publishing artifacts.
