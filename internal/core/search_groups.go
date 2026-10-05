package core

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SearchCandidates selects saved owners before their bodies are read or matched.
type SearchCandidates struct {
	Inputs   []string
	Sources  []Source
	Projects []string
}

type SearchGroup struct {
	REF            string   `json:"ref"`
	Input          *string  `json:"input"`
	Source         Source   `json:"source"`
	Project        *string  `json:"project"`
	Title          *string  `json:"title"`
	StartedAt      *string  `json:"startedAt"`
	LastAt         *string  `json:"lastAt"`
	ImportedAt     *string  `json:"importedAt"`
	Members        []string `json:"members"`
	MatchedMembers []string `json:"matchedMembers"`
	MemberCount    int      `json:"memberCount"`
}

type searchOwner struct {
	SessionRow
	input, project, title, imported *string
	first, last                     *string
}

func (c SearchCandidates) accepts(s searchOwner) bool {
	if len(c.Inputs) > 0 {
		found := false
		for _, p := range c.Inputs {
			if s.input != nil && *s.input == p {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if len(c.Sources) > 0 {
		found := false
		for _, source := range c.Sources {
			if s.Source == source {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if len(c.Projects) > 0 {
		found := false
		if s.project != nil && *s.project != "" {
			name := filepath.Base(*s.project)
			for _, p := range c.Projects {
				if strings.Contains(name, p) {
					found = true
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (d *DB) searchOwners() ([]searchOwner, error) {
	rows, err := d.execer().Query(`SELECT s.input_id,i.input_key,s.source,s.identity,s.session_id,s.parent_identity,s.root_identity,i.root,s.repo_path,s.custom_title,s.imported_at FROM sessions s JOIN inputs i ON i.id=s.input_id
 UNION ALL SELECT -1,'legacy:'||snapshot_sha256,source,session_id,session_id,'','',NULL,repo_path,custom_title,imported_at FROM legacy_sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	owners := []searchOwner{}
	for rows.Next() {
		var s searchOwner
		var key string
		if err := rows.Scan(&s.InputID, &key, &s.Source, &s.Identity, &s.SessionID, &s.ParentIdentity, &s.RootIdentity, &s.input, &s.project, &s.title, &s.imported); err != nil {
			return nil, err
		}
		s.REF = savedREF(key, s.Source, s.Identity)
		if s.project != nil {
			s.RepoPath = *s.project
		}
		owners = append(owners, s)
	}
	return owners, rows.Err()
}

// updateKnownTime compares instants while retaining the original timestamp.
func updateKnownTime(current **string, raw string, latest bool) {
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return
	}
	if *current != nil {
		old, _ := time.Parse(time.RFC3339Nano, **current)
		if latest && !parsed.After(old) || !latest && !parsed.Before(old) {
			return
		}
	}
	value := raw
	*current = &value
}

func (d *DB) readOwnerTimes(owners []searchOwner) error {
	byREF := map[string]int{}
	for i, s := range owners {
		byREF[s.REF] = i
	}
	rows, err := d.execer().Query(`SELECT i.input_key,m.source,m.identity,m.timestamp FROM messages m JOIN inputs i ON i.id=m.input_id WHERE m.membership='body' AND (m.source IN ('codex','claude_code') OR COALESCE(m.is_sidechain,0)=0)
 UNION ALL SELECT 'legacy:'||snapshot_sha256,source,session_id,timestamp FROM legacy_messages WHERE source IN ('codex','claude_code') OR COALESCE(is_sidechain,0)=0`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, id string
		var source Source
		var raw *string
		if err := rows.Scan(&key, &source, &id, &raw); err != nil {
			return err
		}
		if raw == nil {
			continue
		}
		if i, ok := byREF[savedREF(key, source, id)]; ok {
			updateKnownTime(&owners[i].first, *raw, false)
			updateKnownTime(&owners[i].last, *raw, true)
		}
	}
	return rows.Err()
}

type searchBody struct {
	ref, role, content, timestamp string
	number                        int
}

func (d *DB) candidateBodies(owners []searchOwner, filter SessionFilter) ([]searchBody, error) {
	identities := [][3]string{}
	for _, s := range owners {
		key, _, _, _ := parseREF(s.REF)
		identities = append(identities, [3]string{key, string(s.Source), s.Identity})
	}
	encoded, err := json.Marshal(identities)
	if err != nil {
		return nil, err
	}
	q := `WITH candidates AS (SELECT json_extract(value,'$[0]') k,json_extract(value,'$[1]') src,json_extract(value,'$[2]') id FROM json_each(?)), bodies AS (
 SELECT c.k,m.source,m.identity,m.role,m.content,m.timestamp,m.number FROM candidates c JOIN inputs i ON i.input_key=c.k JOIN messages m ON m.input_id=i.id AND m.source=c.src AND m.identity=c.id WHERE m.membership='body' AND (m.source IN ('codex','claude_code') OR COALESCE(m.is_sidechain,0)=0)
 UNION ALL SELECT c.k,m.source,m.session_id,m.role,m.content,m.timestamp,m.number FROM candidates c JOIN legacy_messages m ON m.snapshot_sha256=substr(c.k,8) AND m.source=c.src AND m.session_id=c.id WHERE m.source IN ('codex','claude_code') OR COALESCE(m.is_sidechain,0)=0)
 SELECT k,source,identity,role,content,COALESCE(timestamp,''),number FROM bodies m`
	conditions, args := timeFilterConditions(filter, messageTimestampColumn)
	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}
	args = append([]any{string(encoded)}, args...)
	q += " ORDER BY k,source,identity,number"
	rows, err := d.execer().Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("candidate bodies: %w", err)
	}
	defer rows.Close()
	bodies := []searchBody{}
	for rows.Next() {
		var b searchBody
		var key, id string
		var source Source
		if err := rows.Scan(&key, &source, &id, &b.role, &b.content, &b.timestamp, &b.number); err != nil {
			return nil, err
		}
		b.ref = savedREF(key, source, id)
		bodies = append(bodies, b)
	}
	return bodies, rows.Err()
}

// SearchGroups returns the complete filtered group set. The caller pages only
// after this read, within the same ReadSnapshot used for all related data.
func (d *DB) SearchGroups(candidates SearchCandidates, filter SessionFilter, matcher *PatternMatcher, all bool) ([]SearchGroup, error) {
	owners, err := d.searchOwners()
	if err != nil {
		return nil, err
	}
	if err = d.readOwnerTimes(owners); err != nil {
		return nil, err
	}
	sessions := []SessionRow{}
	byREF := map[string]searchOwner{}
	allowed := map[string]bool{}
	selected := []searchOwner{}
	for _, s := range owners {
		sessions = append(sessions, s.SessionRow)
		byREF[s.REF] = s
		if candidates.accepts(s) {
			allowed[s.REF] = true
			selected = append(selected, s)
		}
	}
	bodies := []searchBody{}
	if matcher != nil || filter.Since != "" || filter.Until != "" {
		bodies, err = d.candidateBodies(selected, filter)
		if err != nil {
			return nil, err
		}
	}
	bodyGroups := map[string][]searchBody{}
	for _, b := range bodies {
		bodyGroups[b.ref] = append(bodyGroups[b.ref], b)
	}
	type orderedGroup struct {
		item SearchGroup
		key  string
	}
	groups := []orderedGroup{}
	for _, g := range resolveSessionGroups(sessions) {
		item := SearchGroup{REF: g.RepresentativeREF, Source: g.Self.Source, Members: []string{}, MatchedMembers: []string{}}
		if g.Root != nil {
			root := byREF[g.Root.REF]
			item.Input = root.input
			item.Project = root.project
			item.Title = root.title
		} else {
			item.Input = byREF[g.Self.REF].input
		}
		seen := []bool{}
		if matcher != nil {
			seen = make([]bool, len(matcher.patterns))
		}
		hasBody := false
		for _, member := range g.Members {
			s := byREF[member.REF]
			item.Members = append(item.Members, s.REF)
			if s.first != nil {
				updateKnownTime(&item.StartedAt, *s.first, false)
			}
			if s.last != nil {
				updateKnownTime(&item.LastAt, *s.last, true)
			}
			if s.imported != nil {
				updateKnownTime(&item.ImportedAt, *s.imported, true)
			}
			if !allowed[s.REF] {
				continue
			}
			item.MatchedMembers = append(item.MatchedMembers, s.REF)
			for _, b := range bodyGroups[s.REF] {
				hasBody = true
				if matcher != nil {
					for i, p := range matcher.patterns {
						if !seen[i] && p.MatchString(b.content) {
							seen[i] = true
						}
					}
				}
			}
		}
		if len(item.MatchedMembers) == 0 {
			continue
		}
		if matcher == nil {
			if (filter.Since != "" || filter.Until != "") && !hasBody {
				continue
			}
		} else {
			matched := false
			if all {
				matched = true
				for _, hit := range seen {
					matched = matched && hit
				}
			} else {
				for _, hit := range seen {
					matched = matched || hit
				}
			}
			if !matched {
				continue
			}
		}
		item.MemberCount = len(item.Members)
		groups = append(groups, orderedGroup{item, g.GroupKey})
	}
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i].item.LastAt, groups[j].item.LastAt
		if a == nil && b != nil {
			return false
		}
		if a != nil && b == nil {
			return true
		}
		if a != nil && b != nil {
			left, _ := time.Parse(time.RFC3339Nano, *a)
			right, _ := time.Parse(time.RFC3339Nano, *b)
			if !left.Equal(right) {
				return left.After(right)
			}
		}
		return groups[i].key < groups[j].key
	})
	result := []SearchGroup{}
	for _, g := range groups {
		result = append(result, g.item)
	}
	return result, nil
}
