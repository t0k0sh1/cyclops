package cli

import (
	"fmt"
	"io"
	"strings"
)

const Version = "0.1.0"

type Options struct {
	List, DryRun, Help, Version bool
	Diff                        string
	Targets                     []string
}

func Parse(args []string) (Options, error) {
	var parsed Options
	parseOptions := true
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if parseOptions && arg == "--" {
			parseOptions = false
			continue
		}
		if parseOptions {
			if arg == "--diff" {
				if parsed.Diff != "" {
					return Options{}, fmt.Errorf("--diff may only be specified once")
				}
				if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
					return Options{}, fmt.Errorf("--diff requires a branch or commit")
				}
				index++
				parsed.Diff = args[index]
				if parsed.Diff == "" {
					return Options{}, fmt.Errorf("--diff requires a branch or commit")
				}
				continue
			}
			if strings.HasPrefix(arg, "--diff=") {
				if parsed.Diff != "" {
					return Options{}, fmt.Errorf("--diff may only be specified once")
				}
				parsed.Diff = strings.TrimPrefix(arg, "--diff=")
				if parsed.Diff == "" {
					return Options{}, fmt.Errorf("--diff requires a branch or commit")
				}
				continue
			}
			switch arg {
			case "--list":
				parsed.List = true
				continue
			case "--dry-run":
				parsed.DryRun = true
				continue
			case "--help":
				parsed.Help = true
				continue
			case "--version":
				parsed.Version = true
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return Options{}, fmt.Errorf("unknown option: %s", arg)
			}
		}
		parsed.Targets = append(parsed.Targets, arg)
	}
	if parsed.List && parsed.DryRun {
		return Options{}, fmt.Errorf("--list and --dry-run cannot be used together")
	}
	if parsed.List && parsed.Diff != "" {
		return Options{}, fmt.Errorf("--list and --diff cannot be used together")
	}
	if parsed.Help && (parsed.Version || parsed.List || parsed.DryRun || parsed.Diff != "" || len(parsed.Targets) > 0) {
		return Options{}, fmt.Errorf("--help cannot be combined with other options or targets")
	}
	if parsed.Version && (parsed.List || parsed.DryRun || parsed.Diff != "" || len(parsed.Targets) > 0) {
		return Options{}, fmt.Errorf("--version cannot be combined with other options or targets")
	}
	return parsed, nil
}

func PrintHelp(stdout io.Writer) {
	fmt.Fprint(stdout, `Cyclops runs mutation tests with the backend detected for the current project.

Usage:
  cyclops [options] [file-or-glob...]

Options:
  --diff REF  Test mutants within changes from a branch or commit
  --dry-run  Analyze mutant candidates without testing them
  --help     Show this help
  --list     List the selected source files without running Gremlins
  --version  Show the Cyclops version

Examples:
  cyclops
  cyclops internal/service/user.go
  cyclops 'internal/**/*.go'
  cyclops --diff origin/main
  cyclops --dry-run --diff HEAD~1
  cyclops --list 'foo/{bar,bas}/**/*.go'
  cyclops --dry-run 'internal/**/*.go'
`)
}
