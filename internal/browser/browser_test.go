package browser

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCommandUsesFixedArguments(t *testing.T) {
	t.Parallel()
	config := Config{Browsers: []Browser{
		{
			ID:         "firefox",
			Executable: "/bin/echo",
			Args:       []string{"--new-window"},
			Profiles:   []Profile{{ID: "work", Args: []string{"--profile", "Work Profile"}}},
		},
	}}
	command, err := config.Command("firefox", "work", "https://example.test/?q=$(touch+pwned)")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/bin/echo", "--new-window", "--profile", "Work Profile", "https://example.test/?q=$(touch+pwned)"}
	if len(command.Args) != len(want) {
		t.Fatalf("args=%q", command.Args)
	}
	for index := range want {
		if command.Args[index] != want[index] {
			t.Fatalf("arg %d=%q want %q", index, command.Args[index], want[index])
		}
	}
}

func TestLoadLowercaseJSONFieldsAndPublicRedaction(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	filename := filepath.Join(t.TempDir(), "config.json")
	contents := `{"browsers":[{"id":"firefox","name":"Firefox","executable":"/bin/echo","args":["--new-window"],"profiles":[{"id":"work","name":"Work","args":["--profile","work"]}]}]}`
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Browsers) != 1 || config.Browsers[0].Executable != "/bin/echo" {
		t.Fatalf("config=%#v", config)
	}
	public := config.Public()
	if public[0].Executable != "" || len(public[0].Args) != 0 || len(public[0].Profiles) != 1 || len(public[0].Profiles[0].Args) != 0 {
		t.Fatalf("public leaked config: %#v", public)
	}
}

func TestLoadDiscoversHeliumAndPersistsDefault(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "helium-browser")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	filename := filepath.Join(t.TempDir(), "config.json")
	config, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Browsers) != 1 || config.Browsers[0].ID != "helium" || config.DefaultBrowserID != "helium" {
		t.Fatalf("config=%#v", config)
	}
	if err := config.SetDefault("missing"); !errors.Is(err, ErrBrowserNotAllowed) {
		t.Fatalf("SetDefault error=%v", err)
	}
	if err := config.Save(filename); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(filename)
	if err != nil || loaded.DefaultBrowserID != "helium" {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}
