---
title: "Installation"
description: "Install hf with Go, Homebrew, Scoop, a Linux package, a release archive, or the container image."
weight: 20
---

hf is a single pure-Go binary with nothing to run alongside it. Pick whichever
channel suits you.

## Go

```bash
go install github.com/tamnd/hf-cli/cmd/hf@latest
```

That puts `hf` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless you moved
it. Make sure that directory is on your `PATH`.

## Homebrew (macOS)

```bash
brew install --cask tamnd/tap/hf
```

The cask installs the prebuilt macOS binary. On Linux, use the packages below or
`go install`.

## Scoop (Windows)

```bash
scoop bucket add tamnd https://github.com/tamnd/scoop-bucket
scoop install hf
```

## Linux (apt and dnf)

A signed apt and dnf repository tracks every release, so `apt upgrade` and
`dnf upgrade` keep hf current.

```bash
# Debian, Ubuntu
curl -fsSL https://tamnd.github.io/linux-repo/gpg.key \
  | sudo gpg --dearmor -o /usr/share/keyrings/tamnd.gpg
echo "deb [signed-by=/usr/share/keyrings/tamnd.gpg] https://tamnd.github.io/linux-repo/apt stable main" \
  | sudo tee /etc/apt/sources.list.d/tamnd.list
sudo apt update && sudo apt install hf

# Fedora, RHEL
sudo dnf config-manager --add-repo https://tamnd.github.io/linux-repo/dnf/tamnd.repo
sudo dnf install hf
```

## Release archives and Linux packages

Every [release](https://github.com/tamnd/hf-cli/releases) attaches `tar.gz`
archives (and a `.zip` for Windows) for Linux, macOS, Windows, and FreeBSD on
amd64 and arm64, plus `.deb`, `.rpm`, and `.apk` packages and a `checksums.txt`
signed with keyless [cosign](https://docs.sigstore.dev/). Download the one for
your platform, extract `hf`, and put it on your `PATH`. To install a package
directly without the repository above:

```bash
# Debian, Ubuntu
sudo dpkg -i hf_*_amd64.deb

# Fedora, RHEL
sudo rpm -i hf-*.x86_64.rpm
```

## Container

```bash
docker run --rm ghcr.io/tamnd/hf:latest models --author google -n 5
```

The image is the same static binary on Alpine, running as a non-root user.

## From source

```bash
git clone https://github.com/tamnd/hf-cli
cd hf-cli
make build        # produces ./bin/hf
./bin/hf version
```

## Checking the install

```bash
hf version
```

prints the version, the commit it was built from, and exits.
