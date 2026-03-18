# Installation

This guide walks you through building Bridge from source.

---

## Step 1: Install Rust

If you don't have Rust installed, get it from [rustup.rs](https://rustup.rs):

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

Follow the prompts, then reload your shell:

```bash
source $HOME/.cargo/env
```

Verify the installation:

```bash
rustc --version
# Should show something like: rustc 1.75.0 (...
```

---

## Step 2: Clone the Repository

```bash
git clone https://github.com/useportal-app/bridge.git
cd bridge
```

---

## Step 3: Build Bridge

For development (faster compile, larger binary):

```bash
make build
```

For production (optimized, stripped):

```bash
make build-release
```

The binary will be at `target/release/bridge` (or `target/debug/bridge` for debug builds).

---

## Step 4: Verify the Build

Check that the binary works:

```bash
./target/release/bridge --help
```

You should see help output showing available commands and options.

---

## What's Next?

Now you have Bridge built. Next, you'll:

1. Set up your [configuration](configuration.md)
2. Run through the [quickstart](quickstart.md) to see Bridge in action

---

## Troubleshooting

### Build fails with "linker not found"

Install a C compiler:

```bash
# Ubuntu/Debian
sudo apt-get install build-essential

# macOS
xcode-select --install

# Fedora/RHEL
sudo dnf install gcc
```

### Build is slow

The first build compiles all dependencies. This is normal. Subsequent builds are much faster. Use `make build` (not `build-release`) during development.

### Out of memory during build

Rust compilation can be memory-intensive. If you hit OOM errors:

```bash
# Reduce parallel jobs
CARGO_BUILD_JOBS=1 make build-release
```
