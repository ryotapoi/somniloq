package ingest

// Source identifies the origin of normalized session log records.
type Source string

const (
	SourceClaudeCode Source = "claude_code"
	SourceCodex      Source = "codex"
)

// File is a JSONL file discovered by a source-specific adapter.
type File struct {
	Path      string
	SessionID string
}

// ImportState records the incremental import cursor for one JSONL file.
type ImportState struct {
	JSONLPath  string
	Source     Source
	FileSize   int64
	LastOffset int64
	ImportedAt string
}

// NormalizedMessage is a source-independent message row ready for persistence.
type NormalizedMessage struct {
	UUID        string
	Source      Source
	ParentUUID  *string
	SessionID   string
	Role        string
	Content     string
	Timestamp   string
	IsSidechain bool
}

// SessionMeta is a source-independent session row ready for persistence.
type SessionMeta struct {
	Source    Source
	SessionID string
	CWD       string
	RepoPath  string
	GitBranch string
	Version   string
	StartedAt string
	EndedAt   string
}

// NormalizedRecord is one conversation record normalized into session and
// message data. Metadata-only records are handled by adapters because their
// meaning is source-specific.
type NormalizedRecord struct {
	Session SessionMeta
	Message NormalizedMessage
}

// ImportTransaction is the source-neutral persistence surface adapters need
// while processing one file. internal/core owns the SQLite implementation
// behind this interface. Source-specific writes (e.g. claude-code session
// titles) live in extension interfaces next to the adapter that needs them,
// asserted against the concrete transaction.
type ImportTransaction interface {
	UpsertSession(meta SessionMeta, importedAt string) error
	InsertMessage(msg NormalizedMessage) error
	UpsertImportState(state ImportState) error
	Commit() error
	Rollback() error
}

// Store starts the transaction used by an adapter to persist one file.
type Store interface {
	BeginImport() (ImportTransaction, error)
}

// RepoResolver resolves a session's working directory to its repository root.
// Contract: an empty cwd resolves to ""; a cwd whose root cannot be determined
// resolves to the cwd itself, never "" — persistence treats "" as missing
// (NULL), which would silently break per-project grouping.
type RepoResolver func(cwd string) string

// Adapter scans and imports one source's JSONL format into normalized records.
type Adapter interface {
	Source() Source
	ScanFiles(rootDir string) ([]File, error)
	ProcessFile(store Store, file File, offset, fileSize int64, importedAt string) (ProcessResult, error)
}
