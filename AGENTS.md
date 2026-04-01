# Pulumi YAML

YAML and JSON language provider for Pulumi. Implements a language host gRPC server
(`pulumi-language-yaml`) and a converter (`pulumi-converter-yaml`) that translates
YAML/JSON programs into PCL for ejection to other Pulumi languages.

## Start here

| Path                         | What it is                                                              |
|------------------------------|-------------------------------------------------------------------------|
| `pkg/pulumiyaml/run.go`      | Core runtime — evaluates templates, registers resources with the engine |
| `pkg/pulumiyaml/analyser.go` | Schema-aware type analysis of YAML templates                            |
| `pkg/pulumiyaml/ast/`        | AST types for parsed YAML programs                                      |
| `pkg/pulumiyaml/codegen/`    | Code generation - YAML<->PCL conversion                                 |
| `pkg/server/server.go`       | Language host gRPC server implementation                                |
| `pkg/converter/`             | Converter gRPC server implementation                                    |

## Repository Structure

| Directory                    | Contents                                                                       |
|------------------------------|--------------------------------------------------------------------------------|
| `cmd/pulumi-language-yaml/`  | Language host binary entry point                                               |
| `cmd/pulumi-converter-yaml/` | Converter binary entry point                                                   |
| `pkg/pulumiyaml/`            | Core library: AST, syntax, codegen, config, diags, packages                    |
| `pkg/server/`                | Language host gRPC server                                                      |
| `pkg/converter/`             | Converter gRPC server                                                          |
| `pkg/tests/`                 | Integration tests with testdata and transpiled examples                        |
| `examples/`                  | 18 example projects (AWS, Azure, EKS, Kubernetes, etc.) (also run as examples) |
| `scripts/`                   | Schema fetching, coverage, and plugin doc utilities                            |

## Command canon

```
make build                    # Build both binaries to bin/
make lint                     # golangci-lint + copyright check
make test                     # Full test suite (builds first, fetches plugins+schemas)
make test_short               # Fast: skips integration tests
make test_live                # Requires AWS credentials (PULUMI_LIVE_TEST=true)
make get_schemas              # Download provider schemas for tests
make get_plugins              # Install Pulumi provider plugins for tests
make lint-golang              # Lint only the go code
```

## Code Conventions

### Forbidden Patterns

- **Do not edit schema JSON files** in `pkg/pulumiyaml/testing/test/testdata/`. They are downloaded by `scripts/get_schemas.sh` and gitignored (partially). Regenerate with `make get_schemas`.
- **Do not edit files in `pkg/tests/transpiled_examples/`** by hand. They are golden files updated with `PULUMI_ACCEPT=true go test --run=TestGenerateExamples ./...`.
- **Do not use root-level `configuration`** — it is deprecated. Use `config` instead.
- **Copyright headers are required** on all Go files except generated examples. CI checks via `pulumictl copyright`.
- **Do not expose Go types in user-facing errors.** Show user-meaningful types (e.g., "got boolean"), not Go types (e.g., `*ast.BooleanExpr`). Reviewers consistently reject this.
- **Do not use `path.Dir`/`path.Join` for filesystem paths.** Use `filepath.Dir`/`filepath.Join` — `path` is for URL paths and breaks on Windows.
- **Do not maintain manual skip lists for tests.** Use programmatic checks (e.g., `if strings.HasPrefix(name, "provider-") { skip }`) instead of listing test names. Manual lists rot.

### Review Expectations

These patterns are consistently flagged in PR review:

- **Error messages must be precise and non-redundant.** Consolidate duplicate diagnostics into summary/detail. Always return diagnostics — don't swallow them.
- **Naming must match pulumi/pulumi and PCL.** Field names, resource option names, and codegen output must align with the core SDK. Check PCL equivalents before naming new fields.
- **Tests must exercise behavior, not just declare it.** A config test must use the config value, not just declare it exists. Reviewers catch tests that don't actually assert anything meaningful.
- **Changelog entries need precise wording.** Describe what changed from the user's perspective, not internal implementation details.
- **Prefer simple APIs.** Avoid unnecessary boolean parameters, intermediate checks, or complex type hierarchies when a simpler design works. Flip if-guards for fail-fast readability.
- **Dependency upgrades propagate downstream.** Go version bumps and pulumi SDK upgrades affect consumers like docsgen and terraform-bridge. Consider downstream impact.

### Testing

- Use `testify/assert` and `testify/require`. Prefer `assert.Equal` on whole structs over per-field assertions.
- Golden file tests use `PULUMI_ACCEPT=true` to update expected output.
- `autogold` is used in a few test files for snapshot testing.
- Tests run with `-race` and `-parallel 10` by default.

### Changelog

Changes require a changelog entry via `changie new`. Components: `codegen`, `docs`, `runtime`, `convert`. Kinds: `Improvements` (minor), `Bug Fixes` (patch).

## Architecture

### Token Resolution

A central concept: user-written resource type strings (e.g., `aws:s3:Bucket`) are resolved
to canonical schema tokens (e.g., `aws:s3/bucket:Bucket`). The resolution algorithm is
documented in `pkg/pulumiyaml/README.md`. Five-step process involving exact match, module
alias lookup, camelCase conversion, and provider fallback.

### Expression Evaluation

YAML templates support interpolation (`${resource.output}`), built-in functions
(`fn::invoke`, `fn::join`, `fn::select`, `fn::toJSON`, etc.), and secret marking
(`fn::secret`). Expressions are parsed into AST nodes (`pkg/pulumiyaml/ast/expr.go`)
and evaluated during runtime in `run.go`.

### Two Binaries, One Core

Both `pulumi-language-yaml` (runs YAML programs) and `pulumi-converter-yaml` (translates
YAML to other languages) share `pkg/pulumiyaml/`. The language host calls `run.go`;
the converter calls `codegen/`.

### Package/Plugin System

`pkg/pulumiyaml/packages.go` defines the `Package` interface for interacting with
provider schemas. `PackageLoader` resolves resource types and functions against schemas.
Test schemas live in `pkg/pulumiyaml/testing/test/testdata/` as large JSON files.

## Escalate immediately if

- Changing the gRPC protocol interface (affects all Pulumi language hosts)
- Modifying token resolution logic (breaks existing YAML programs silently)
- Adding or removing built-in functions (`fn::*`) — public API surface
- Touching `pkg/pulumiyaml/testing/test/testdata/*.json` schemas by hand
- Tests fail with schema loading errors after a dependency update (likely need `make get_schemas`)

## If you change...

| What changed                                | Run                                                                                                                 |
|---------------------------------------------|---------------------------------------------------------------------------------------------------------------------|
| Any Go code                                 | `make lint && make test_short`                                                                                      |
| `pkg/pulumiyaml/codegen/`                   | `PULUMI_ACCEPT=true go test --run=TestGenerateExamples ./...` then review diffs in `pkg/tests/transpiled_examples/` |
| `go.mod`                                    | `go mod tidy && make lint`                                                                                          |
| Provider plugin versions in `Makefile`      | `make get_plugins`                                                                                                  |
| Schema versions in `scripts/get_schemas.sh` | `make get_schemas`                                                                                                  |
| `examples/`                                 | `make test_live` (requires AWS creds)                                                                               |
| Changelog                                   | `changie new` to create fragment, `changie batch auto && changie merge` for release                                 |
