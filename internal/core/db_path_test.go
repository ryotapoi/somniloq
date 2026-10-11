package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseLiteralSpecialCharacterPaths(t *testing.T) {
	for _, relative := range []bool{false, true} {
		name := "absolute"
		if relative {
			name = "relative"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sessions.db?mode=ro#100%25")
			if relative {
				t.Chdir(dir)
				path = filepath.Base(path)
			}
			db, err := OpenDB(path)
			must(t, err)
			defer db.Close()
			root := testTempDir(t)
			writeCursorFullFixture(t, root, "literal", "saved content")
			result, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceCursorAgent, Root: root}}})
			must(t, err)
			if result.FilesImported != 1 || len(result.Errors) != 0 {
				t.Fatalf("import: %+v", result)
			}
			db.db.SetMaxIdleConns(0)
			var fk int
			must(t, db.db.QueryRow("PRAGMA foreign_keys").Scan(&fk))
			if fk != 1 {
				t.Fatal("replacement connection disabled foreign keys")
			}
			must(t, db.Close())
			assertDBFileMode(t, path, 0600)
			data, err := os.ReadFile(path)
			must(t, err)
			if !bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
				t.Fatal("specified file is not SQLite")
			}
			if _, err := os.Stat(filepath.Join(dir, "sessions.db")); !os.IsNotExist(err) {
				t.Fatalf("shortened file exists: %v", err)
			}
			db, err = OpenDBRead(path)
			must(t, err)
			defer db.Close()
			var content string
			must(t, db.db.QueryRow("SELECT content FROM messages").Scan(&content))
			if content != "saved content" {
				t.Fatalf("content = %q", content)
			}
			if _, err := db.db.Exec("DELETE FROM messages"); err == nil {
				t.Fatal("read-only DB accepted write")
			}
		})
	}
}

func TestPrepareMigrationLiteralSpecialCharacterPaths(t *testing.T) {
	for _, queryDestination := range []bool{false, true} {
		name := "special source and destination"
		if queryDestination {
			name = "destination appends query to source"
		}
		t.Run(name, func(t *testing.T) {
			ordinary := makeLegacySnapshot(t, legacySnapshotSchema+`INSERT INTO sessions(source,session_id,imported_at) VALUES('codex','s',''); INSERT INTO messages(uuid,source,session_id,role,content,timestamp) VALUES('u','codex','s','user','saved','');`)
			from := ordinary
			destination := ordinary + "?mode=rwc#100%25"
			if !queryDestination {
				from = ordinary + "?source#100%25"
				must(t, os.Rename(ordinary, from))
				destination = filepath.Join(t.TempDir(), "target.db?mode=ro#100%25")
			}
			before, err := os.ReadFile(from)
			must(t, err)
			var firstDigest string
			for attempt := 0; attempt < 2; attempt++ {
				db, digest, copied, err := prepareMigration(from, destination)
				must(t, err)
				if copied != (attempt == 0) {
					t.Fatalf("attempt %d copied=%v", attempt, copied)
				}
				if attempt == 0 {
					firstDigest = digest
				} else if digest != firstDigest {
					t.Fatal("retry digest changed")
				}
				var content string
				must(t, db.db.QueryRow("SELECT content FROM legacy_messages").Scan(&content))
				if content != "saved" {
					t.Fatalf("content = %q", content)
				}
				must(t, db.Close())
				assertDBFileMode(t, destination, 0600)
				after, err := os.ReadFile(from)
				must(t, err)
				if !bytes.Equal(before, after) {
					t.Fatal("snapshot bytes changed")
				}
				for _, suffix := range []string{"-wal", "-shm", "-journal"} {
					if _, err := os.Lstat(from + suffix); !os.IsNotExist(err) {
						t.Fatalf("source sidecar %s: %v", suffix, err)
					}
				}
			}
			db, err := OpenDBRead(destination)
			must(t, err)
			must(t, db.Close())
		})
	}
}

func TestDatabaseSymlinkParentPathIdentity(t *testing.T) {
	for _, name := range []string{"sessions.db", "sessions.db?mode=ro#100%25"} {
		for _, relative := range []bool{false, true} {
			label := name + "/absolute"
			if relative {
				label = name + "/relative"
			}
			t.Run(label, func(t *testing.T) {
				root := testTempDir(t)
				physical := filepath.Join(root, "physical")
				lexical := filepath.Join(root, "lexical")
				must(t, os.MkdirAll(filepath.Join(physical, "nested"), 0700))
				must(t, os.Mkdir(lexical, 0700))
				must(t, os.Symlink(filepath.Join(physical, "nested"), filepath.Join(lexical, "link")))
				// Keep /../ after the symlink: filepath.Join would change its meaning.
				path := lexical + "/link/../" + name
				if relative {
					t.Chdir(root)
					path = "lexical/link/../" + name
				}
				db, err := OpenDB(path)
				must(t, err)
				defer db.Close()
				inputID := testInput(t, db, SourceCodex)
				must(t, db.UpsertSession(inputID, SessionMeta{Source: SourceCodex, SessionID: "s"}, ""))
				must(t, db.InsertMessage(inputID, NormalizedMessage{Source: SourceCodex, SessionID: "s", UUID: "u", Role: "user", Content: "physical content"}))
				must(t, db.Close())
				actual := filepath.Join(physical, name)
				assertDBFileMode(t, actual, 0600)
				data, err := os.ReadFile(actual)
				must(t, err)
				if !bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
					t.Fatal("OS-resolved file is not SQLite")
				}
				if _, err := os.Stat(filepath.Join(lexical, name)); !os.IsNotExist(err) {
					t.Fatalf("lexically cleaned path exists: %v", err)
				}
				db, err = OpenDBRead(path)
				must(t, err)
				defer db.Close()
				var content string
				must(t, db.db.QueryRow("SELECT content FROM messages").Scan(&content))
				if content != "physical content" {
					t.Fatalf("read content = %q", content)
				}
				if _, err := db.db.Exec("DELETE FROM messages"); err == nil {
					t.Fatal("read-only DB accepted write")
				}
			})
		}
	}
}
