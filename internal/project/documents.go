package project

import (
	"fmt"
	"os"

	"github.com/pocketbase/pocketbase/core"
)

// maxDocumentBytes bounds a single document body.
//
// The editor saves what the user typed; a body larger than this is a
// mistake or an attack, not a chapter. It is generous on purpose: a
// 100,000-word book is well under a megabyte.
const maxDocumentBytes = 8 << 20

// readItemFile reads the Markdown body of an item.
//
// A missing file reads as an empty document rather than an error: the
// record is the source of truth for the item's existence, and the next
// save recreates the file. Deleting a file by hand should not make the
// chapter unreachable.
func (s *Store) readItemFile(record *core.Record) (string, error) {
	rel := record.GetString("file_path")
	if rel == "" {
		rel = documentRelPath(record.Id)
	}
	target, err := s.absolute(record.GetString("project"), rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read the document: %w", err)
	}
	return string(data), nil
}

// ReadDocument returns a document with its body and the current revision.
func (s *Store) ReadDocument(itemID string) (Document, error) {
	record, err := s.itemRecord(itemID)
	if err != nil {
		return Document{}, err
	}
	if record.GetString("kind") != KindDocument {
		return Document{}, fmt.Errorf("project: %q is a folder, not a document", record.GetString("title"))
	}
	if record.GetBool("trashed") {
		return Document{}, fmt.Errorf("project: the document is in the Trash")
	}

	content, err := s.readItemFile(record)
	if err != nil {
		return Document{}, err
	}

	revision := ""
	s.mu.Lock()
	repository, repoErr := s.repo(record.GetString("project"))
	s.mu.Unlock()
	if repoErr == nil {
		if head, headErr := repository.Head(); headErr == nil {
			revision = head
		}
	}

	return Document{Item: toItem(record), Content: content, Revision: revision}, nil
}

// SaveDocument writes a Markdown body and records the revision.
//
// The order matters: the file is written first, so a failure after it
// leaves the text on disk with slightly stale counts, not a lost
// sentence. The commit is the last step, and the record is updated with
// its hash so the editor can show which revision it is looking at.
func (s *Store) SaveDocument(itemID, content string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveDocument(itemID, content)
}

// saveDocument is the mutex-holding implementation, shared by
// SaveScrivenings.
func (s *Store) saveDocument(itemID, content string) (Document, error) {
	record, err := s.itemRecord(itemID)
	if err != nil {
		return Document{}, err
	}
	if record.GetString("kind") != KindDocument {
		return Document{}, fmt.Errorf("project: %q is a folder, not a document", record.GetString("title"))
	}
	if record.GetBool("trashed") {
		return Document{}, fmt.Errorf("project: the document is in the Trash")
	}
	if len(content) > maxDocumentBytes {
		return Document{}, fmt.Errorf("project: the document exceeds %d bytes", maxDocumentBytes)
	}

	projectID := record.GetString("project")
	rel := record.GetString("file_path")
	if rel == "" {
		rel = documentRelPath(record.Id)
	}
	target, err := s.absolute(projectID, rel)
	if err != nil {
		return Document{}, err
	}
	if err := writeFileAtomic(target, content); err != nil {
		return Document{}, err
	}

	record.Set("file_path", rel)
	record.Set("word_count", CountWords(content))
	record.Set("char_count", CountCharacters(content))
	if err := s.app.Save(record); err != nil {
		return Document{}, fmt.Errorf("store the document counts: %w", err)
	}

	commit, err := s.commit(projectID, "Edit \u201c"+record.GetString("title")+"\u201d", TriggerEdit, itemID)
	if err != nil {
		return Document{}, err
	}
	if commit.Created {
		record.Set("last_commit", commit.Hash)
		if err := s.app.Save(record); err != nil {
			s.logError("document", err)
		}
	}

	s.audit(projectID, "document", "save", ActorUser, "item", itemID, map[string]any{
		"words":  record.GetInt("word_count"),
		"commit": commit.Hash,
	})
	s.notifyBinder(projectID)

	return Document{Item: toItem(record), Content: content, Revision: commit.Hash}, nil
}

// ReadScrivenings returns several documents as one ordered reading
// surface.
func (s *Store) ReadScrivenings(itemIDs []string) (Scrivenings, error) {
	result := Scrivenings{Title: "Scrivenings", Sections: make([]Section, 0, len(itemIDs))}
	for _, itemID := range itemIDs {
		record, err := s.itemRecord(itemID)
		if err != nil {
			return Scrivenings{}, err
		}
		if record.GetString("kind") != KindDocument {
			continue
		}
		// The Trash is not an editing surface: its documents are not
		// part of a Scrivenings view.
		if record.GetBool("trashed") {
			continue
		}
		content, err := s.readItemFile(record)
		if err != nil {
			return Scrivenings{}, err
		}
		result.Sections = append(result.Sections, Section{
			ItemID:  record.Id,
			Title:   record.GetString("title"),
			Content: content,
		})
	}
	return result, nil
}

// SaveScrivenings writes the edited sections back in one operation.
//
// It is the write half of the Scrivenings view: the editor shows the
// sections merged, and splitting the buffer back into sections is the
// editor's job, not the store's. Each section is saved with the same
// rules as SaveDocument, and the commit covers the whole edit.
func (s *Store) SaveScrivenings(sections []Section) (Scrivenings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	saved := Scrivenings{Title: "Scrivenings", Sections: make([]Section, 0, len(sections))}
	for _, section := range sections {
		document, err := s.saveDocument(section.ItemID, section.Content)
		if err != nil {
			return Scrivenings{}, err
		}
		saved.Sections = append(saved.Sections, Section{
			ItemID:  document.Item.ID,
			Title:   document.Item.Title,
			Content: document.Content,
		})
	}
	return saved, nil
}
