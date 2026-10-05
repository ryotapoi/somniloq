package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

const minimalTOMLConfig = "db = 'history.db'\n[[inputs]]\nsource = 'codex'\nroot = 'logs'\n"

func writeConfigFixture(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigStrictValidation(t *testing.T) {
	tests := map[string]string{
		"old JSON":                 `{"db":"history.db"}`,
		"unknown key":              "unknown = 1\n" + minimalTOMLConfig,
		"excluded patterns":        "excludeUserMessagePatterns = ['^/']\n" + minimalTOMLConfig,
		"unknown input key":        minimalTOMLConfig + "extra = 'x'\n",
		"database type":            "db = 3\n[[inputs]]\nsource = 'codex'\nroot = 'logs'\n",
		"root type":                "db = 'history.db'\n[[inputs]]\nsource = 'codex'\nroot = 3\n",
		"name type":                minimalTOMLConfig + "name = 3\n",
		"alias type":               minimalTOMLConfig + "[projectAliases]\napp = [3]\n",
		"empty database":           "db = ''\n[[inputs]]\nsource = 'codex'\nroot = 'logs'\n",
		"missing database":         "[[inputs]]\nsource = 'codex'\nroot = 'logs'\n",
		"empty inputs":             "db = 'history.db'\ninputs = []\n",
		"missing inputs":           "db = 'history.db'\n",
		"empty root":               "db = 'history.db'\n[[inputs]]\nsource = 'codex'\nroot = ''\n",
		"database source":          "db = 'history.db'\n[[inputs]]\nsource = 'claude_code'\nroot = 'logs'\n",
		"unknown source":           "db = 'history.db'\n[[inputs]]\nsource = 'other'\nroot = 'logs'\n",
		"day boundary":             "dayBoundary = '24:00'\n" + minimalTOMLConfig,
		"empty day boundary":       "dayBoundary = ''\n" + minimalTOMLConfig,
		"signed day boundary":      "dayBoundary = '+1:00'\n" + minimalTOMLConfig,
		"day boundary type":        "dayBoundary = 4\n" + minimalTOMLConfig,
		"alias canonical conflict": minimalTOMLConfig + "[projectAliases]\napp = ['other']\nother = ['old']\n",
		"alias group conflict":     minimalTOMLConfig + "[projectAliases]\napp = ['old']\nother = ['old']\n",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeConfigFixture(t, t.TempDir(), body)
			if _, err := loadConfig(path); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadConfigNameAndDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".somniloq", "config")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "work_2.toml")
	if err := os.WriteFile(path, []byte(minimalTOMLConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig("work_2")
	if err != nil {
		t.Fatal(err)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.DB != filepath.Join(realDir, "history.db") || c.Inputs[0].Root != filepath.Join(realDir, "logs") || c.DayBoundary != "00:00" {
		t.Fatalf("config = %+v", c)
	}
	for _, value := range []string{"", "missing", filepath.Join(home, "missing.toml")} {
		if _, err := loadConfig(value); err == nil || !strings.Contains(err.Error(), configSetupHint) {
			t.Fatalf("loadConfig(%q): %v", value, err)
		}
	}
}

func TestLoadConfigSymlinkBaseAndDuplicateInputs(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	linkDir := filepath.Join(dir, "link")
	for _, d := range []string{real, linkDir, filepath.Join(real, "logs")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("logs", filepath.Join(real, "alias")); err != nil {
		t.Fatal(err)
	}
	body := minimalTOMLConfig + "[[inputs]]\nname = 'renamed'\nsource = 'codex'\nroot = 'alias'\n[[inputs]]\nsource = 'claude-code'\nroot = 'alias'\n[projectAliases]\napp = ['old']\n"
	target := writeConfigFixture(t, real, body)
	path := filepath.Join(linkDir, "chosen.toml")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	canonicalReal, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	if c.DB != filepath.Join(canonicalReal, "history.db") || len(c.Inputs) != 2 || c.Inputs[0].Root != filepath.Join(canonicalReal, "logs") || c.Inputs[1].Source != core.SourceClaudeCode {
		t.Fatalf("config = %+v", c)
	}
	if !reflect.DeepEqual(c.expandProject("old"), []string{"app", "old"}) {
		t.Fatal("alias expansion lost")
	}
}

func TestConfigInitNamedArbitraryOutputAndNoDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	destination := filepath.Join(home, "custom", "profile.toml")
	var out, errOut bytes.Buffer
	code, err := configInitCmd([]string{"work", "--output", destination}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, errOut.String())
	}
	canonicalHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != filepath.Join(canonicalHome, "custom", "profile.toml")+"\n" {
		t.Fatalf("stdout=%q", out.String())
	}
	c, err := loadConfig(destination)
	if err != nil {
		t.Fatal(err)
	}
	if c.DB != filepath.Join(canonicalHome, ".somniloq", "work.db") || len(c.Inputs) != 3 {
		t.Fatalf("config=%+v", c)
	}
	if _, err := os.Stat(filepath.Join(home, ".somniloq")); !os.IsNotExist(err) {
		t.Fatalf("init created DB parent: %v", err)
	}
	if c.Inputs[0].Name != "Claude" || c.Inputs[1].Name != "Codex" || c.Inputs[2].Name != "Cursor" {
		t.Fatalf("default names = %+v", c.Inputs)
	}
}

func TestConfigInitExclusiveDestinations(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, kind := range []string{"file", "directory", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			destination := filepath.Join(dir, "config.toml")
			target := filepath.Join(dir, "target")
			switch kind {
			case "file":
				if err := os.WriteFile(destination, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(destination, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
				fallthrough
			case "dangling symlink":
				if err := os.Symlink(target, destination); err != nil {
					t.Fatal(err)
				}
			}
			var out, errOut bytes.Buffer
			code, err := configInitCmd([]string{"--output", destination}, &out, &errOut)
			if err != nil || code != 2 || out.Len() != 0 {
				t.Fatalf("code=%d err=%v stdout=%q", code, err, out.String())
			}
			if kind == "file" || kind == "symlink" {
				data, err := os.ReadFile(destination)
				if err != nil || string(data) != "keep" {
					t.Fatalf("contents changed: %s, %v", data, err)
				}
			}
			if kind == "dangling symlink" {
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatalf("dangling target created: %v", err)
				}
			}
		})
	}
}

func TestConfigInitDefaultAndExplicitDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out, errOut bytes.Buffer
	code, err := configInitCmd([]string{"--db", "custom.db"}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	c, err := loadConfig("default")
	if err != nil {
		t.Fatal(err)
	}
	if c.DB != filepath.Join(filepath.Dir(c.Path), "custom.db") {
		t.Fatalf("db=%q", c.DB)
	}
	if _, err := os.Stat(c.DB); !os.IsNotExist(err) {
		t.Fatalf("DB created: %v", err)
	}
	for _, args := range [][]string{{"bad.name"}, {"one", "two"}, {"--unknown"}} {
		code, err := configInitCmd(args, &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil || code != 2 {
			t.Fatalf("args=%v code=%d err=%v", args, code, err)
		}
	}
}

func TestLoadConfigLiteralPathAndHomeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_ROOT", "expanded")
	path := writeConfigFixture(t, home, "db = '~/missing/history.db'\n[[inputs]]\nsource = 'codex'\nroot = '~'\n[[inputs]]\nsource = 'codex'\nroot = '~other'\n[[inputs]]\nsource = 'codex'\nroot = '$CONFIG_ROOT'\n")
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	if c.DB != filepath.Join(realHome, "missing", "history.db") || c.Inputs[0].Root != realHome || c.Inputs[1].Root != filepath.Join(realHome, "~other") || c.Inputs[2].Root != filepath.Join(realHome, "$CONFIG_ROOT") {
		t.Fatalf("config = %+v", c)
	}
	// Relative config paths use cwd, while their contents use the config parent.
	t.Chdir(home)
	if _, err := loadConfig("./config.toml"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(home, "bare")); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig("./bare"); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig("bare"); err == nil {
		t.Fatal("bare should resolve a config name, not a cwd file")
	}
	if err := os.Symlink(filepath.Join(home, "missing"), filepath.Join(home, "dangling.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig("./dangling.toml"); err == nil || !strings.Contains(err.Error(), configSetupHint) {
		t.Fatalf("missing symlink error = %v", err)
	}
}

func TestLoadConfigMissingSuffixBelowSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	path := writeConfigFixture(t, dir, "db = 'alias/missing/db.sqlite'\n[[inputs]]\nsource = 'cursor-agent'\nroot = 'alias/missing/logs'\n")
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	real, err = filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	if c.DB != filepath.Join(real, "missing", "db.sqlite") || c.Inputs[0].Root != filepath.Join(real, "missing", "logs") {
		t.Fatalf("config = %+v", c)
	}
	if _, err := os.Stat(filepath.Join(real, "missing")); !os.IsNotExist(err) {
		t.Fatalf("load created missing suffix: %v", err)
	}
}

func TestLoadConfigIOErrorClassification(t *testing.T) {
	dir := t.TempDir()
	_, err := loadConfig(dir)
	var ioErr *ConfigIOError
	if !errors.As(err, &ioErr) {
		t.Fatalf("directory read should be I/O error: %v", err)
	}
	path := writeConfigFixture(t, dir, "db = ''\n")
	_, err = loadConfig(path)
	if errors.As(err, &ioErr) {
		t.Fatalf("validation should be input error: %v", err)
	}
	_, err = loadConfig(filepath.Join(dir, "missing.toml"))
	if errors.As(err, &ioErr) {
		t.Fatalf("missing config should be input error: %v", err)
	}
}

func TestConfigInitPreservesExistingConfigAndDatabase(t *testing.T) {
	for _, tt := range []struct {
		explicitDB bool
		existing   string
	}{
		{false, "config"},
		{false, "database"},
		{true, "database"},
	} {
		t.Run(fmt.Sprintf("explicit=%t/%s", tt.explicitDB, tt.existing), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, ".somniloq", "config")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(dir, "work.toml")
			dbPath := filepath.Join(home, ".somniloq", "work.db")
			args := []string{"work"}
			if tt.explicitDB {
				dbPath = filepath.Join(dir, "custom.db")
				args = append(args, "--db", "custom.db")
			}
			existingPath := dbPath
			if tt.existing == "config" {
				existingPath = configPath
			}
			if err := os.WriteFile(existingPath, []byte("keep "+existingPath), 0o600); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			code, err := configInitCmd(args, &out, &errOut)
			if code != 2 || err != nil || out.Len() != 0 || !strings.Contains(errOut.String(), "already exists") {
				t.Fatalf("init: %d %v %q %q", code, err, out.String(), errOut.String())
			}
			data, err := os.ReadFile(existingPath)
			if err != nil || string(data) != "keep "+existingPath {
				t.Fatalf("modified %s: %q %v", existingPath, data, err)
			}
			if tt.existing == "database" {
				if _, err := os.Lstat(configPath); !os.IsNotExist(err) {
					t.Fatalf("created config: %v", err)
				}
			}
		})
	}
}

