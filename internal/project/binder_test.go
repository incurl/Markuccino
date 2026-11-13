package project

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestMoveIntoTheTrashRemembersTheOrigin verifies that dragging an item
// onto the Trash behaves like the Trash button.
func TestMoveIntoTheTrashRemembersTheOrigin(t *testing.T) {
	store := newTestStore(t)
	project, err := store.CreateProject("Drag to trash")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	items := mustItems(t, store, project.ID)
	draft := findItem(t, items, DraftRootTitle)
	trash := findItem(t, items, TrashRootTitle)

	document, err := store.CreateItem(project.ID, draft.ID, KindDocument, "Scene")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	trashed, err := store.MoveItem(document.ID, trash.ID, 0)
	if err != nil {
		t.Fatalf("move into the trash: %v", err)
	}
	if !trashed.Trashed || trashed.Bucket != BucketTrash {
		t.Fatalf("the item was not trashed: %+v", trashed)
	}

	restored, err := store.RestoreItem(document.ID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.ParentID != draft.ID {
		t.Errorf("restored parent = %q, want the draft root %q", restored.ParentID, draft.ID)
	}
}

// TestNotifierFiresOnAStructuralChange verifies the event the frontend
// listens for actually fires.
func TestNotifierFiresOnAStructuralChange(t *testing.T) {
	store := newTestStore(t)

	var mu sync.Mutex
	var projects []string
	store.SetNotifier(func(projectID string) {
		mu.Lock()
		defer mu.Unlock()
		projects = append(projects, projectID)
	})

	project, err := store.CreateProject("Notify")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	draft := findItem(t, mustItems(t, store, project.ID), DraftRootTitle)
	if _, err := store.CreateItem(project.ID, draft.ID, KindDocument, "Chapter"); err != nil {
		t.Fatalf("create document: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(projects) == 0 {
		t.Fatal("the notifier never fired")
	}
	for _, id := range projects {
		if id != project.ID {
			t.Errorf("the notifier reported the project %q, want %q", id, project.ID)
		}
	}
}

// TestMoveTrashRestore walks the full binder lifecycle: move a document
// into a folder, trash the folder, restore it, then delete it for good.
func TestMoveTrashRestore(t *testing.T) {
	store := newTestStore(t)
	project, err := store.CreateProject("Binder")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	draft := findItem(t, mustItems(t, store, project.ID), DraftRootTitle)

	folder, err := store.CreateItem(project.ID, draft.ID, KindFolder, "Part One")
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	document, err := store.CreateItem(project.ID, draft.ID, KindDocument, "Chapter One")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	if _, err := store.SaveDocument(document.ID, "Some words in the chapter.\n"); err != nil {
		t.Fatalf("save document: %v", err)
	}

	moved, err := store.MoveItem(document.ID, folder.ID, 0)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moved.ParentID != folder.ID {
		t.Errorf("parent = %q, want %q", moved.ParentID, folder.ID)
	}
	if moved.Position != 0 {
		t.Errorf("position = %d, want 0", moved.Position)
	}

	// A folder cannot be moved into its own subtree.
	if _, err := store.MoveItem(folder.ID, document.ID, 0); err == nil {
		t.Error("moving a folder into its own document should fail")
	}

	trashed, err := store.TrashItem(folder.ID)
	if err != nil {
		t.Fatalf("trash: %v", err)
	}
	if !trashed.Trashed || trashed.Bucket != BucketTrash {
		t.Errorf("the trashed folder has bucket %q and trashed %v", trashed.Bucket, trashed.Trashed)
	}
	// The child follows the folder into the trash.
	child, err := store.GetItem(document.ID)
	if err != nil {
		t.Fatalf("get the child: %v", err)
	}
	if !child.Trashed {
		t.Error("the child of a trashed folder is not trashed")
	}
	// A trashed document is not readable as a draft.
	if _, err := store.ReadDocument(document.ID); err == nil {
		t.Error("a trashed document should not be readable")
	}
	if _, err := store.SaveDocument(document.ID, "edited in the trash\n"); err == nil {
		t.Error("a trashed document should not be writable")
	}

	restored, err := store.RestoreItem(folder.ID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Trashed || restored.Bucket != BucketDraft {
		t.Errorf("the restored folder has bucket %q and trashed %v", restored.Bucket, restored.Trashed)
	}
	child, err = store.GetItem(document.ID)
	if err != nil {
		t.Fatalf("get the restored child: %v", err)
	}
	if child.Trashed || child.Bucket != BucketDraft {
		t.Errorf("the restored child has bucket %q and trashed %v", child.Bucket, child.Trashed)
	}
	if child.ParentID != folder.ID {
		t.Errorf("the restored child's parent = %q, want %q", child.ParentID, folder.ID)
	}

	// A document that is not trashed cannot be deleted permanently.
	if err := store.DeleteItem(document.ID); err == nil {
		t.Error("deleting a non-trashed item should fail")
	}

	path := filepath.Join(store.dataDir, "projects", project.ID, filepath.FromSlash(document.FilePath))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the document file is missing before the delete: %v", err)
	}

	if _, err := store.TrashItem(document.ID); err != nil {
		t.Fatalf("trash the document: %v", err)
	}
	if err := store.DeleteItem(document.ID); err != nil {
		t.Fatalf("delete the document: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the document file survived the delete: %v", err)
	}
	if _, err := store.GetItem(document.ID); err == nil {
		t.Error("the deleted record is still readable")
	}
}

// TestDuplicateItem verifies that a copy carries the body and the
// metadata and gets its own file.
func TestDuplicateItem(t *testing.T) {
	store := newTestStore(t)
	project, err := store.CreateProject("Duplicates")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	draft := findItem(t, mustItems(t, store, project.ID), DraftRootTitle)
	document, err := store.CreateItem(project.ID, draft.ID, KindDocument, "Scene")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	if _, err := store.SaveDocument(document.ID, "The body of the scene.\n"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := store.UpdateItem(document.ID, ItemMeta{
		Title:    "Scene",
		Synopsis: "A synopsis",
		Keywords: []string{"rain", "night"},
		Label:    "Chapter",
		Status:   "First Draft",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	copyItem, err := store.DuplicateItem(document.ID)
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if copyItem.ID == document.ID {
		t.Fatal("the duplicate has the same id")
	}
	if copyItem.Title != "Scene copy" {
		t.Errorf("title = %q, want %q", copyItem.Title, "Scene copy")
	}
	if copyItem.Synopsis != "A synopsis" || copyItem.Label != "Chapter" {
		t.Errorf("the metadata was not copied: %+v", copyItem)
	}
	if len(copyItem.Keywords) != 2 {
		t.Errorf("keywords = %v", copyItem.Keywords)
	}
	copied, err := store.ReadDocument(copyItem.ID)
	if err != nil {
		t.Fatalf("read the copy: %v", err)
	}
	if copied.Content != "The body of the scene.\n" {
		t.Errorf("copied content = %q", copied.Content)
	}
	if copyItem.FilePath == document.FilePath {
		t.Error("the copy shares the original's file")
	}
}
