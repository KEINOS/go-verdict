# E2E Test Package

This package tests the built `verdict` binary with YAML scenarios.

The tests use the `e2e` build tag, so they do not run during normal `go test ./...`.

Run through Makefile:

```sh
make e2e
```

Run directly:

```sh
make build data
VERDICT_BIN="$(pwd)/dist/verdict" \
VERDICT_E2E_SCENARIOS_DIR="$(pwd)/testdata/e2e-scenarios" \
go test -tags=e2e -race ./tools/e2e/...
```

File roles:

- `e2e_test.go`: main E2E test entry point.
- `schema_test.go`: YAML schema, decode, and validation.
- `scenario_test.go`: scenario file discovery and path setup.
- `runner_test.go`: subprocess execution and output checks.
- `schema_unit_test.go`: schema and validation self-tests.
- `runner_unit_test.go`: runner and stdin self-tests.

Scenario files live in `testdata/e2e-scenarios/`. Relative paths in scenario args and `stdin_file` are repository-root relative.