func TestConfigInitDatabaseEntriesAndPathBase(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "dangling symlink", "home"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			// The command cwd differs from the output parent, which itself is a symlink.
			t.Chdir(t.TempDir())
			real := filepath.Join(home, "real")
			if err := os.Mkdir(real, 0o700); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(home, "alias")
			if err := os.Symlink(real, alias); err != nil {
				t.Fatal(err)
			}
			dbPath := filepath.Join(real, "history.db")
			argument := "history.db"
			target := filepath.Join(real, "target")
			switch kind {
			case "home":
				argument = "~/real/history.db"
				fallthrough
			case "file":
				if err := os.WriteFile(dbPath, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(dbPath, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
				fallthrough
			case "dangling symlink":
				if err := os.Symlink(target, dbPath); err != nil {
					t.Fatal(err)
				}
			}
			destination := filepath.Join(alias, "profile.toml")
			var out, errOut bytes.Buffer
			code, err := configInitCmd([]string{"--output", destination, "--db", argument}, &out, &errOut)
			if code != 2 || err != nil || out.Len() != 0 || !strings.Contains(errOut.String(), "database destination already exists") {
				t.Fatalf("init: %d %v %q %q", code, err, out.String(), errOut.String())
			}
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatalf("created config: %v", err)
			}
			if kind == "file" || kind == "symlink" || kind == "home" {
				data, err := os.ReadFile(dbPath)
				if err != nil || string(data) != "keep" {
					t.Fatalf("changed database: %q %v", data, err)
				}
			}
			if kind == "dangling symlink" {
				if link, err := os.Readlink(dbPath); err != nil || link != target {
					t.Fatalf("changed symlink: %q %v", link, err)
				}
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatalf("created target: %v", err)
				}
			}
		})
	}
}
