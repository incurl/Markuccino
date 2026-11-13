package project

import (
	"fmt"
	"os"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// toItem converts a binder record into the frontend shape.
func toItem(record *core.Record) Item {
	return Item{
		ID:               record.Id,
		ProjectID:        record.GetString("project"),
		ParentID:         record.GetString("parent"),
		Kind:             record.GetString("kind"),
		Bucket:           record.GetString("bucket"),
		Title:            record.GetString("title"),
		FilePath:         record.GetString("file_path"),
		Position:         record.GetInt("position"),
		Synopsis:         record.GetString("synopsis"),
		Notes:            record.GetString("notes"),
		Keywords:         decodeStrings(record, "keywords"),
		Label:            record.GetString("label"),
		Status:           record.GetString("status"),
		IncludeInCompile: record.GetBool("include_in_compile"),
		WordCount:        record.GetInt("word_count"),
		CharCount:        record.GetInt("char_count"),
		CustomMeta:       decodeMap(record, "custom_meta"),
		LastCommit:       record.GetString("last_commit"),
		Trashed:          record.GetBool("trashed"),
		Created:          formatTime(record, "created"),
		Updated:          formatTime(record, "updated"),
	}
}

// itemRecord loads one binder item.
func (s *Store) itemRecord(itemID string) (*core.Record, error) {
	if strings.TrimSpace(itemID) == "" {
		return nil, fmt.Errorf("project: an item id is required")
	}
	record, err := s.app.FindRecordById(CollectionItems, itemID)
	if err != nil {
		return nil, fmt.Errorf("find the binder item %q: %w", itemID, err)
	}
	return record, nil
}

// GetItem returns one binder item.
func (s *Store) GetItem(itemID string) (Item, error) {
	record, err := s.itemRecord(itemID)
	if err != nil {
		return Item{}, err
	}
	return toItem(record), nil
}

// CreateItem adds an item below parentID. An empty parentID means the
// draft root, which is what the binder's "new chapter" button uses.
func (s *Store) CreateItem(projectID, parentID, kind, title string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if kind != KindFolder && kind != KindDocument {
		return Item{}, fmt.Errorf("project: unknown item kind %q", kind)
	}

	parent, err := s.resolveParent(projectID, parentID)
	if err != nil {
		return Item{}, err
	}
	parentID = ""
	bucket := BucketDraft
	if parent != nil {
		parentID = parent.Id
		bucket = parent.GetString("bucket")
		if parent.GetString("kind") != KindFolder {
			return Item{}, fmt.Errorf("project: a document cannot contain another item")
		}
	}
	if bucket == "" {
		bucket = BucketDraft
	}

	position, err := s.nextPosition(projectID, parentID)
	if err != nil {
		return Item{}, err
	}

	fallback := DefaultDocumentTitle
	if kind == KindFolder {
		fallback = DefaultFolderTitle
	}
	item, err := s.createItem(projectID, parentID, bucket, kind, CleanTitle(title, fallback), position)
	if err != nil {
		return Item{}, err
	}

	message := "Create " + kind + " \u201c" + item.Title + "\u201d"
	if _, err := s.commit(projectID, message, TriggerStructural, item.ID); err != nil {
		return Item{}, err
	}
	s.audit(projectID, "binder", "create", ActorUser, "item", item.ID,
		map[string]any{"kind": kind, "title": item.Title, "parent": parentID})
	s.notifyBinder(projectID)
	return item, nil
}

// createItem inserts the record and, for a document, its Markdown file.
//
// The caller holds the mutex. The record is saved before the file is
// written, because the file name is the record id.
func (s *Store) createItem(projectID, parentID, bucket, kind, title string, position int) (Item, error) {
	collection, err := s.app.FindCollectionByNameOrId(CollectionItems)
	if err != nil {
		return Item{}, fmt.Errorf("find the binder collection: %w", err)
	}

	record := core.NewRecord(collection)
	record.Set("project", projectID)
	if parentID != "" {
		record.Set("parent", parentID)
	}
	record.Set("kind", kind)
	record.Set("bucket", bucket)
	record.Set("title", title)
	record.Set("position", position)
	record.Set("keywords", []string{})
	record.Set("custom_meta", map[string]any{})
	record.Set("include_in_compile", bucket == BucketDraft)
	// The Trash root is a container, not a trashed item: it must accept
	// moves and restores. Trashed state is applied by TrashItem, by a
	// move into the Trash, or by a duplicate of a trashed subtree.
	record.Set("trashed", false)
	if err := s.app.Save(record); err != nil {
		return Item{}, fmt.Errorf("create the binder item: %w", err)
	}

	if kind == KindDocument {
		rel := documentRelPath(record.Id)
		target, pathErr := s.absolute(projectID, rel)
		if pathErr != nil {
			return Item{}, pathErr
		}
		if err := writeFileAtomic(target, ""); err != nil {
			return Item{}, err
		}
		record.Set("file_path", rel)
		if err := s.app.Save(record); err != nil {
			return Item{}, fmt.Errorf("store the document path: %w", err)
		}
	}

	return toItem(record), nil
}

// resolveParent loads the parent item, or returns the draft root when the
// caller passed no parent.
func (s *Store) resolveParent(projectID, parentID string) (*core.Record, error) {
	if parentID != "" {
		record, err := s.itemRecord(parentID)
		if err != nil {
			return nil, err
		}
		if record.GetString("project") != projectID {
			return nil, fmt.Errorf("project: the parent belongs to another project")
		}
		if record.GetBool("trashed") {
			return nil, fmt.Errorf("project: a trashed folder cannot receive an item")
		}
		return record, nil
	}

	return s.rootRecord(projectID, BucketDraft)
}

// rootRecord returns the top-level group for a bucket.
func (s *Store) rootRecord(projectID, bucket string) (*core.Record, error) {
	record, err := s.app.FindFirstRecordByFilter(CollectionItems,
		"project = {:project} && bucket = {:bucket} && parent = \"\"",
		dbx.Params{"project": projectID, "bucket": bucket})
	if err != nil {
		return nil, fmt.Errorf("find the %s root: %w", bucket, err)
	}
	return record, nil
}

// siblingRecords returns the children of parentID, ordered.
func (s *Store) siblingRecords(projectID, parentID string) ([]*core.Record, error) {
	filter := "project = {:project} && parent = {:parent}"
	params := dbx.Params{"project": projectID, "parent": parentID}
	if parentID == "" {
		filter = "project = {:project} && parent = \"\""
		params = dbx.Params{"project": projectID}
	}
	records, err := s.app.FindRecordsByFilter(CollectionItems, filter, "position,created", 0, 0, params)
	if err != nil {
		return nil, fmt.Errorf("list the siblings: %w", err)
	}
	return records, nil
}

// nextPosition returns the position after the last sibling.
func (s *Store) nextPosition(projectID, parentID string) (int, error) {
	siblings, err := s.siblingRecords(projectID, parentID)
	if err != nil {
		return 0, err
	}
	position := 0
	for _, sibling := range siblings {
		if sibling.GetInt("position") >= position {
			position = sibling.GetInt("position") + 1
		}
	}
	return position, nil
}

// renumber writes 0..n-1 into the position of the given records.
func (s *Store) renumber(records []*core.Record) error {
	for index, record := range records {
		if record.GetInt("position") == index {
			continue
		}
		record.Set("position", index)
		if err := s.app.Save(record); err != nil {
			return fmt.Errorf("reorder the binder: %w", err)
		}
	}
	return nil
}

// removeRecord returns records without the one with id.
func removeRecord(records []*core.Record, id string) []*core.Record {
	filtered := make([]*core.Record, 0, len(records))
	for _, record := range records {
		if record.Id != id {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

// RenameItem changes an item's title.
//
// The Markdown file is named after the record id, so a rename never moves
// a file; it only changes the label the binder shows.
func (s *Store) RenameItem(itemID, title string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.itemRecord(itemID)
	if err != nil {
		return Item{}, err
	}
	fallback := DefaultDocumentTitle
	if record.GetString("kind") == KindFolder {
		fallback = DefaultFolderTitle
	}
	title = CleanTitle(title, fallback)
	if title == record.GetString("title") {
		return toItem(record), nil
	}

	record.Set("title", title)
	if err := s.app.Save(record); err != nil {
		return Item{}, fmt.Errorf("rename the item: %w", err)
	}

	projectID := record.GetString("project")
	if _, err := s.commit(projectID, "Rename \u201c"+title+"\u201d", TriggerStructural, itemID); err != nil {
		return Item{}, err
	}
	s.audit(projectID, "binder", "rename", ActorUser, "item", itemID, map[string]any{"title": title})
	s.notifyBinder(projectID)
	return toItem(record), nil
}

// UpdateItem writes the inspector's metadata back to the record.
//
// The body is not touched here: it belongs to SaveDocument, which also
// updates the counts and the history. Metadata lives in PocketBase, so
// changing it does not create a Git commit.
func (s *Store) UpdateItem(itemID string, meta ItemMeta) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.itemRecord(itemID)
	if err != nil {
		return Item{}, err
	}
	fallback := DefaultDocumentTitle
	if record.GetString("kind") == KindFolder {
		fallback = DefaultFolderTitle
	}
	if meta.Keywords == nil {
		meta.Keywords = []string{}
	}
	if meta.CustomMeta == nil {
		meta.CustomMeta = map[string]any{}
	}

	record.Set("title", CleanTitle(meta.Title, fallback))
	record.Set("synopsis", meta.Synopsis)
	record.Set("notes", meta.Notes)
	record.Set("keywords", meta.Keywords)
	record.Set("label", meta.Label)
	record.Set("status", meta.Status)
	record.Set("include_in_compile", meta.IncludeInCompile)
	record.Set("custom_meta", meta.CustomMeta)
	if err := s.app.Save(record); err != nil {
		return Item{}, fmt.Errorf("update the item: %w", err)
	}

	projectID := record.GetString("project")
	s.audit(projectID, "binder", "update", ActorUser, "item", itemID,
		map[string]any{"title": record.GetString("title")})
	s.notifyBinder(projectID)
	return toItem(record), nil
}

// MoveItem moves an item below parentID at the given index.
//
// An empty parentID means the draft root. The index is the position among
// the target's children after the item has been removed from its old
// place, which is what a drag-and-drop gestures computes.
func (s *Store) MoveItem(itemID, parentID string, index int) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.itemRecord(itemID)
	if err != nil {
		return Item{}, err
	}
	projectID := record.GetString("project")
	if record.GetString("parent") == "" {
		return Item{}, fmt.Errorf("project: a top-level group cannot be moved")
	}

	target, err := s.resolveParent(projectID, parentID)
	if err != nil {
		return Item{}, err
	}
	if target.GetString("kind") != KindFolder {
		return Item{}, fmt.Errorf("project: an item can only be moved into a folder")
	}
	if target.Id == itemID {
		return Item{}, fmt.Errorf("project: an item cannot be moved into itself")
	}
	if descendant, err := s.isDescendant(projectID, target.Id, itemID); err != nil {
		return Item{}, err
	} else if descendant {
		return Item{}, fmt.Errorf("project: an item cannot be moved into its own subtree")
	}

	oldParent := record.GetString("parent")
	oldSiblings, err := s.siblingRecords(projectID, oldParent)
	if err != nil {
		return Item{}, err
	}
	if err := s.renumber(removeRecord(oldSiblings, itemID)); err != nil {
		return Item{}, err
	}

	newSiblings, err := s.siblingRecords(projectID, target.Id)
	if err != nil {
		return Item{}, err
	}
	newSiblings = removeRecord(newSiblings, itemID)

	// Save the new parent before the renumber pass, so the record is
	// written once with its final place. Moving into the Trash bucket is
	// a delete even though the Trash root itself is not a trashed item.
	wasTrashed := record.GetBool("trashed")
	targetBucket := target.GetString("bucket")
	targetTrashed := target.GetBool("trashed") || targetBucket == BucketTrash
	record.Set("parent", target.Id)
	record.Set("bucket", targetBucket)
	record.Set("trashed", targetTrashed)

	// Dragging into the Trash is a delete, so it remembers where the item
	// came from, exactly like TrashItem. Dragging out of it clears the
	// memory: the item is not a restored one any more.
	if targetBucket == BucketTrash && !wasTrashed {
		record.Set("original_parent", oldParent)
		record.Set("original_position", record.GetInt("position"))
	}
	if targetBucket != BucketTrash {
		record.Set("original_parent", "")
		record.Set("original_position", 0)
	}
	if err := s.app.Save(record); err != nil {
		return Item{}, fmt.Errorf("move the item: %w", err)
	}
	if err := s.setSubtreeBucket(projectID, itemID, targetBucket, targetTrashed); err != nil {
		return Item{}, err
	}

	if index < 0 {
		index = 0
	}
	if index > len(newSiblings) {
		index = len(newSiblings)
	}
	ordered := make([]*core.Record, 0, len(newSiblings)+1)
	ordered = append(ordered, newSiblings[:index]...)
	ordered = append(ordered, record)
	ordered = append(ordered, newSiblings[index:]...)
	if err := s.renumber(ordered); err != nil {
		return Item{}, err
	}

	if _, err := s.commit(projectID, "Move \u201c"+record.GetString("title")+"\u201d", TriggerStructural, itemID); err != nil {
		return Item{}, err
	}
	s.audit(projectID, "binder", "move", ActorUser, "item", itemID,
		map[string]any{"parent": target.Id, "index": index})
	s.notifyBinder(projectID)
	return toItem(record), nil
}

// TrashItem moves an item and its subtree into the Trash.
//
// It remembers where the item came from, so Restore can put it back. The
// Markdown file stays where it is: the bucket is metadata, and moving a
// deleted chapter's file would make the restore a file operation too.
func (s *Store) TrashItem(itemID string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.itemRecord(itemID)
	if err != nil {
		return Item{}, err
	}
	projectID := record.GetString("project")
	if record.GetString("parent") == "" {
		return Item{}, fmt.Errorf("project: a top-level group cannot be trashed")
	}
	if record.GetBool("trashed") {
		return toItem(record), nil
	}

	trashRoot, err := s.rootRecord(projectID, BucketTrash)
	if err != nil {
		return Item{}, err
	}

	oldSiblings, err := s.siblingRecords(projectID, record.GetString("parent"))
	if err != nil {
		return Item{}, err
	}
	if err := s.renumber(removeRecord(oldSiblings, itemID)); err != nil {
		return Item{}, err
	}

	record.Set("original_parent", record.GetString("parent"))
	record.Set("original_position", record.GetInt("position"))
	record.Set("parent", trashRoot.Id)
	record.Set("bucket", BucketTrash)
	record.Set("trashed", true)
	if err := s.app.Save(record); err != nil {
		return Item{}, fmt.Errorf("move the item to the trash: %w", err)
	}
	if err := s.setSubtreeBucket(projectID, itemID, BucketTrash, true); err != nil {
		return Item{}, err
	}

	trashSiblings, err := s.siblingRecords(projectID, trashRoot.Id)
	if err != nil {
		return Item{}, err
	}
	ordered := append(removeRecord(trashSiblings, itemID), record)
	if err := s.renumber(ordered); err != nil {
		return Item{}, err
	}

	if _, err := s.commit(projectID, "Move \u201c"+record.GetString("title")+"\u201d to the Trash",
		TriggerStructural, itemID); err != nil {
		return Item{}, err
	}
	s.audit(projectID, "binder", "trash", ActorUser, "item", itemID,
		map[string]any{"title": record.GetString("title")})
	s.notifyBinder(projectID)
	return toItem(record), nil
}

// RestoreItem puts a trashed item back where it came from.
//
// The original parent may have been trashed or deleted in the meantime,
// so the fallback is the draft root: a restored chapter always lands
// somewhere the user can see it.
func (s *Store) RestoreItem(itemID string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.itemRecord(itemID)
	if err != nil {
		return Item{}, err
	}
	projectID := record.GetString("project")
	if !record.GetBool("trashed") {
		return toItem(record), nil
	}

	target, err := s.rootRecord(projectID, BucketDraft)
	if err != nil {
		return Item{}, err
	}
	if originalID := record.GetString("original_parent"); originalID != "" {
		if original, findErr := s.itemRecord(originalID); findErr == nil {
			if original.GetString("project") == projectID &&
				original.GetString("kind") == KindFolder &&
				!original.GetBool("trashed") {
				target = original
			}
		}
	}

	currentSiblings, err := s.siblingRecords(projectID, record.GetString("parent"))
	if err != nil {
		return Item{}, err
	}
	if err := s.renumber(removeRecord(currentSiblings, itemID)); err != nil {
		return Item{}, err
	}

	position := record.GetInt("original_position")
	record.Set("parent", target.Id)
	record.Set("bucket", target.GetString("bucket"))
	record.Set("trashed", target.GetBool("trashed"))
	record.Set("original_parent", "")
	record.Set("original_position", 0)
	if err := s.app.Save(record); err != nil {
		return Item{}, fmt.Errorf("restore the item: %w", err)
	}
	if err := s.setSubtreeBucket(projectID, itemID, target.GetString("bucket"), target.GetBool("trashed")); err != nil {
		return Item{}, err
	}

	siblings, err := s.siblingRecords(projectID, target.Id)
	if err != nil {
		return Item{}, err
	}
	siblings = removeRecord(siblings, itemID)
	if position < 0 {
		position = 0
	}
	if position > len(siblings) {
		position = len(siblings)
	}
	ordered := make([]*core.Record, 0, len(siblings)+1)
	ordered = append(ordered, siblings[:position]...)
	ordered = append(ordered, record)
	ordered = append(ordered, siblings[position:]...)
	if err := s.renumber(ordered); err != nil {
		return Item{}, err
	}

	if _, err := s.commit(projectID, "Restore \u201c"+record.GetString("title")+"\u201d",
		TriggerRestore, itemID); err != nil {
		return Item{}, err
	}
	s.audit(projectID, "binder", "restore", ActorUser, "item", itemID,
		map[string]any{"title": record.GetString("title")})
	s.notifyBinder(projectID)
	return toItem(record), nil
}

// DeleteItem permanently removes an item and its subtree.
//
// Only a trashed item can be deleted for good: the two-step delete is the
// Scrivener metaphor, and it is also the safety net that keeps a
// misplaced click from destroying a chapter. The Markdown files go with
// the records.
func (s *Store) DeleteItem(itemID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.itemRecord(itemID)
	if err != nil {
		return err
	}
	if !record.GetBool("trashed") {
		return fmt.Errorf("project: move the item to the Trash before deleting it permanently")
	}
	projectID := record.GetString("project")
	if record.GetString("parent") == "" {
		return fmt.Errorf("project: a top-level group cannot be deleted")
	}

	records, err := s.subtreeRecords(projectID, itemID)
	if err != nil {
		return err
	}
	for _, descendant := range records {
		if path := descendant.GetString("file_path"); path != "" {
			if target, pathErr := s.absolute(projectID, path); pathErr == nil {
				if removeErr := osRemove(target); removeErr != nil {
					s.logError("delete", removeErr)
				}
			}
		}
	}

	parentID := record.GetString("parent")
	if err := s.app.Delete(record); err != nil {
		return fmt.Errorf("delete the item: %w", err)
	}

	siblings, err := s.siblingRecords(projectID, parentID)
	if err != nil {
		return err
	}
	if err := s.renumber(siblings); err != nil {
		return err
	}

	if _, err := s.commit(projectID, "Delete \u201c"+record.GetString("title")+"\u201d",
		TriggerStructural, ""); err != nil {
		return err
	}
	s.audit(projectID, "binder", "delete", ActorUser, "item", itemID,
		map[string]any{"title": record.GetString("title")})
	s.notifyBinder(projectID)
	return nil
}

// DuplicateItem copies an item, its metadata and its subtree.
//
// The copy is inserted directly after the original, which is where a
// duplicated chapter is expected to appear. The Markdown bodies are
// copied byte for byte; the copy gets its own file because the file name
// is the new record id.
func (s *Store) DuplicateItem(itemID string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	source, err := s.itemRecord(itemID)
	if err != nil {
		return Item{}, err
	}
	projectID := source.GetString("project")
	parentID := source.GetString("parent")

	newID, err := s.duplicateRecord(source, parentID, source.GetInt("position")+1, true)
	if err != nil {
		return Item{}, err
	}

	siblings, err := s.siblingRecords(projectID, parentID)
	if err != nil {
		return Item{}, err
	}
	position := source.GetInt("position") + 1
	if position > len(siblings)-1 {
		position = len(siblings) - 1
	}
	if position < 0 {
		position = 0
	}
	var duplicate *core.Record
	ordered := make([]*core.Record, 0, len(siblings))
	for _, sibling := range siblings {
		if sibling.Id == newID {
			duplicate = sibling
			continue
		}
		ordered = append(ordered, sibling)
	}
	if duplicate == nil {
		return Item{}, fmt.Errorf("project: the duplicate disappeared")
	}
	ordered = append(ordered, nil)
	copy(ordered[position+1:], ordered[position:])
	ordered[position] = duplicate
	if err := s.renumber(ordered); err != nil {
		return Item{}, err
	}

	if _, err := s.commit(projectID, "Duplicate \u201c"+source.GetString("title")+"\u201d",
		TriggerStructural, newID); err != nil {
		return Item{}, err
	}
	s.audit(projectID, "binder", "duplicate", ActorUser, "item", newID,
		map[string]any{"source": itemID})
	s.notifyBinder(projectID)
	return toItem(duplicate), nil
}

// duplicateRecord copies one record and recurses into its children.
func (s *Store) duplicateRecord(source *core.Record, parentID string, position int, isRoot bool) (string, error) {
	title := source.GetString("title")
	if isRoot {
		title += " copy"
	}
	projectID := source.GetString("project")
	copyItem, err := s.createItem(projectID, parentID, source.GetString("bucket"),
		source.GetString("kind"), title, position)
	if err != nil {
		return "", err
	}

	record, err := s.itemRecord(copyItem.ID)
	if err != nil {
		return "", err
	}
	record.Set("synopsis", source.GetString("synopsis"))
	record.Set("notes", source.GetString("notes"))
	record.Set("keywords", decodeStrings(source, "keywords"))
	record.Set("label", source.GetString("label"))
	record.Set("status", source.GetString("status"))
	record.Set("include_in_compile", source.GetBool("include_in_compile"))
	record.Set("custom_meta", decodeMap(source, "custom_meta"))
	// A duplicate of a trashed subtree stays in the Trash.
	record.Set("trashed", source.GetBool("trashed"))

	if source.GetString("kind") == KindDocument {
		content, readErr := s.readItemFile(source)
		if readErr != nil {
			return "", readErr
		}
		target, pathErr := s.absolute(projectID, copyItem.FilePath)
		if pathErr != nil {
			return "", pathErr
		}
		if writeErr := writeFileAtomic(target, content); writeErr != nil {
			return "", writeErr
		}
		record.Set("word_count", CountWords(content))
		record.Set("char_count", CountCharacters(content))
	}
	if err := s.app.Save(record); err != nil {
		return "", fmt.Errorf("copy the item metadata: %w", err)
	}

	children, err := s.siblingRecords(projectID, source.Id)
	if err != nil {
		return "", err
	}
	for index, child := range children {
		if _, err := s.duplicateRecord(child, copyItem.ID, index, false); err != nil {
			return "", err
		}
	}
	return copyItem.ID, nil
}

// subtreeRecords returns an item and all its descendants, parents before
// children.
func (s *Store) subtreeRecords(projectID, rootID string) ([]*core.Record, error) {
	all, err := s.itemRecords(projectID, "", nil)
	if err != nil {
		return nil, err
	}
	children := make(map[string][]*core.Record, len(all))
	for _, record := range all {
		children[record.GetString("parent")] = append(children[record.GetString("parent")], record)
	}

	root, err := s.itemRecord(rootID)
	if err != nil {
		return nil, err
	}
	result := []*core.Record{root}
	queue := []string{rootID}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range children[parent] {
			result = append(result, child)
			queue = append(queue, child.Id)
		}
	}
	return result, nil
}

// isDescendant reports whether candidate is in the subtree of ancestor.
func (s *Store) isDescendant(projectID, candidate, ancestor string) (bool, error) {
	current := candidate
	for depth := 0; depth < 1000; depth++ {
		if current == ancestor {
			return true, nil
		}
		if current == "" {
			return false, nil
		}
		record, err := s.itemRecord(current)
		if err != nil {
			return false, nil
		}
		if record.GetString("project") != projectID {
			return false, nil
		}
		current = record.GetString("parent")
	}
	return false, fmt.Errorf("project: the binder tree is deeper than 1000 levels")
}

// setSubtreeBucket applies a bucket and trash flag to an item's
// descendants.
//
// Moving a folder into the research area or the trash must take its
// contents with it; a child that kept its old bucket would compile into
// the manuscript from the trash.
func (s *Store) setSubtreeBucket(projectID, rootID, bucket string, trashed bool) error {
	records, err := s.subtreeRecords(projectID, rootID)
	if err != nil {
		return err
	}
	for _, record := range records[1:] {
		if record.GetString("bucket") == bucket && record.GetBool("trashed") == trashed {
			continue
		}
		record.Set("bucket", bucket)
		record.Set("trashed", trashed)
		if err := s.app.Save(record); err != nil {
			return fmt.Errorf("move the subtree: %w", err)
		}
	}
	return nil
}

// osRemove removes a file, treating "already gone" as success.
func osRemove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}
