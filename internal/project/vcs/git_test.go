package vcs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryLifecycle(t *testing.T) {
	dir := t.TempDir()
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}

	if _, err := repo.Head(); !errors.Is(err, ErrNoCommits) {
		t.Fatalf("a fresh repository should have no commits, got %v", err)
	}

	ctx := context.Background()
	if history, err := repo.History(ctx, 0); err != nil || len(history) != 0 {
		t.Fatalf("empty history = %v, %v", history, err)
	}

	path := filepath.Join(dir, "documents")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("create the documents directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "a.md"), []byte("first version\n"), 0o600); err != nil {
		t.Fatalf("write the first version: %v", err)
	}

	first, err := repo.Commit(ctx, "Write the first version")
	if err != nil {
		t.Fatalf("commit the first version: %v", err)
	}
	if !first.Created || first.Hash == "" {
		t.Fatalf("the first commit = %+v", first)
	}
	if len(first.Files) != 1 || first.Files[0] != "documents/a.md" {
		t.Errorf("changed files = %v", first.Files)
	}

	content, err := repo.ReadFile(ctx, first.Hash, "documents/a.md")
	if err != nil {
		t.Fatalf("read the file: %v", err)
	}
	if content != "first version\n" {
		t.Errorf("content = %q", content)
	}

	// A commit with no change returns the previous revision and does not
	// pretend it created one.
	same, err := repo.Commit(ctx, "Nothing changed")
	if err != nil {
		t.Fatalf("commit without a change: %v", err)
	}
	if same.Created {
		t.Error("an unchanged commit should not report Created")
	}
	if same.Hash != first.Hash {
		t.Errorf("unchanged commit hash = %q, want %q", same.Hash, first.Hash)
	}

	if err := os.WriteFile(filepath.Join(path, "a.md"), []byte("second version\n"), 0o600); err != nil {
		t.Fatalf("write the second version: %v", err)
	}
	second, err := repo.Commit(ctx, "Rewrite the first version")
	if err != nil {
		t.Fatalf("commit the second version: %v", err)
	}
	if second.Parent != first.Hash {
		t.Errorf("parent = %q, want %q", second.Parent, first.Hash)
	}

	history, err := repo.History(ctx, 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(history))
	}
	if history[0].Hash != second.Hash {
		t.Error("the history is not newest first")
	}

	files, err := repo.ListFiles(ctx, first.Hash)
	if err != nil {
		t.Fatalf("list the files: %v", err)
	}
	if len(files) != 1 || files[0] != "documents/a.md" {
		t.Errorf("files = %v", files)
	}

	// Restoring writes the old content back without committing.
	if err := repo.RestoreFile(ctx, first.Hash, "documents/a.md"); err != nil {
		t.Fatalf("restore the file: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "a.md"))
	if err != nil {
		t.Fatalf("read the restored file: %v", err)
	}
	if string(data) != "first version\n" {
		t.Errorf("restored file = %q", string(data))
	}
	if changed, err := repo.HasChanges(); err != nil || !changed {
		t.Errorf("the working tree should have changes: %v, %v", changed, err)
	}
}

func TestReadFileOfAMissingPath(t *testing.T) {
	dir := t.TempDir()
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	ctx := context.Background()
	commit, err := repo.Commit(ctx, "one")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := repo.ReadFile(ctx, commit.Hash, "missing.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("reading a missing file = %v, want ErrNotFound", err)
	}
}

func TestValidatePath(t *testing.T) {
	for _, valid := range []string{"documents/a.md", "a", "documents/sub/b.md"} {
		if err := ValidatePath(valid); err != nil {
			t.Errorf("ValidatePath(%q) = %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "/etc/passwd", "../x", "documents/../../x", "a\\b"} {
		if err := ValidatePath(invalid); err == nil {
			t.Errorf("ValidatePath(%q) should fail", invalid)
		}
	}
}
