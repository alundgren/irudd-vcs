# Development

This document describes how to build irudd-vcs from source and the day-to-day
development workflow. For prerequisites, see [build-setup.md](build-setup.md).
For running tests, see [testing.md](testing.md).

Area-specific guidelines:

- [Backend development guidelines](guidelines-backend.md)

## Building

To build the server for development, run:

```bash
make build
```

No build tags are required: SQLite support is compiled in by default, which is
enough for local development. The `bindata` tag embeds the `options/` assets into
the binary and is only needed when packaging a self-contained build, so leave it out
during development.

See `make help` for all available targets.

## Building continuously

To rebuild and restart the server automatically when source files change:

```bash
make watch-backend
```

Watching all backend source files may hit the default open-files limit on macOS or
Linux; raise it with `ulimit -n 12288` for the current shell, or in your shell
startup file to make it permanent.

## Formatting, linting and checks

Format your code first:

```bash
make fmt
```

Then lint:

```bash
# lint everything (Go, editorconfig, spelling)
make lint
# or only the Go code
make lint-go
```

Many linters can fix issues automatically with `make lint-fix` (or
`make lint-go-fix`). The combined consistency checks are available as
`make checks`.

## Updating the API

When you create or change API routes, you **must** update the
[Swagger](https://swagger.io/docs/specification/2-0/what-is-swagger/) documentation
using [go-swagger](https://goswagger.io/) comments. See the
[backend development guidelines](guidelines-backend.md) for how API routes,
request/response structs, and swagger definitions fit together.

Regenerate and validate the specs after changing an endpoint, then commit the
updated files in `modules/apispec/`:

```bash
make generate-swagger
make swagger-validate
```

`make generate-swagger` also regenerates the OpenAPI 3.0 spec
(`make generate-openapi3`). Verify the committed specs are up to date with:

```bash
make swagger-check openapi3-check
```

## Creating new configuration options

When adding configuration options it is not enough to add them to the
`modules/setting` files. Also update
[`custom/conf/app.example.ini`](../custom/conf/app.example.ini).

## Database migrations

If you make breaking changes to a database-persisted struct under `models/`, add a
new migration in `models/migrations/`. See [testing.md](testing.md#migration-tests)
for running the migration tests.

## Testing

For unit, integration, and migration tests, see [testing.md](testing.md).

## IDE configuration

### GoLand

Clicking the `Run Application` arrow on `func main()` in `/main.go` starts a
debuggable server instance.

The `Output Directory` in `Run/Debug Configuration` **must** be set to the project
directory (the one containing `main.go` and `go.mod`). Otherwise the working
directory is a GoLand temporary directory, which prevents the server from finding
`custom/conf/app.ini` and its data directories.
