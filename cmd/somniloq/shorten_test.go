package main

import "testing"

func TestShortenProject_NormalPath(t *testing.T) {
	got := shortenProject("/Users/ryota/Sources/myproject", "-Users-ryota-Sources-myproject")
	if got != "myproject" {
		t.Errorf("got %q, want %q", got, "myproject")
	}
}

func TestShortenProject_EmptyCWD(t *testing.T) {
	got := shortenProject("", "-Users-test-proj")
	if got != "-Users-test-proj" {
		t.Errorf("got %q, want %q", got, "-Users-test-proj")
	}
}

func TestShortenProject_WorktreePath(t *testing.T) {
	got := shortenProject("/Users/ryota/Sources/myproject/.claude/worktrees/xyz", "-Users-ryota-Sources-myproject")
	if got != "myproject" {
		t.Errorf("got %q, want %q", got, "myproject")
	}
}

func TestShortenProject_HyphenatedName(t *testing.T) {
	got := shortenProject("/Users/ryota/Sources/202512-phase2", "-Users-ryota-Sources-202512-phase2")
	if got != "202512-phase2" {
		t.Errorf("got %q, want %q", got, "202512-phase2")
	}
}

func TestShortenProject_TrailingSlash(t *testing.T) {
	got := shortenProject("/Users/ryota/Sources/myproject/", "-Users-ryota-Sources-myproject")
	if got != "myproject" {
		t.Errorf("got %q, want %q", got, "myproject")
	}
}

func TestShortenProject_RootPath(t *testing.T) {
	got := shortenProject("/", "-")
	if got != "/" {
		t.Errorf("got %q, want %q", got, "/")
	}
}
