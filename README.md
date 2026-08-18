# cyclops

`cyclops` is a small wrapper for mutation testing tools. The first supported
backend is [Gremlins](https://gremlins.dev/).

## Install prerequisites

```sh
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
```

## Run

Build and install Cyclops:

```sh
go install ./cmd/cyclops
```

Then run `cyclops` at the root of a Go module:

```sh
cyclops
```

With no arguments, Cyclops runs `gremlins unleash`, which performs Gremlins'
normal full mutation test. Cyclops forwards standard input, output, error, and
the Gremlins exit code.

To mutate selected source files, pass their paths as positional arguments:

```sh
cyclops internal/service/user.go
cyclops internal/service/user.go internal/service/order.go
```

Cyclops runs the relevant package tree while excluding all unselected source
files from mutation. All selected files must belong to the same Go module.
Duplicate paths are ignored. Test files (`*_test.go`) cannot be mutation
targets.
