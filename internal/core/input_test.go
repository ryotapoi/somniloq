package core

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalPath_ResolvesExistingParentAndHome(t *testing.T) {
	base := testTempDir(t)
	real := filepath.Join(base, "real")
	must(t, os.Mkdir(real, 0700))
	must(t, os.Symlink(real, filepath.Join(base, "alias")))
	for _, path := range []string{"alias/missing/tail", "alias/./missing/tail", filepath.Join(base, "alias", "missing", "tail")} {
		got, err := CanonicalPath(path, base)
		must(t, err)
		if want := filepath.Join(real, "missing", "tail"); got != want {
			t.Fatalf("%q = %q, want %q", path, got, want)
		}
	}
	t.Setenv("HOME", real)
	for _, tc := range []struct{ path, want string }{{"~", real}, {"~/tail", filepath.Join(real, "tail")}, {"~other", filepath.Join(base, "~other")}, {"mid~", filepath.Join(base, "mid~")}} {
		got, err := CanonicalPath(tc.path, base)
		must(t, err)
		if got != tc.want {
			t.Fatalf("%q = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestRootREF_StrictCanonicalIdentity(t *testing.T) {
	key := InputKey(SourceCodex, "/root")
	id := "session<&>日本語"
	ref := RootREF(key, SourceCodex, id)
	k, source, got, err := parseRootREF(ref)
	must(t, err)
	if k != key || source != SourceCodex || got != id {
		t.Fatalf("parsed %q %q %q", k, source, got)
	}
	body, err := base64.RawURLEncoding.DecodeString(strings.Split(ref, ":")[3])
	must(t, err)
	if string(body) != `["session<&>日本語"]` {
		t.Fatalf("identity = %s", body)
	}
	prefix := "slq1:" + key + ":codex:"
	for _, invalid := range []string{id, ref[:20], strings.Replace(ref, "slq1:", "slq2:", 1), strings.Replace(ref, ":codex:", ":other:", 1), strings.Replace(ref, key, strings.ToUpper(key), 1), ref + "=", prefix + base64.RawURLEncoding.EncodeToString([]byte(`[ "session" ]`)), prefix + base64.RawURLEncoding.EncodeToString([]byte(`[""]`)), prefix + base64.RawURLEncoding.EncodeToString([]byte(`["parent","child"]`))} {
		if _, _, _, err := parseRootREF(invalid); err == nil {
			t.Fatalf("accepted invalid REF %q", invalid)
		}
	}
}
