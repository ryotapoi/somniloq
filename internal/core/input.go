package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Input identifies one source's canonical scan root. Name is display-only.
type Input struct {
	Source Source
	Root   string
	Name   string
}

func validSource(source Source) bool {
	return source == SourceClaudeCode || source == SourceCodex || source == SourceCursorAgent
}

func InputKey(source Source, root string) string {
	sum := sha256.Sum256([]byte(string(source) + "\x00" + root))
	return hex.EncodeToString(sum[:])
}

// CanonicalPath resolves an existing path or its longest existing parent.
func CanonicalPath(path, base string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	var tail []string
	parent := path
	for {
		_, err := os.Lstat(parent)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", err
		}
		tail = append(tail, filepath.Base(parent))
		parent = next
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	for i := len(tail) - 1; i >= 0; i-- {
		parent = filepath.Join(parent, tail[i])
	}
	return filepath.Clean(parent), nil
}

func rootIdentity(sessionID string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode([]string{sessionID})
	return strings.TrimSuffix(b.String(), "\n")
}

func RootREF(inputKey string, source Source, sessionID string) string {
	return "slq1:" + inputKey + ":" + string(source) + ":" + base64.RawURLEncoding.EncodeToString([]byte(rootIdentity(sessionID)))
}

type REFError struct{}

func (*REFError) Error() string { return "invalid REF: expected slq1 full reference" }

func parseRootREF(ref string) (key string, source Source, sessionID string, err error) {
	invalid := func() (string, Source, string, error) {
		return "", "", "", &REFError{}
	}
	parts := strings.Split(ref, ":")
	if len(parts) != 4 || parts[0] != "slq1" || len(parts[1]) != 64 {
		return invalid()
	}
	decoded, decodeErr := hex.DecodeString(parts[1])
	if decodeErr != nil || hex.EncodeToString(decoded) != parts[1] {
		return invalid()
	}
	source = Source(parts[2])
	if !validSource(source) {
		return invalid()
	}
	data, decodeErr := base64.RawURLEncoding.DecodeString(parts[3])
	if decodeErr != nil {
		return invalid()
	}
	var identity []string
	if json.Unmarshal(data, &identity) != nil || len(identity) != 1 || identity[0] == "" {
		return invalid()
	}
	if RootREF(parts[1], source, identity[0]) != ref {
		return invalid()
	}
	return parts[1], source, identity[0], nil
}

func (d *DB) EnsureInput(input Input) (int64, error) {
	if !validSource(input.Source) || input.Root == "" {
		return 0, fmt.Errorf("invalid input source/root")
	}
	key := InputKey(input.Source, input.Root)
	_, err := d.db.Exec(`INSERT INTO inputs(input_key,source,root) VALUES(?,?,?) ON CONFLICT(input_key) DO NOTHING`, key, input.Source, input.Root)
	if err != nil {
		return 0, err
	}
	var id int64
	err = d.db.QueryRow(`SELECT id FROM inputs WHERE input_key=?`, key).Scan(&id)
	return id, err
}
