package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// unsetAllGitEnv は GIT_ プレフィックスの全環境変数を一時的に unset する。
// GIT_DIR / GIT_WORK_TREE / GIT_CEILING_DIRECTORIES 等が rev-parse --show-toplevel
// に影響するため、テストの決定性を確保するために列挙ではなく走査で unset する。
// t.Setenv("GIT_DIR", "") は空文字が "fatal: not a git repository: ”" を誘発
// するため使わない。
func unsetAllGitEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := kv[:eq]
		if !strings.HasPrefix(key, "GIT_") {
			continue
		}
		orig, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("os.Unsetenv(%q): %v", key, err)
		}
		k := key
		v := orig
		t.Cleanup(func() {
			_ = os.Setenv(k, v)
		})
	}
}

// TestResolveRepoPath_Empty は仕様 1 の早期 return を担保する。
// git -C "" rev-parse を起動させない意図は実装側のコメントで保証する。
func TestResolveRepoPath_Empty(t *testing.T) {
	if got := ResolveRepoPath(""); got != "" {
		t.Errorf("ResolveRepoPath(\"\") = %q, want empty", got)
	}
}

func TestResolveRepoPath_Worktree(t *testing.T) {
	// 仕様 2 が仕様 3（git 経路）より優先されることを固定化する。GIT_* を
	// unset した上で、存在しない cwd を渡しても worktree prefix が返ること
	// （= git 経路を通っていないこと）で担保する。
	unsetAllGitEnv(t)

	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{
			name: "worktree root",
			cwd:  "/Users/foo/repo/.claude/worktrees/feat-x",
			want: "/Users/foo/repo",
		},
		{
			name: "worktree subdirectory resolves to worktree host",
			cwd:  "/Users/foo/repo/.claude/worktrees/feat-x/sub/dir",
			want: "/Users/foo/repo",
		},
		{
			// strings.Index 固定: LastIndex への変更で壊れる病的入力。
			name: "multiple fragments cut at first occurrence",
			cwd:  "/foo/.claude/worktrees/x/.claude/worktrees/y",
			want: "/foo",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveRepoPath(tc.cwd); got != tc.want {
				t.Errorf("ResolveRepoPath(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}

	// 文字列処理の回帰防止: 類似文字列 (/.claude/worktreesXYZ/) は worktree マッチに
	// 切れないこと。外側の unsetAllGitEnv と存在しない cwd の組み合わせで、
	// git 経路も空文字を返すため結果は "" に固定できる。
	t.Run("similar but not exact does not match", func(t *testing.T) {
		cwd := "/foo/bar/.claude/worktreesXYZ/baz"
		if got := ResolveRepoPath(cwd); got != "" {
			t.Errorf("ResolveRepoPath(%q) = %q, want empty", cwd, got)
		}
	})
}

// t.Parallel は使わない。os.Unsetenv はプロセスグローバルで、
// 並列化すると GIT_* 復元中の他テストが影響を受けうる。
func TestResolveRepoPath_GitToplevel(t *testing.T) {
	unsetAllGitEnv(t)

	dir := t.TempDir()
	// macOS の t.TempDir は /var/folders/... のシンボリックリンク経由。
	// git rev-parse --show-toplevel は実パスを返すので期待値も実パス化する。
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}

	// CI ホームの .gitconfig に safe.directory 制限があっても通るよう
	// テスト側の git init のみ -c safe.directory=* を付ける。
	// 本体コード側は付けない（プラン参照）。
	cmd := exec.Command("git", "-c", "safe.directory=*", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("Mkdir(%q): %v", sub, err)
	}

	tests := []struct {
		name string
		cwd  string
	}{
		{"toplevel direct", dir},
		{"subdirectory resolves to root", sub},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveRepoPath(tc.cwd); got != want {
				t.Errorf("ResolveRepoPath(%q) = %q, want %q", tc.cwd, got, want)
			}
		})
	}
}

// TestResolveRepoPath_GitToplevel_PreservesTrailingSpace は git rev-parse の
// 出力から TrimSpace ではなく TrimRight("\r\n") のみを行うことを担保する。
// ディレクトリ名末尾に空白を含むリポジトリでパスが壊れないこと。
func TestResolveRepoPath_GitToplevel_PreservesTrailingSpace(t *testing.T) {
	unsetAllGitEnv(t)

	parent := t.TempDir()
	repo := filepath.Join(parent, "trail ") // 末尾スペース入り
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("Mkdir(%q): %v", repo, err)
	}
	want, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", repo, err)
	}

	cmd := exec.Command("git", "-c", "safe.directory=*", "-C", repo, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	if got := ResolveRepoPath(repo); got != want {
		t.Errorf("ResolveRepoPath(%q) = %q, want %q (trailing space must survive)", repo, got, want)
	}
}

func TestResolveRepoPath_NotGitRepo(t *testing.T) {
	unsetAllGitEnv(t)

	// 非 git ディレクトリ（rev-parse 非 0 終了パス）。
	nonGit := t.TempDir()
	// 実在しないパス（chdir 失敗パス）。
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	tests := []struct {
		name string
		cwd  string
	}{
		{"non-git directory", nonGit},
		{"nonexistent path", missing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveRepoPath(tc.cwd); got != "" {
				t.Errorf("ResolveRepoPath(%q) = %q, want empty", tc.cwd, got)
			}
		})
	}
}
