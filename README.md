# cyclops

Cyclops is a Go mutation-testing wrapper that focuses
[Gremlins](https://gremlins.dev/) on selected files, glob patterns, or Git
changes.

Cyclops accepts files to include and translates that selection into the
exclusion rules required by Gremlins.

## Requirements

- Go 1.23 or later
- Git, when using `--diff`
- Gremlins available in `PATH`

Install Gremlins:

```sh
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
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

Run Cyclops from a Go module. With no arguments, it performs a full Gremlins
mutation test:

```sh
cd /path/to/go-module
cyclops
```

### Select files

Pass one or more Go source files to mutate only those files:

```sh
cyclops internal/service/user.go
cyclops internal/service/user.go internal/service/order.go
```

All selected files must belong to the same Go module. Duplicate paths are
ignored. A directly specified `*_test.go` file is rejected.

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

Show the final source-file selection without starting Gremlins:

```sh
cyclops --list
cyclops --list 'foo/{bar,bas}/**/*.go'
```

Without file or pattern arguments, `--list` shows all Go source files below the
current directory that Cyclops considers for mutation.

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

### Help and version

```sh
cyclops --help
cyclops --version
```

## License

Cyclops is available under the [MIT License](LICENSE).
