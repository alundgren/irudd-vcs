# irudd-vcs

irudd-vcs is a fork of [Gitea](https://github.com/go-gitea/gitea) slimmed down to a
backend-only VCS server. It keeps the Gitea HTTP API (`/api/v1`), git over HTTP and
Git LFS, and stores its data in SQLite. There is no web UI, install page, SSH server,
mail, Actions, package registry or background cron jobs.

The goal is a locally running VCS that works as a Gitea provider for
[t3code](https://github.com/pingdotgg/t3code) and other software that supports
Gitea as a backend.

## Building

See [docs/build-setup.md](docs/build-setup.md) for prerequisites
and [docs/development.md](docs/development.md) for the development workflow, linting, and testing.

To build from source or make a distribution package, see [docs/build-source.md](docs/build-source.md).

## Running

There is no install page. If `custom/conf/app.ini` doesn't exist, the first `gitea web`
creates a minimal one with `INSTALL_LOCK = true` and generated secrets, using SQLite under
`data/`. An existing config must set `INSTALL_LOCK = true` in its `[security]` section.
See [app.example.ini](custom/conf/app.example.ini) for the options that are still supported.

```bash
./gitea web
./gitea admin user create --admin --username <name> --email <email> --random-password
```

Run `./gitea help` to see all available commands.

The API is documented by the generated specs served at `/swagger.v1.json` (Swagger 2.0)
and `/openapi3.v1.json` (OpenAPI 3.0) when `[api] ENABLE_SWAGGER` is enabled.

## Contributing

Read the [contributors guide](CONTRIBUTING.md) before starting to work on a change.

## License

This project is licensed under the MIT License.
See the [LICENSE](LICENSE) file for the full license text.
