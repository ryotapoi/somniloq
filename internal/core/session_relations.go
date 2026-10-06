package core

import (
	"fmt"
	"sort"
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
	return d.NewSessionResolver().Resolve(ref, true)
}

type sessionNamespace struct {
	inputID int64
	source  Source
}

// SessionResolver shares namespace relations for one read operation.
// Create it inside ReadSnapshot and discard it when the callback returns.
type SessionResolver struct {
	db         *DB
	namespaces map[sessionNamespace]*sessionRelations
}

func (d *DB) NewSessionResolver() *SessionResolver {
	return &SessionResolver{db: d, namespaces: map[sessionNamespace]*sessionRelations{}}
}

// Resolve preserves full REF validation and optionally expands descendants.
func (r *SessionResolver) Resolve(ref string, descendants bool) (*SessionResolution, error) {
	self, err := r.db.LookupSessionREF(ref)
	if err != nil || self == nil {
		return nil, err
	}
	if self.InputID < 0 || self.Source == SourceCursorAgent {
		self.ParentREF, self.RootREF = "", ""
		result := &SessionResolution{Self: *self, GroupKey: ref, RepresentativeREF: ref, Members: []SessionRow{*self}}
		if descendants {
			result.Descendants = []SessionRow{*self}
		}
		return result, nil
	}
	namespace := sessionNamespace{inputID: self.InputID, source: self.Source}
	g := r.namespaces[namespace]
	if g == nil {
		rows, err := r.db.execer().Query(sessionRowSelect+` WHERE s.input_id=? AND s.source=? GROUP BY s.input_id,s.source,s.identity`, self.InputID, self.Source)
		if err != nil {
			return nil, fmt.Errorf("resolve sessions: %w", err)
		}
		sessions, err := scanSessionRows(rows, "resolve sessions")
		if err != nil {
			return nil, err
		}
		g = buildSessionRelations(sessions)
		r.namespaces[namespace] = g
	}
	key, _, _, _ := parseREF(ref)
	return g.resolve(*self, key, descendants), nil
}

type sessionRelations struct {
	byID        map[string]SessionRow
	parents     map[string]string
	children    map[string][]SessionRow
	groups      map[string][]SessionRow
	groupKeys   map[string]string
	diagnostics []string
}

func buildSessionRelations(sessions []SessionRow) *sessionRelations {
	g := &sessionRelations{byID: map[string]SessionRow{}, parents: map[string]string{}, groups: map[string][]SessionRow{}, groupKeys: map[string]string{}}
	for _, s := range sessions {
		g.byID[s.Identity] = s
		if s.ParentIdentity != "" {
			g.parents[s.Identity] = s.ParentIdentity
		}
	}
	cyclic := map[string]bool{}
	for id := range g.byID {
		path := []string{}
		seen := map[string]int{}
		for current := id; current != ""; current = g.parents[current] {
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
	for id := range cyclic {
		delete(g.parents, id)
		g.diagnostics = append(g.diagnostics, "unconfirmed cyclic parent: "+g.byID[id].REF)
	}
	sort.Strings(g.diagnostics)
	for id, s := range g.byID {
		group := id
		if s.Source == SourceClaudeCode {
			if s.RootIdentity != "" {
				group = s.RootIdentity
			}
		} else {
			for g.parents[group] != "" {
				group = g.parents[group]
			}
		}
		g.groupKeys[id] = group
	}
	for id, s := range g.byID {
		s.ParentREF, s.RootREF = "", ""
		if p, ok := g.byID[g.parents[id]]; ok {
			s.ParentREF = p.REF
		}
		if r, ok := g.byID[g.groupKeys[id]]; ok {
			s.RootREF = r.REF
		}
		g.byID[id] = s
		group := g.groupKeys[id]
		g.groups[group] = append(g.groups[group], s)
	}
	for group := range g.groups {
		sort.Slice(g.groups[group], func(i, j int) bool { return g.groups[group][i].REF < g.groups[group][j].REF })
	}
	return g
}

func (g *sessionRelations) resolve(self SessionRow, inputKey string, descendants bool) *SessionResolution {
	group := g.groupKeys[self.Identity]
	result := &SessionResolution{Self: g.byID[self.Identity], GroupKey: IdentityREF(inputKey, self.Source, group), Members: g.groups[group], Diagnostics: g.diagnostics}
	if p, ok := g.byID[g.parents[self.Identity]]; ok {
		result.Parent = &p
	}
	if r, ok := g.byID[group]; ok {
		result.Root = &r
		result.RepresentativeREF = r.REF
	} else {
		result.RepresentativeREF = result.Members[0].REF
	}
	if !descendants {
		return result
	}
	if g.children == nil {
		g.children = map[string][]SessionRow{}
		for id, parent := range g.parents {
			if _, exists := g.byID[parent]; exists {
				g.children[parent] = append(g.children[parent], g.byID[id])
			}
		}
		for parent := range g.children {
			sort.Slice(g.children[parent], func(i, j int) bool { return g.children[parent][i].REF < g.children[parent][j].REF })
		}
	}
	visited := map[string]bool{}
	var visit func(SessionRow)
	visit = func(s SessionRow) {
		if visited[s.Identity] {
			return
		}
		visited[s.Identity] = true
		result.Descendants = append(result.Descendants, s)
		for _, c := range g.children[s.Identity] {
			visit(c)
		}
	}
	visit(result.Self)
	return result
}

func resolveSessionRelations(self SessionRow, sessions []SessionRow, inputKey string) *SessionResolution {
	return buildSessionRelations(sessions).resolve(self, inputKey, true)
}

// resolveSessionGroups builds each namespace graph once, keeping saved members
// separate from missing parent keys and preserving the single-REF resolver rules.
func resolveSessionGroups(sessions []SessionRow) []*SessionResolution {
	namespaces := map[string][]SessionRow{}
	results := []*SessionResolution{}
	for _, s := range sessions {
		if s.InputID < 0 || s.Source == SourceCursorAgent {
			root := s
			results = append(results, &SessionResolution{Self: s, Root: &root, GroupKey: s.REF, RepresentativeREF: s.REF, Members: []SessionRow{s}})
			continue
		}
		key, _, _, _ := parseREF(s.REF)
		namespace := key + ":" + string(s.Source)
		namespaces[namespace] = append(namespaces[namespace], s)
	}
	for _, rows := range namespaces {
		key, _, _, _ := parseREF(rows[0].REF)
		g := buildSessionRelations(rows)
		for _, members := range g.groups {
			results = append(results, g.resolve(members[0], key, false))
		}
	}
	return results
}
