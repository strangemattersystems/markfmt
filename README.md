# markfmt

A fast, zero-config formatter that rewrites Markdown into one canonical style.

## Install

```sh
brew install strangemattersystems/tap/markfmt
```

Or with Go:

```sh
go install github.com/strangemattersystems/markfmt/cmd/markfmt@latest
```

Binaries for macOS, Linux and Windows are on the [releases page](https://github.com/strangemattersystems/markfmt/releases).

## Usage

Format files or directories in place:

```sh
markfmt README.md docs/
```

Check formatting in CI. markfmt lists each unformatted file and exits with status 1:

```sh
markfmt --check .
```

Directories skip hidden files and directories. Skip more by name, glob or path:

```sh
markfmt --exclude testdata --exclude 'CHANGELOG*' .
```

With no path, markfmt reads standard input and writes standard output.

## Docker

```sh
docker run --rm -v "$PWD:/work" -w /work --user "$(id -u):$(id -g)" ghcr.io/strangemattersystems/markfmt .
```
