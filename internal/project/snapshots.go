package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/incurl/Markuccino/internal/project/vcs"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// CreateSnapshot commits the working tree and records it as a snapshot.
//
// A snapshot is the Git commit; the PocketBase row is its queryable
// index. Saving a document already commits, so a manual snapshot is for a
// named point in time: "before the rewrite", "sent to the editor".
func (s *Store) CreateSnapshot(projectID, itemID, message string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.projectRecord(projectID); err != nil {
		return Snapshot{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Snapshot"
		if itemID != "" {
			if record, err := s.itemRecord(itemID); err == nil {
				message = "Snapshot of \u201c" + record.GetString("title") + "\u201d"
			}
		}
	}

	commit, err := s.commit(projectID, message, TriggerManual, itemID)
	if err != nil {
		return Snapshot{}, err
	}
	s.audit(projectID, "snapshot", "create", ActorUser, "item", itemID,
		map[string]any{"message": message, "commit": commit.Hash})

	return s.snapshotByCommit(projectID, commit.Hash)
}

// ListSnapshots returns the recorded snapshots, newest first. An empty
// itemID lists the whole project.
func (s *Store) ListSnapshots(projectID, itemID string, limit int) ([]Snapshot, error) {
	filter := "project = {:project}"
	params := dbx.Params{"project": projectID}
	if itemID != "" {
		filter += " && item = {:item}"
		params["item"] = itemID
	}
	if limit <= 0 {
		limit = 100
	}
	records, err := s.app.FindRecordsByFilter(CollectionSnapshots, filter, "-created", limit, 0, params)
	if err != nil {
		return nil, fmt.Errorf("list the snapshots: %w", err)
	}
	snapshots := make([]Snapshot, 0, len(records))
	for _, record := range records {
		snapshots = append(snapshots, toSnapshot(record))
	}
	return snapshots, nil
}

// snapshotByCommit finds the newest snapshot row of a commit.
//
// The same commit can be recorded more than once: saving a document
// commits, and a manual snapshot taken while the tree is clean points at
// that same commit. The newest row is the one the caller just created.
func (s *Store) snapshotByCommit(projectID, hash string) (Snapshot, error) {
	records, err := s.app.FindRecordsByFilter(CollectionSnapshots,
		"project = {:project} && commit_hash = {:hash}", "-created", 1, 0,
		dbx.Params{"project": projectID, "hash": hash})
	if err != nil {
		return Snapshot{}, fmt.Errorf("find the snapshot of %s: %w", hash, err)
	}
	if len(records) == 0 {
		return Snapshot{}, fmt.Errorf("find the snapshot of %s: %w", hash, errNotFound)
	}
	return toSnapshot(records[0]), nil
}

// toSnapshot converts a record into the frontend shape.
func toSnapshot(record *core.Record) Snapshot {
	return Snapshot{
		ID:           record.Id,
		ProjectID:    record.GetString("project"),
		ItemID:       record.GetString("item"),
		CommitHash:   record.GetString("commit_hash"),
		ParentHash:   record.GetString("parent_hash"),
		Message:      record.GetString("message"),
		Author:       record.GetString("author"),
		Trigger:      record.GetString("trigger"),
		ChangedFiles: decodeStrings(record, "changed_files"),
		Created:      formatTime(record, "created"),
	}
}

// History returns the project's revisions, newest first.
func (s *Store) History(projectID string, limit int) ([]Revision, error) {
	s.mu.Lock()
	repository, err := s.repo(projectID)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	commits, err := repository.History(ctx, limit)
	if err != nil {
		return nil, err
	}
	revisions := make([]Revision, 0, len(commits))
	for _, commit := range commits {
		revisions = append(revisions, mapRevision(commit))
	}
	return revisions, nil
}

// DiffRevision renders the change a revision made to one document.
//
// The diff runs from the parent revision to the revision itself, which is
// the direction "what did this snapshot change" asks for. The first
// revision of a file diffs against an empty file.
func (s *Store) DiffRevision(projectID, itemID, hash string) (RevisionDiff, error) {
	record, err := s.itemRecord(itemID)
	if err != nil {
		return RevisionDiff{}, err
	}
	if record.GetString("project") != projectID {
		return RevisionDiff{}, fmt.Errorf("project: the item belongs to another project")
	}
	path := record.GetString("file_path")
	if path == "" {
		path = documentRelPath(record.Id)
	}

	s.mu.Lock()
	repository, err := s.repo(projectID)
	s.mu.Unlock()
	if err != nil {
		return RevisionDiff{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	commit, err := repository.Get(ctx, hash)
	if err != nil {
		return RevisionDiff{}, err
	}
	newContent, err := repository.ReadFile(ctx, hash, path)
	if err != nil {
		if errors.Is(err, vcs.ErrNotFound) || strings.Contains(err.Error(), vcs.ErrNotFound.Error()) {
			return RevisionDiff{ItemID: itemID, From: commit.Parent, To: hash}, nil
		}
		return RevisionDiff{}, err
	}
	oldContent := ""
	if commit.Parent != "" {
		if content, readErr := repository.ReadFile(ctx, commit.Parent, path); readErr == nil {
			oldContent = content
		}
	}

	return RevisionDiff{
		ItemID: itemID,
		From:   commit.Parent,
		To:     hash,
		Text:   UnifiedDiff(path, commit.Parent, hash, oldContent, newContent),
	}, nil
}

// RestoreDocument puts a document back to the version in a revision.
//
// It writes the old content and commits it as a new revision rather than
// moving the branch: the history stays append-only, so restoring is
// itself undoable.
func (s *Store) RestoreDocument(itemID, hash string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.itemRecord(itemID)
	if err != nil {
		return Document{}, err
	}
	if record.GetString("kind") != KindDocument {
		return Document{}, fmt.Errorf("project: %q is a folder, not a document", record.GetString("title"))
	}
	projectID := record.GetString("project")
	path := record.GetString("file_path")
	if path == "" {
		path = documentRelPath(record.Id)
	}

	repository, err := s.repo(projectID)
	if err != nil {
		return Document{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := repository.RestoreFile(ctx, hash, path); err != nil {
		return Document{}, err
	}
	content, err := repository.ReadFile(ctx, hash, path)
	if err != nil {
		return Document{}, err
	}

	record.Set("word_count", CountWords(content))
	record.Set("char_count", CountCharacters(content))
	if err := s.app.Save(record); err != nil {
		return Document{}, fmt.Errorf("restore the document counts: %w", err)
	}

	commit, err := s.commit(projectID, "Restore \u201c"+record.GetString("title")+"\u201d to "+shortHash(hash),
		TriggerRestore, itemID)
	if err != nil {
		return Document{}, err
	}
	record.Set("last_commit", commit.Hash)
	if err := s.app.Save(record); err != nil {
		s.logError("restore", err)
	}

	s.audit(projectID, "document", "restore", ActorUser, "item", itemID,
		map[string]any{"commit": commit.Hash, "restored_from": hash})
	s.notifyBinder(projectID)
	return Document{Item: toItem(record), Content: content, Revision: commit.Hash}, nil
}

// RestoreProject puts every document back to its content at a revision.
//
// The files are restored one by one, then a single commit records the
// whole operation. Items that did not exist in the old revision keep
// their current content: restoring a project must not delete a chapter
// that was written after the snapshot.
func (s *Store) RestoreProject(projectID, hash string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	repository, err := s.repo(projectID)
	if err != nil {
		return Workspace{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	files, err := repository.ListFiles(ctx, hash)
	if err != nil {
		return Workspace{}, err
	}
	restored := 0
	for _, path := range files {
		if !strings.HasPrefix(path, documentsDir+"/") {
			continue
		}
		if err := repository.RestoreFile(ctx, hash, path); err != nil {
			return Workspace{}, err
		}
		restored++
	}

	records, err := s.itemRecords(projectID, "", nil)
	if err != nil {
		return Workspace{}, err
	}
	for _, record := range records {
		if record.GetString("kind") != KindDocument {
			continue
		}
		content, readErr := s.readItemFile(record)
		if readErr != nil {
			continue
		}
		record.Set("word_count", CountWords(content))
		record.Set("char_count", CountCharacters(content))
		if saveErr := s.app.Save(record); saveErr != nil {
			return Workspace{}, fmt.Errorf("restore the document counts: %w", saveErr)
		}
	}

	commit, err := s.commit(projectID, "Restore the project to "+shortHash(hash), TriggerRestore, "")
	if err != nil {
		return Workspace{}, err
	}
	s.audit(projectID, "project", "restore", ActorUser, "project", projectID,
		map[string]any{"commit": commit.Hash, "restored_from": hash, "files": restored})
	s.notifyBinder(projectID)

	project, err := s.GetProject(projectID)
	if err != nil {
		return Workspace{}, err
	}
	return Workspace{Project: project, Items: s.mustItems(projectID)}, nil
}

// mustItems returns the items of a project, ignoring the error because the
// project was just loaded successfully. It is the one place that is safe:
// the query only reads rows the cascade keeps alive.
func (s *Store) mustItems(projectID string) []Item {
	records, err := s.itemRecords(projectID, "", nil)
	if err != nil {
		return []Item{}
	}
	items := make([]Item, 0, len(records))
	for _, record := range records {
		items = append(items, toItem(record))
	}
	sortItems(items)
	return items
}
