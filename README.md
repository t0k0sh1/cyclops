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

### Normalized results

Backend execution produces an internal normalized result containing backend
identity and version when reported, requested targets and diff base, run state,
summary counts, individual mutant locations, operators, and execution errors.
The common mutant statuses are `killed`, `survived`, `uncovered`, `skipped`,
`timed-out`, `unviable`, `runnable`, `error`, and `unknown`.

Gremlins results are read from its `--output` JSON file. cargo-mutants dry-run
candidates are read from `--list --json`, and complete runs are read from
`outcomes.json` in a temporary output directory. The original backend JSON is
retained alongside normalized fields so unknown or backend-specific data is not
discarded.

This model is currently internal: Cyclops continues to pass backend output
through unchanged and preserves existing exit behavior. A public JSON format
and the non-blocking CI reporter described in [#1](https://github.com/t0k0sh1/cyclops/issues/1)
are intentionally deferred until the internal schema has been exercised.

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

List the production files in packages whose tests changed from a reference:

```sh
cyclops --list-test-targets origin/main
```

This command does not run mutation testing. For Go, it finds changed
`*_test.go` files and uses `go list` in each affected directory to list the
package's buildable non-test Go files. For Rust, it finds changed integration
tests under a package's `tests/` directory and explicit `[[test]]` targets,
uses `cargo metadata` to identify the owning package, and lists that package's
`src/**/*.rs` files. Workspace packages are mapped independently. An empty
mapping prints nothing and never falls back to the whole repository.

For cargo-mutants, Cyclops generates `git diff REF` and passes its temporary
file to `--in-diff`. cargo-mutants exit code 2 means surviving mutants were
found; it is preserved and is not treated as an internal execution error.

## Non-blocking GitHub pull-request reports

This repository contains a working GitHub Actions integration:

- [`.github/workflows/cyclops-analyze.yml`](.github/workflows/cyclops-analyze.yml)
  analyzes each pull request and creates or updates one persistent comment with
  the Cyclops result for reviewers.

Copy the workflow to the same path in a Go or Rust repository. In the workflow,
replace the local Cyclops installation because Cyclops is not built by that
repository. Install the mutation engine selected by the repository marker:

```yaml
- name: Install Cyclops and mutation engine
  shell: bash
  run: |
    set -euo pipefail
    go install github.com/t0k0sh1/cyclops/cmd/cyclops@latest
    if [[ -f Cargo.toml ]]; then
      cargo install --locked cargo-mutants
    elif [[ -f go.mod ]]; then
      GOTOOLCHAIN=auto go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
    else
      exit 1
    fi
```

No personal access token or repository secret is required. The workflow sets
its own `GITHUB_TOKEN` permissions: `contents: read` to check out and analyze
the code, and `pull-requests: write` to read and update the report comment. The
bundled GitHub-maintained Actions are pinned to full commit SHAs; update those
pins deliberately when upgrading their noted major versions.

### Reporting sequence

On the first run, the analysis fetches the target branch and invokes:

```sh
CYCLOPS_DIFF_BASE=refs/remotes/origin/main
cyclops --diff "$CYCLOPS_DIFF_BASE"
```

After the workflow successfully creates the report comment, that comment stores
the analyzed head SHA in a machine-readable marker. The next run reads the
marker and uses that SHA as `CYCLOPS_DIFF_BASE`, so only newly pushed changes
are analyzed. The stored SHA advances only after a successful Cyclops run (or
a successful no-candidate report) is published.

If the marker is missing or malformed, the comment was deleted, or the stored
SHA is no longer an ancestor after a rebase or force-push, analysis falls back
to the target branch. Before invoking Cyclops, the workflow checks for added,
copied, modified, renamed, type-changed, unmerged, or broken-pair Go production
files or Rust files under `src/`. A deletion-only or otherwise empty
production-code diff is reported as **No mutation candidates**; the mutation
engine is not started.

When an incremental push changes only Go test files or Rust files outside
`src/`, the workflow runs
`cyclops --list-test-targets "$CYCLOPS_DIFF_BASE"` and mutation-tests the
returned production files explicitly. For Go, all buildable production files
in each changed test's package are selected. For Rust integration or custom
test targets, all `.rs` files below the owning Cargo package's `src/` directory
are selected. This is a conservative package-level mapping, not a claim that
every selected file is covered by the changed test. The PR comment labels the
run as `test-affected packages` and lists the chosen production targets.

Package-level mapping is deliberate. Neither Go nor Cargo exposes a stable
static test-to-production dependency map, and coverage-based selection would
require executing and reliably isolating the changed tests before choosing
mutants. It can also miss setup-dependent paths that the changed tests are
intended to exercise. Package membership keeps the mutation scope bounded and
deterministic.

Rust unit tests embedded in `src/**/*.rs` with `#[cfg(test)]` are treated as
production-file changes, so Cyclops uses the ordinary line diff for that file.
Precisely separating an edited inline test from production Rust would require
syntax-aware range analysis; filename and Cargo metadata alone cannot do it
reliably. Rust files outside `src/` that are neither conventional integration
tests nor explicit `[[test]]` targets produce an empty test mapping.

When production and test files change together, the production diff remains
authoritative and Cyclops uses `--diff` as before. Deleted test directories,
packages with no buildable production files, and empty mappings are reported as
no candidates instead of triggering an unfiltered run. Build tags and the
runner's Go environment affect which files `go list` considers buildable.

Each pull request has one analysis concurrency group. A new push cancels an
older analysis, and the comment step compares the analyzed head SHA with the
pull request's current head before updating the comment. A stale run therefore
cannot overwrite newer state. If comment permission or publication fails, the
previous comment and SHA remain unchanged, so the next run includes the
unreported changes.

All potentially failing analysis and comment steps use `continue-on-error`; the
job is informational. Also ensure **Cyclops analysis** is not configured as a
required status check in a branch protection rule or ruleset. Errors remain
visible in the persistent comment when it can be published, and are always
available in the Actions logs.

The bundled workflow supports Go repositories with Gremlins 0.6.0 and Cargo
repositories with cargo-mutants. It expects `go.mod` or `Cargo.toml` at the
repository root.

### Help and version

```sh
cyclops --help
cyclops --version
```

## License

Cyclops is available under the [MIT License](LICENSE).
