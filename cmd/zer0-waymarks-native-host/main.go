package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zer0/zer0-waymarks/internal/bridge"
	"github.com/zer0/zer0-waymarks/internal/nativehost"
	"github.com/zer0/zer0-waymarks/internal/store"
)

func main() {
	if err := validateArguments(os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "native host received invalid arguments")
		os.Exit(1)
	}
	root, err := dataDirectory()
	if err == nil {
		var localStore *store.Store
		localStore, err = store.Open(root)
		if err == nil {
			var stateRoot string
			stateRoot, err = stateDirectory()
			if err == nil {
				err = nativehost.Serve(os.Stdin, os.Stdout, bridge.New(localStore, stateRoot))
			}
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "native host failed")
		os.Exit(1)
	}
}

func validateArguments(arguments []string) error {
	if len(arguments) == 1 {
		return nil
	}
	if len(arguments) != 2 || !filepath.IsAbs(arguments[1]) || filepath.Base(arguments[1]) != "zer0.waymarks.json" {
		return fmt.Errorf("unexpected native host arguments")
	}
	info, err := os.Stat(arguments[1])
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("invalid native host manifest")
	}
	return nil
}

func stateDirectory() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base != "" && !filepath.IsAbs(base) {
		return "", fmt.Errorf("XDG_STATE_HOME must be absolute")
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "zer0-waymarks"), nil
}

func dataDirectory() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base != "" && !filepath.IsAbs(base) {
		return "", fmt.Errorf("XDG_DATA_HOME must be absolute")
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "zer0-waymarks"), nil
}
