# markfmt

markfmt is a fast, zero-config formatter that rewrites Markdown into one canonical style.

## Install

```sh
go install github.com/strangemattersystems/markfmt/cmd/markfmt@latest
```

## Usage

```sh
markfmt [-check] [path ...]
```

markfmt rewrites each path in place. With no path, or the path `-`, it reads standard input and writes standard output.

With `-check`, markfmt rewrites nothing. It prints each input that is not formatted.

| Exit status | Meaning |
| --- | --- |
| 0 | All inputs are formatted. |
| 1 | With `-check`, one or more inputs are not formatted. |
| 2 | markfmt cannot read, format or write an input. |

## Container

```sh
docker run --rm -v "$PWD:/work" -w /work --user "$(id -u):$(id -g)" ghcr.io/strangemattersystems/markfmt README.md
```

The `--user` flag keeps the ownership of the files that markfmt rewrites.

## License

[MIT](LICENSE)
