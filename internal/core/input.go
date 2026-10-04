package core

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ryotapoi/somniloq/internal/ingest"
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

func rootIdentity(sessionID string) string { return ingest.Identity(sessionID) }

func RootREF(inputKey string, source Source, sessionID string) string {
	return IdentityREF(inputKey, source, rootIdentity(sessionID))
}

// IdentityREF identifies a single source conversation in one input.
func IdentityREF(inputKey string, source Source, identity string) string {
	return "slq1:" + inputKey + ":" + string(source) + ":" + base64.RawURLEncoding.EncodeToString([]byte(identity))
}

type REFError struct{}

func (*REFError) Error() string { return "invalid REF: expected slq1 full reference" }

func parseREF(ref string) (key string, source Source, sessionID string, err error) {
	invalid := func() (string, Source, string, error) {
		return "", "", "", &REFError{}
	}
	parts := strings.Split(ref, ":")
	if len(parts) == 5 && parts[0] == "slq1" && parts[1] == "legacy" {
		digest, e := hex.DecodeString(parts[2])
		data, de := base64.RawURLEncoding.DecodeString(parts[4])
		src := Source(parts[3])
		if e != nil || len(digest) != 32 || hex.EncodeToString(digest) != parts[2] || de != nil || len(data) == 0 || !utf8.Valid(data) || !validSource(src) || LegacyREF(parts[2], src, string(data)) != ref {
			return invalid()
		}
		return "legacy:" + parts[2], src, string(data), nil
	}
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
	if json.Unmarshal(data, &identity) != nil || (len(identity) != 1 && !(source == SourceClaudeCode && len(identity) == 2)) || identity[0] == "" || (len(identity) == 2 && identity[1] == "") {
		return invalid()
	}
	if IdentityREF(parts[1], source, ingest.Identity(identity...)) != ref {
		return invalid()
	}
	return parts[1], source, ingest.Identity(identity...), nil
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

func parseRootREF(ref string) (string, Source, string, error) {
	key, source, identity, err := parseREF(ref)
	if err != nil {
		return "", "", "", err
	}
	if strings.HasPrefix(key, "legacy:") {
		return "", "", "", &REFError{}
	}
	var parts []string
	_ = json.Unmarshal([]byte(identity), &parts)
	if len(parts) != 1 {
		return "", "", "", &REFError{}
	}
	return key, source, parts[0], nil
}
