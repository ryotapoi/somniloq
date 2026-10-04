package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func testInputID(t *testing.T, db *core.DB, source core.Source) int64 {
	t.Helper()
	id, err := db.EnsureInput(core.Input{Source: source, Root: "/test/" + string(source)})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testREF(t *testing.T, db *core.DB, source core.Source, sessionID string) string {
	t.Helper()
	rows, err := db.LookupSessionsByID(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Source == source {
			return r.REF
		}
	}
	t.Fatalf("missing session %s/%s", source, sessionID)
	return ""
}

func fixtureREF(source core.Source, id string) string {
	key := sha256.Sum256([]byte(string(source) + "\x00/test/" + string(source)))
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode([]string{id})
	return fmt.Sprintf("slq1:%x:%s:%s", key, source, base64.RawURLEncoding.EncodeToString(bytes.TrimSuffix(buf.Bytes(), []byte("\n"))))
}

func canonicalTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := core.CanonicalPath(path, "")
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
func importedInputID(t *testing.T, db *core.DB, source core.Source, root string) int64 {
	t.Helper()
	id, err := db.EnsureInput(core.Input{Source: source, Root: canonicalTestPath(t, root)})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
