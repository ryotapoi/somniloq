package main

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
	"modernc.org/sqlite"
)

func TestImportConfiguredCmdRetainsSummaryAndEarlierDiagnosticOnFatalError(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "input")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		data := fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"cwd":""}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"body"}]}}
`, id)
		if err := os.WriteFile(filepath.Join(root, id+".jsonl"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var db *core.DB
	fn := fmt.Sprintf("close_import_%d", time.Now().UnixNano())
	if err := sqlite.RegisterScalarFunction(fn, 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) { return int64(0), db.Close() }); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "import.db")
	var err error
	db, err = core.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	setup, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Close()
	for _, query := range []string{
		"CREATE TRIGGER reject_group BEFORE INSERT ON sessions WHEN NEW.session_id='b' BEGIN SELECT RAISE(ABORT, 'initial failure'); END",
		"CREATE TRIGGER close_writer AFTER INSERT ON sessions WHEN NEW.session_id='c' BEGIN SELECT " + fn + "(); END",
	} {
		if _, err := setup.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	code, err := importConfiguredCmd(nil, staticDB(db), config{Inputs: []core.Input{{Source: core.SourceCodex, Root: root}}}, strings.NewReader(""), &out, &errOut, false)
	if code != 1 || err == nil || !strings.Contains(err.Error(), "d: begin: sql: database is closed") {
		t.Fatalf("exit=%d err=%v", code, err)
	}
	if out.String() != "Imported 2 files (4 scanned, 0 skipped, 2 failed, 0 unparsed lines)\n" {
		t.Fatalf("stdout: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "b: constraint failed: initial failure") {
		t.Fatalf("initial diagnostic lost: %q", errOut.String())
	}
}
