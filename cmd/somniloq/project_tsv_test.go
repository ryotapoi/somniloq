package main

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestProjectOutput_ControlCharacters(t *testing.T) {
	const alias = "alias\twith\nline\rend"
	const rawPath = "/repos/raw\twith\nline\rend"
	const collisionPath = "/repos/raw with line end"
	cfg := config{ProjectAliases: map[string][]string{alias: {"old-one", "old-two"}}}
	commands := []struct {
		name    string
		run     func([]string, func() (*core.DB, error), config, io.Writer, io.Writer) (int, error)
		columns int
		project int
	}{
		{"sessions", sessionsCmd, 8, 3},
		{"projects", projectsCmd, 2, 0},
	}
	for _, command := range commands {
		for _, short := range []bool{false, true} {
			for _, format := range []string{"tsv", "json"} {
				t.Run(fmt.Sprintf("%s/short=%v/%s", command.name, short, format), func(t *testing.T) {
					db, err := core.OpenDB(":memory:")
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { db.Close() })
					paths := []string{"/repos/old-one", "/repos/old-two", rawPath, collisionPath, "/repos/normal"}
					for i, path := range paths {
						if err := db.UpsertSession(testInputID(t, db,
							core.SourceClaudeCode), core.SessionMeta{
							Source: core.SourceClaudeCode, SessionID: fmt.Sprintf("session-%d", i),
							RepoPath: path, StartedAt: "2026-03-29T10:00:00Z",
						}, "2026-03-29T15:00:00Z"); err != nil {
							t.Fatal(err)
						}
					}
					args := []string{"--format", format}
					if short {
						args = append(args, "--short")
					}
					var out, errOut bytes.Buffer
					code, err := command.run(args, staticDB(db), cfg, &out, &errOut)
					if err != nil || code != 0 || errOut.Len() != 0 {
						t.Fatalf("command: code=%d err=%v stderr=%q", code, err, errOut.String())
					}

					wantRaw, wantCollision, wantNormal := rawPath, collisionPath, "/repos/normal"
					if short {
						wantRaw, wantCollision, wantNormal = "raw\twith\nline\rend", "raw with line end", "normal"
					}
					wantRows := 5
					aliasRows := 2
					if command.name == "projects" {
						wantRows, aliasRows = 4, 1
					}
					gotNames := map[string]int{}
					if format == "json" {
						rows := decodeJSONArray(t, out.Bytes())
						if len(rows) != wantRows {
							t.Fatalf("JSON rows = %d, want %d", len(rows), wantRows)
						}
						for _, row := range rows {
							project := row["project"].(string)
							gotNames[project]++
							if command.name == "projects" {
								wantCount := float64(1)
								if project == alias {
									wantCount = 2
								}
								if row["sessionCount"] != wantCount {
									t.Errorf("%q sessionCount = %v, want %v", project, row["sessionCount"], wantCount)
								}
							}
						}
						want := map[string]int{alias: aliasRows, wantRaw: 1, wantCollision: 1, wantNormal: 1}
						if !reflect.DeepEqual(gotNames, want) {
							t.Errorf("JSON projects = %#v, want %#v", gotNames, want)
						}
					} else {
						rows := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
						if len(rows) != wantRows || strings.Contains(out.String(), "\r") {
							t.Fatalf("TSV row boundaries: want %d rows without CR, got %q", wantRows, out.String())
						}
						for _, row := range rows {
							columns := strings.Split(row, "\t")
							if len(columns) != command.columns {
								t.Fatalf("TSV columns = %d, want %d: %q", len(columns), command.columns, row)
							}
							gotNames[columns[command.project]]++
							if command.name == "projects" {
								wantCount := "1"
								if columns[0] == "alias with line end" {
									wantCount = "2"
								}
								if columns[1] != wantCount {
									t.Errorf("TSV count = %q, want %q: %q", columns[1], wantCount, row)
								}
							}
						}
						want := map[string]int{"alias with line end": aliasRows, wantCollision: 2, wantNormal: 1}
						if !reflect.DeepEqual(gotNames, want) {
							t.Errorf("TSV projects = %#v, want %#v", gotNames, want)
						}
					}
				})
			}
		}
	}
}
