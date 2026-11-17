# Prepare build environment

Complete the steps in [build-setup.md](build-setup.md) to prepare your environment for building from source.

# Build

Various [make tasks](../Makefile) are provided to keep the build process as simple as possible.

Depending on requirements, the following build tags can be included.

- `bindata`: Build a single monolithic binary with the `options/` assets (gitignore, license, label and readme templates) embedded. Recommended for distribution.

To include all assets, use the `bindata` tag:

```bash
TAGS="bindata" make build
```

## Changing default paths

The server searches for a number of things from the _`CustomPath`_.
By default, this is the `custom/` directory in the current working directory when running the server.
It will also look for its configuration file _`CustomConf`_ in `$(CustomPath)/conf/app.ini`,
and will use the current working directory as the relative base path _`AppWorkPath`_.

These values, although useful when developing, may conflict with downstream users preferences.

For packagers who need to use paths like `/etc/gitea/app.ini`,
they should define these values at build time for `make build` by environment variable like
`LDFLAGS='-X "module.Var1=Value1" -X "module.Var2=Value2"' TAGS="bindata" make build`.

- _`CustomConf`_: `-X "gitea.dev/modules/setting.CustomConf=/etc/gitea/app.ini"`
- _`AppWorkPath`_: `-X "gitea.dev/modules/setting.AppWorkPath=/var/lib/gitea"`
- _`CustomPath`_: `-X "gitea.dev/modules/setting.CustomPath=/var/lib/gitea/custom"`
- Default PID file location: `-X "gitea.dev/cmd.PIDFile=/run/gitea.pid"`

Add as many of the strings with their preceding `-X` to the `LDFLAGS` variable and run `make build`
with the appropriate `TAGS` as above.

Running `gitea help` will allow you to review what the computed settings will be for your `gitea`.

## Cross Build

The build needs no cgo (SQLite is the pure-Go `modernc.org/sqlite`), so Go cross-builds every target itself.
`make release-binaries` builds the `RELEASE_TARGETS`.

For example, to cross build for Linux ARM64:

```
GOOS=linux GOARCH=arm64 TAGS="bindata" make build
```

### Adding shell autocompletion

Shell completion can be generated directly from binary with:
```sh
gitea completion <shell>
```

Supported values for `<shell>` are `bash`, `fish`, `pwsh` and `zsh`.
Details on how to load the completion for your shell can be found in the completion command help.
