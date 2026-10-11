package claudecode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type Adapter struct {
	resolveRepoPath ingest.RepoResolver
}

func NewAdapter(resolveRepoPath ingest.RepoResolver) Adapter {
	return Adapter{resolveRepoPath: resolveRepoPath}
}

func (a Adapter) ScanFiles(projectsDir string) ([]string, []error) {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("scan %s: %w", projectsDir, err)}
	}

	var files []string
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projDir := entry.Name()
		subPath := filepath.Join(projectsDir, projDir)
		subEntries, err := os.ReadDir(subPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("scan %s: %w", subPath, err))
			continue
		}
		for _, se := range subEntries {
			if se.IsDir() {
				agents := filepath.Join(subPath, se.Name(), "subagents")
				entries, err := os.ReadDir(agents)
				if err != nil {
					if !errors.Is(err, os.ErrNotExist) {
						errs = append(errs, fmt.Errorf("scan %s: %w", agents, err))
					}
					continue
				}
				for _, e := range entries {
					if !e.IsDir() && strings.HasPrefix(e.Name(), "agent-") && strings.HasSuffix(e.Name(), ".jsonl") {
						files = append(files, filepath.Join(agents, e.Name()))
					}
				}
				continue
			}
			name := se.Name()
			if !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			files = append(files, filepath.Join(subPath, name))
		}
	}
	sort.Strings(files)
	return files, errs
}
