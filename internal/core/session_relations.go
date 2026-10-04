package core

import (
	"fmt"
	"sort"
	"strings"
)

// SessionResolution separates grouping membership from confirmed descendants.
// Missing parents are keys only; they never become saved members.
type SessionResolution struct {
	Self              SessionRow
	Parent            *SessionRow
	Root              *SessionRow
	GroupKey          string
	RepresentativeREF string
	Members           []SessionRow
	Descendants       []SessionRow
	Diagnostics       []string
}

// ResolveSession resolves a full REF within its input and source namespace.
// Descendants include Self in parent-first DFS order, with siblings sorted by REF.
// Legacy and Cursor sessions remain independent.
func (d *DB) ResolveSession(ref string) (*SessionResolution, error) {
	self, err := d.LookupSessionREF(ref)
	if err != nil || self == nil {
		return nil, err
	}
	if self.InputID < 0 || self.Source == SourceCursorAgent {
		self.ParentREF, self.RootREF = "", ""
		return &SessionResolution{Self: *self, GroupKey: ref, RepresentativeREF: ref, Members: []SessionRow{*self}, Descendants: []SessionRow{*self}}, nil
	}
	rows, err := d.execer().Query(sessionRowSelect+` WHERE s.input_id=? AND s.source=? GROUP BY s.input_id,s.source,s.identity`, self.InputID, self.Source)
	if err != nil {
		return nil, fmt.Errorf("resolve sessions: %w", err)
	}
	sessions, err := scanSessionRows(rows, "resolve sessions")
	if err != nil {
		return nil, err
	}
	key, _, _, _ := parseREF(ref)
	return resolveSessionRelations(*self, sessions, key), nil
}

func resolveSessionRelations(self SessionRow, sessions []SessionRow, inputKey string) *SessionResolution {
	byID := map[string]SessionRow{}
	parents := map[string]string{}
	for _, s := range sessions {
		byID[s.Identity] = s
		if s.ParentIdentity != "" {
			parents[s.Identity] = s.ParentIdentity
		}
	}
	// Remove every edge participating in a cycle, rather than choosing an
	// arbitrary root from a cycle. Edges into the cycle remain explicit evidence.
	cyclic := map[string]bool{}
	for id := range byID {
		path := []string{}
		seen := map[string]int{}
		for current := id; current != ""; current = parents[current] {
			if at, ok := seen[current]; ok {
				for _, member := range path[at:] {
					cyclic[member] = true
				}
				break
			}
			seen[current] = len(path)
			path = append(path, current)
		}
	}
	result := &SessionResolution{}
	for id := range cyclic {
		delete(parents, id)
		result.Diagnostics = append(result.Diagnostics, "unconfirmed cyclic parent: "+byID[id].REF)
	}
	sort.Strings(result.Diagnostics)
	groupID := func(id string) string {
		if self.Source == SourceClaudeCode {
			if root := byID[id].RootIdentity; root != "" {
				return root
			}
			return id
		}
		for parents[id] != "" {
			id = parents[id]
		}
		return id
	}
	group := groupID(self.Identity)
	result.GroupKey = IdentityREF(inputKey, self.Source, group)
	// Expose only existing, confirmed direct parents; root membership is separate.
	for id, s := range byID {
		s.ParentREF = ""
		s.RootREF = ""
		if p, ok := byID[parents[id]]; ok {
			s.ParentREF = p.REF
		}
		if r, ok := byID[groupID(id)]; ok {
			s.RootREF = r.REF
		}
		byID[id] = s
	}
	result.Self = byID[self.Identity]
	if p, ok := byID[parents[self.Identity]]; ok {
		result.Parent = &p
	}
	if r, ok := byID[group]; ok {
		result.Root = &r
		result.RepresentativeREF = r.REF
	}
	for id, s := range byID {
		if groupID(id) == group {
			result.Members = append(result.Members, s)
		}
	}
	sort.Slice(result.Members, func(i, j int) bool { return result.Members[i].REF < result.Members[j].REF })
	if result.RepresentativeREF == "" {
		result.RepresentativeREF = result.Members[0].REF
	}
	children := map[string][]SessionRow{}
	for id, parent := range parents {
		if _, exists := byID[parent]; exists {
			children[parent] = append(children[parent], byID[id])
		}
	}
	for parent := range children {
		sort.Slice(children[parent], func(i, j int) bool { return children[parent][i].REF < children[parent][j].REF })
	}
	visited := map[string]bool{}
	var visit func(SessionRow)
	visit = func(s SessionRow) {
		if visited[s.Identity] {
			return
		}
		visited[s.Identity] = true
		result.Descendants = append(result.Descendants, s)
		for _, c := range children[s.Identity] {
			visit(c)
		}
	}
	visit(result.Self)
	return result
}

// sessionScopeCondition preserves the full namespace, including legacy snapshots.
func sessionScopeCondition(sessions []SessionRow) (string, []any) {
	conditions := make([]string, 0, len(sessions))
	args := []any{}
	for _, s := range sessions {
		key, _, _, _ := parseREF(s.REF)
		conditions = append(conditions, "(m.input_key=? AND m.source=? AND m.identity=?)")
		args = append(args, key, s.Source, s.Identity)
	}
	if len(conditions) == 0 {
		return "0", args
	}
	return "(" + strings.Join(conditions, " OR ") + ")", args
}
