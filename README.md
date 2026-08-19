# cyclops

Cyclops is a mutation-testing wrapper that focuses supported mutation engines
on selected files, glob patterns, or Git changes.

Cyclops accepts files to include and translates that selection into the native
filters supported by the selected backend.

## Backends

Cyclops selects a backend from project files in or above the current directory:

- `go.mod`: [Gremlins](https://gremlins.dev/)
- `Cargo.toml`: [cargo-mutants](https://mutants.rs/)

A directory tree containing both markers is ambiguous and rejected. When no
recognized marker exists, Cyclops preserves its original behavior and uses
Gremlins.

For Rust projects, Cyclops maps target files to repeated cargo-mutants `--file`
filters. `--dry-run` becomes `--list --json`, `--list` becomes `--list-files`,
and `--diff REF` generates a temporary Git diff for `--in-diff`. Native
`.cargo/mutants.toml` configuration continues to apply.

## Requirements

- Go 1.23 or later
- Git, when using `--diff`
- Gremlins available in `PATH` for Go projects, or cargo-mutants for Rust projects

Install Gremlins:

```sh
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
```

Install cargo-mutants:

```sh
cargo install --locked cargo-mutants
```

## Installation

Install the latest version:

```sh
go install github.com/t0k0sh1/cyclops/cmd/cyclops@latest
```

To install from a local checkout instead:

```sh
cd /path/to/cyclops
go install ./cmd/cyclops
```

Ensure Go's binary directory is in `PATH` if the command cannot be found:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

Verify the installation:

```sh
cyclops --version
```

## Usage

Run Cyclops from a Go module or Cargo project. With no arguments, it performs a
full mutation test with the detected backend:

```sh
cd /path/to/project
cyclops
```

### Select files

Pass one or more Go or Rust source files to mutate only those files:

```sh
cyclops internal/service/user.go
cyclops internal/service/user.go internal/service/order.go
cyclops src/lib.rs
```

Go targets must belong to the same Go module; directly specified `*_test.go`
files are rejected. Rust targets must be inside the detected Cargo project.

### Select files with patterns

Quote patterns so Cyclops expands them consistently instead of the shell:

```sh
cyclops 'internal/**/*.go'
cyclops 'foo/{bar,bas}/*.go'
cyclops 'internal/**/service_?.go'
cyclops 'pkg/[a-z]*.go'
```

Patterns support `*`, `**`, `?`, character classes such as `[a-z]`, and
alternatives such as `{bar,bas}`. Test files matched by a pattern are ignored.
A pattern that matches no Go source files is an error.

### List selected files

Show the final source-file selection without starting mutation tests:

```sh
cyclops --list
cyclops --list 'foo/{bar,bas}/**/*.go'
```

Without file or pattern arguments, `--list` shows the source files the selected
backend considers for mutation.

### Preview mutants

Analyze mutant candidates without running tests against each mutant:

```sh
cyclops --dry-run
cyclops --dry-run 'internal/**/*.go'
```

`--dry-run` cannot be combined with `--list`.

### Test Git changes

Limit mutation testing to lines changed from a branch or commit:

```sh
cyclops --diff origin/main
cyclops --dry-run --diff HEAD~1
cyclops --diff origin/main 'internal/**/*.go'
```

When files or patterns are also provided, Cyclops applies both filters.
`--diff` cannot be combined with `--list`.

Gremlins 0.6.0 treats an empty Git diff as an unfiltered run. Check that the
requested diff is non-empty before running Cyclops if an unexpected full run
would be costly.

For cargo-mutants, Cyclops generates `git diff REF` and passes its temporary
file to `--in-diff`. cargo-mutants exit code 2 means surviving mutants were
found; it is preserved and is not treated as an internal execution error.

### Help and version

```sh
cyclops --help
cyclops --version
```

## License

Cyclops is available under the [MIT License](LICENSE).
