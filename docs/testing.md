# Testing

There are three kinds of automated tests: backend unit tests, integration tests,
and migration tests. All of them use SQLite, so no extra services are required.

For prerequisites see [build-setup.md](build-setup.md); for the build workflow see
[development.md](development.md).

## Unit tests

Backend unit tests live in `*_test.go` files next to the code they cover. Set
`GITEA_TEST_LOG_SQL=1` to log all SQL statements executed during the tests.

```bash
make test-backend
```

To run a single backend test, use `go test` directly or the `#` selector:

```bash
go test -run '^TestName$' ./modulepath/
make test-backend#TestName
```

## Integration tests

Integration tests exercise the server against a real SQLite database. They live in
`tests/integration/` and require [Git LFS](https://git-lfs.com/) to be installed:

```bash
make test-integration
```

This runs each top-level test in its own process, three for every four CPUs at once; set
`TEST_PARALLEL` to change that. Each process is a `GITEA_TEST_INSTANCE` with its own work path,
database and port (3003 plus the instance), and the slowest tests of the last run start first.
The repository hooks run the test binary itself as gitea, so no separately built gitea is needed.

Run a single integration test with the `#` selector:

```bash
make test-integration#TestName
```

If you hit errors such as a mismatched database version, try a clean rebuild first:

```bash
make clean build
```

## Migration tests

If you change a database-persisted struct under `models/` you will usually need a
new migration in `models/migrations/`. Run the migration tests with:

```bash
make test-migration
```

## Writing tests

Please submit changes with additional unit and integration tests as appropriate.
Prefer unit tests when the logic can be tested in isolation, and keep integration
tests fast (aim for sub-2s runtime).
