package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/incurl/Markuccino/internal/pocketbase"
)

// newTestStore creates a writing store backed by a throwaway database and
// data directory.
func newTestStore(t *testing.T) *Store {
	t.Helper()

	dir := t.TempDir()
	service := pocketbase.NewService(dir)
	if err := service.Bootstrap(); err != nil {
		t.Fatalf("bootstrap pocketbase: %v", err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close pocketbase: %v", err)
		}
	})

	return NewStore(service.App(), dir, nil)
}

// findItem finds an item by title.
func findItem(t *testing.T, items []Item, title string) Item {
	t.Helper()
	for _, item := range items {
		if item.Title == title {
			return item
		}
	}
	t.Fatalf("no item titled %q in %d items", title, len(items))
	return Item{}
}

// mustItems returns the workspace items, failing the test on error.
func mustItems(t *testing.T, store *Store, projectID string) []Item {
	t.Helper()
	items, err := store.Items(projectID)
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	return items
}

// TestCreateProject verifies the three binder roots and the initial
// commit.
func TestCreateProject(t *testing.T) {
	store := newTestStore(t)

	project, err := store.CreateProject("The Long Winter")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.Name != "The Long Winter" {
		t.Errorf("name = %q", project.Name)
	}
	if project.Slug != "the-long-winter" {
		t.Errorf("slug = %q", project.Slug)
	}

	workspace, err := store.Workspace(project.ID)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if len(workspace.Items) != 3 {
		t.Fatalf("expected 3 binder roots, got %d", len(workspace.Items))
	}
	for _, want := range []string{DraftRootTitle, ResearchRootTitle, TrashRootTitle} {
		findItem(t, workspace.Items, want)
	}

	history, err := store.History(project.ID, 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected one initial revision, got %d", len(history))
	}
	if history[0].Message != "Create project \u201cThe Long Winter\u201d" {
		t.Errorf("initial commit message = %q", history[0].Message)
	}

	// The workspace is a real Git repository and a real directory.
	root := filepath.Join(store.dataDir, "projects", project.ID)
	if info, statErr := os.Stat(filepath.Join(root, ".git")); statErr != nil || !info.IsDir() {
		t.Errorf("the project has no .git directory: %v", statErr)
	}
	if info, statErr := os.Stat(filepath.Join(root, documentsDir)); statErr != nil || !info.IsDir() {
		t.Errorf("the project has no documents directory: %v", statErr)
	}
}

// TestDocumentRoundTrip verifies that a body survives a save, a read, a
// count and a commit.
func TestDocumentRoundTrip(t *testing.T) {
	store := newTestStore(t)
	project, err := store.CreateProject("Round Trip")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	draft := findItem(t, mustItems(t, store, project.ID), DraftRootTitle)
	document, err := store.CreateItem(project.ID, draft.ID, KindDocument, "Chapter One")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	if document.FilePath == "" {
		t.Fatal("the document has no file path")
	}

	content := "# Chapter One\n\nIt was a **dark** and stormy night.\n"
	saved, err := store.SaveDocument(document.ID, content)
	if err != nil {
		t.Fatalf("save document: %v", err)
	}
	if saved.Revision == "" {
		t.Error("the save did not return a revision")
	}

	read, err := store.ReadDocument(document.ID)
	if err != nil {
		t.Fatalf("read document: %v", err)
	}
	if read.Content != content {
		t.Errorf("content = %q, want %q", read.Content, content)
	}
	if read.Item.WordCount != 9 {
		t.Errorf("word count = %d, want 9", read.Item.WordCount)
	}

	// The file is on disk where the record says it is.
	target := filepath.Join(store.dataDir, "projects", project.ID, filepath.FromSlash(document.FilePath))
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read the file: %v", readErr)
	}
	if string(data) != content {
		t.Errorf("the file on disk = %q", string(data))
	}

	// The project's word count includes the draft and excludes research.
	updated, err := store.GetProject(project.ID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if updated.WordCount != 9 {
		t.Errorf("draft word count = %d, want 9", updated.WordCount)
	}
}

// TestSnapshotAndRestore verifies that a snapshot is a Git commit and
// that restoring writes the old content back.
func TestSnapshotAndRestore(t *testing.T) {
	store := newTestStore(t)
	project, err := store.CreateProject("Snapshots")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	draft := findItem(t, mustItems(t, store, project.ID), DraftRootTitle)
	document, err := store.CreateItem(project.ID, draft.ID, KindDocument, "Opening")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	first := "The first version.\n"
	if _, err := store.SaveDocument(document.ID, first); err != nil {
		t.Fatalf("save the first version: %v", err)
	}
	snapshot, err := store.CreateSnapshot(project.ID, document.ID, "Before the rewrite")
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snapshot.CommitHash == "" {
		t.Fatal("the snapshot has no commit hash")
	}
	if snapshot.Message != "Before the rewrite" {
		t.Errorf("snapshot message = %q", snapshot.Message)
	}

	second := "The second version, which is longer.\n"
	if _, err := store.SaveDocument(document.ID, second); err != nil {
		t.Fatalf("save the second version: %v", err)
	}

	diff, err := store.DiffRevision(project.ID, document.ID, snapshot.CommitHash)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if diff.Text == "" {
		t.Error("the diff of a changed revision is empty")
	}

	restored, err := store.RestoreDocument(document.ID, snapshot.CommitHash)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Content != first {
		t.Errorf("restored content = %q, want %q", restored.Content, first)
	}

	// The restore is itself a revision: the history stays append-only.
	history, err := store.History(project.ID, 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) < 4 {
		t.Errorf("expected at least 4 revisions, got %d", len(history))
	}

	snapshots, err := store.ListSnapshots(project.ID, document.ID, 0)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snapshots) < 2 {
		t.Errorf("expected at least 2 snapshots, got %d", len(snapshots))
	}
}
