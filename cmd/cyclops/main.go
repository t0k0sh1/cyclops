package main

import (
	"os"

	"github.com/t0k0sh1/cyclops/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
