package core

import (
	"io"
	"os/exec"
	"strings"
)

// worktreePathFragment は cwd（実パス）内の Claude Code worktree マーカー。
// cmd/somniloq/shorten.go の projectWorktreeMarker とは別物（あちらは
// project_dir のハイフン置換済みキー "--claude-worktrees-" 用で値自体が異なる）。
const worktreePathFragment = "/.claude/worktrees/"

// ResolveRepoPath は Claude Code JSONL の cwd から実リポジトリパスを解決する。
//
// 評価順:
//  1. cwd が空 → 空文字
//  2. cwd が worktreePathFragment を含む → その直前までの絶対パス
//  3. git -C <cwd> rev-parse --show-toplevel の成功出力
//  4. どれも失敗 → cwd をそのまま返す（git 配下外でも cwd を一意キーとして採用するため）
//
// 空 cwd は最初に弾くこと。git -C "" rev-parse はカレントディレクトリ扱いで
// 呼び出し元プロセスの実リポジトリを誤って引いてしまうため。
func ResolveRepoPath(cwd string) string {
	if cwd == "" {
		return ""
	}
	// 複数回出現する病的入力は最初の出現位置で切る（strings.Index）。
	// LastIndex への変更を避ける意図をテストで固定化している。
	if i := strings.Index(cwd, worktreePathFragment); i >= 0 {
		return cwd[:i]
	}
	// 引数順「git -C <cwd> rev-parse ...」は変更しないこと。<cwd> が -C の直後に
	// 来るから位置引数として安全に渡る。別順（例: git rev-parse -C <cwd>）に
	// すると <cwd> の先頭が '-' のときオプションとして解釈される可能性がある。
	cmd := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel")
	// Output() は Stderr が nil だと *exec.ExitError.Stderr 用に内部バッファへ
	// 溜める（親の stderr には流さない）。バックフィルで多数回呼ばれるため
	// io.Discard を明示して捨てる。副作用として *exec.ExitError.Stderr は空に
	// なる。将来失敗理由をログに出したくなったら bytes.Buffer に変えること。
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		// git 失敗は cwd 返却に集約（仕様 4）。
		return cwd
	}
	// git rev-parse --show-toplevel は末尾に改行を付ける。先頭/末尾の空白を
	// 含む正当なパス（例: "/tmp/dir with trailing space / ..."）を壊さないよう
	// 改行のみを剥がす（TrimSpace は使わない）。
	return strings.TrimRight(string(out), "\r\n")
}
