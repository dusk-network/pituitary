# Development Prerequisites

Pituitary currently requires:

- Go 1.25+
- `make`
- a CGO-capable C toolchain for the `sqlite-vec` path

## macOS

```sh
xcode-select --install
```

If you prefer Homebrew-managed toolchains, ensure `clang` is available in your shell.

## Ubuntu / Debian

```sh
sudo apt-get update
sudo apt-get install -y build-essential make
```

## Fedora

```sh
sudo dnf install -y gcc make
```

## Verify The Toolchain

```sh
git clone https://github.com/dusk-network/pituitary.git
cd pituitary
make smoke-sqlite-vec
make ci
```

If `make smoke-sqlite-vec` passes, the local CGO and SQLite setup is usable for development.
