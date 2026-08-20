package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const FileName = "cyclops.yaml"

type Config struct {
	Backend string `yaml:"backend"`
}

func Load(start string) (Config, string, bool, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return Config{}, "", false, fmt.Errorf("resolve configuration directory: %w", err)
	}
	if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	for current := dir; ; current = filepath.Dir(current) {
		path := filepath.Join(current, FileName)
		info, statErr := os.Stat(path)
		if statErr == nil {
			if !info.Mode().IsRegular() {
				return Config{}, path, true, fmt.Errorf("%s is not a regular file", path)
			}
			loaded, loadErr := read(path)
			return loaded, path, true, loadErr
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return Config{}, path, false, fmt.Errorf("inspect %s: %w", path, statErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return Config{}, "", false, nil
}

func read(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var loaded Config
	if err := decoder.Decode(&loaded); err != nil && err != io.EOF {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("parse %s: multiple YAML documents are not supported", path)
		}
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	loaded.Backend = strings.TrimSpace(loaded.Backend)
	if loaded.Backend == "" {
		return Config{}, fmt.Errorf("parse %s: backend is required", path)
	}
	return loaded, nil
}
