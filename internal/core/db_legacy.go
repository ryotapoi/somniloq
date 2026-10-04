package core

import (
	"encoding/base64"
	"strings"
)

// LegacyInputID identifies saved history whose original input is unknown.
const LegacyInputID int64 = -1

func LegacyREF(digest string, source Source, sessionID string) string {
	return "slq1:legacy:" + digest + ":" + string(source) + ":" + base64.RawURLEncoding.EncodeToString([]byte(sessionID))
}

func savedREF(key string, source Source, identity string) string {
	if strings.HasPrefix(key, "legacy:") {
		return LegacyREF(strings.TrimPrefix(key, "legacy:"), source, identity)
	}
	return IdentityREF(key, source, identity)
}

// Legacy rows remain outside normal input tables so full imports cannot erase
// history with unknown input membership.
const legacySessionRowSelect = `SELECT -1, 'legacy:' || s.snapshot_sha256, s.source, s.session_id, s.session_id,
 '', '', '', '', '', '', '', COALESCE(s.cwd,''), COALESCE(s.repo_path,''),
 COALESCE(s.started_at,''), COALESCE(s.ended_at,''), COALESCE(s.custom_title,''),
 COUNT(m.uuid), COALESCE(SUM(OCTET_LENGTH(m.content)) FILTER (WHERE m.source IN ('codex','claude_code') OR COALESCE(m.is_sidechain,0)=0),0)
 FROM legacy_sessions s LEFT JOIN legacy_messages m
 ON m.snapshot_sha256=s.snapshot_sha256 AND m.source=s.source AND m.session_id=s.session_id`
