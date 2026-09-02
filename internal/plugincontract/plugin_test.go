package plugincontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pluginRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "noctalia-plugin", "waymarks"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func read(t *testing.T, name string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(pluginRoot(t), name))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func TestManifestContract(t *testing.T) {
	manifest := read(t, "plugin.toml")
	for _, required := range []string{
		`id = "zer0/waymarks"`, `plugin_api = 24`, `dependencies = ["zer0-waymarks-helper"]`,
		`prefix = "wm"`, `entry = "launcher.luau"`, `entry = "panel.luau"`,
	} {
		if !strings.Contains(manifest, required) {
			t.Errorf("manifest missing %q", required)
		}
	}
}

func TestScriptsNeverUseShellCommandStrings(t *testing.T) {
	for _, name := range []string{"shared.luau", "launcher.luau", "panel.luau"} {
		script := read(t, name)
		for _, forbidden := range []string{"/bin/sh", "sh -c", "runAsync(\"", "runStream("} {
			if strings.Contains(script, forbidden) {
				t.Errorf("%s contains forbidden %q", name, forbidden)
			}
		}
	}
	if !strings.Contains(read(t, "shared.luau"), "noctalia.runAsync(argv") {
		t.Error("shared helper does not use argv-table runAsync")
	}
}

func TestPanelProvidesRequiredUserFlows(t *testing.T) {
	panel := read(t, "panel.luau")
	for _, required := range []string{
		`"list", "--query"`, `"add", "--url"`, `"update", editId`, `"remove", bookmark.id`, `"browsers", "--json"`,
		`"--note"`, `"--tags"`, `"set-default-browser"`, `addNote`, `addTags`, `function onKey`, `"ctrl+n"`, `"ctrl+f"`,
		`key = "status"`, `height = 20`, `viewMode == "search"`, `function startAdd`,
		`ui.input`, `ui.select`, `helper.openArguments`, `helper.operationId`,
	} {
		if !strings.Contains(panel, required) {
			t.Errorf("panel missing flow marker %q", required)
		}
	}
	launcher := read(t, "launcher.luau")
	for _, required := range []string{`function onQuery`, `function onActivate`, `helper.openArguments`} {
		if !strings.Contains(launcher, required) {
			t.Errorf("launcher missing flow marker %q", required)
		}
	}
}

func TestEnglishTranslationsAreValidJSON(t *testing.T) {
	var translations map[string]string
	if err := json.Unmarshal([]byte(read(t, "translations/en.json")), &translations); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"settings.browser_id.label", "settings.browser_id.description", "settings.profile_id.label", "settings.profile_id.description"} {
		if translations[key] == "" {
			t.Errorf("missing translation %q", key)
		}
	}
}
