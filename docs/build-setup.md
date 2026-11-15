# Setup and requirements

This document lists the tools you need to build irudd-vcs from source. Once your
environment is ready, see [development.md](development.md) for the build and
development workflow, and [testing.md](testing.md) for running tests.

For the contribution workflow and review process, see [CONTRIBUTING.md](../CONTRIBUTING.md).

## Requirements

### Go

[Install Go](https://go.dev/doc/install) and set up your Go environment. The
required version is the one declared in [`go.mod`](../go.mod); using exactly that
version avoids `gofmt` differences between Go releases.

> [!NOTE]
> Some `make` tasks build external Go tools on demand (for example `make
> watch-backend`). To use them, the `"$GOPATH"/bin` directory must be on your
> executable `PATH`; otherwise you have to manage those tools yourself.

No Node.js or Python toolchain is needed: every build, lint and code generation
tool is a Go program run with `go run`.

### Make

The repository uses [GNU Make](https://www.gnu.org/software/make/) to drive builds,
linting, and tests. On Windows it can be installed via [MSYS2](https://www.msys2.org/) or
[Chocolatey](https://chocolatey.org/packages/make). Without Make, read the recipe of a
target in the [`Makefile`](../Makefile) and run its commands directly.

### Git LFS

The integration tests require [Git LFS](https://git-lfs.com/) to be installed.

### Docker or Podman (optional)

`make lint-shell` runs ShellCheck in a container unless a matching `shellcheck`
binary is installed.

## Installing dependencies

Most build and test targets install the dependencies they need on their own. To
fetch everything up front, run `make deps` (or the per-group `make deps-backend` and
`make deps-tools`).
