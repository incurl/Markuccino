package bindings

import (
	"fmt"

	"github.com/incurl/Markuccino/internal/project"
)

// The project methods are thin adapters over internal/project. The rules
// live in that package; this file exists so the frontend can reach them.

// ListProjects returns the writing projects, most recently updated first.
func (s *AppService) ListProjects() ([]project.Project, error) {
	if s.projects == nil {
		return nil, fmt.Errorf("the writing store is not available")
	}
	return s.projects.ListProjects()
}

// CreateProject creates a project with its workspace and Git repository.
func (s *AppService) CreateProject(name string) (project.Project, error) {
	if s.projects == nil {
		return project.Project{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.CreateProject(name)
}

// RenameProject changes a project's display name.
func (s *AppService) RenameProject(projectID, name string) (project.Project, error) {
	if s.projects == nil {
		return project.Project{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.RenameProject(projectID, name)
}

// DeleteProject removes a project, its workspace and its Git repository.
func (s *AppService) DeleteProject(projectID string) error {
	if s.projects == nil {
		return fmt.Errorf("the writing store is not available")
	}
	return s.projects.DeleteProject(projectID)
}

// GetWorkspace returns a project and its flat binder list.
func (s *AppService) GetWorkspace(projectID string) (project.Workspace, error) {
	if s.projects == nil {
		return project.Workspace{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.Workspace(projectID)
}

// SaveLayout stores the restored session state of a project.
func (s *AppService) SaveLayout(projectID string, layout project.Layout) error {
	if s.projects == nil {
		return fmt.Errorf("the writing store is not available")
	}
	return s.projects.SaveLayout(projectID, layout)
}

// SetLastOpenItem remembers which item was open.
func (s *AppService) SetLastOpenItem(projectID, itemID string) error {
	if s.projects == nil {
		return fmt.Errorf("the writing store is not available")
	}
	return s.projects.SetLastOpenItem(projectID, itemID)
}

// SaveProjectSettings replaces the per-project preferences.
func (s *AppService) SaveProjectSettings(projectID string, settings map[string]any) error {
	if s.projects == nil {
		return fmt.Errorf("the writing store is not available")
	}
	return s.projects.SaveSettings(projectID, settings)
}

// CreateBinderItem adds a folder or a document to the binder.
func (s *AppService) CreateBinderItem(projectID, parentID, kind, title string) (project.Item, error) {
	if s.projects == nil {
		return project.Item{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.CreateItem(projectID, parentID, kind, title)
}

// RenameBinderItem changes an item's title.
func (s *AppService) RenameBinderItem(itemID, title string) (project.Item, error) {
	if s.projects == nil {
		return project.Item{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.RenameItem(itemID, title)
}

// UpdateBinderItem writes the inspector's metadata back.
func (s *AppService) UpdateBinderItem(itemID string, meta project.ItemMeta) (project.Item, error) {
	if s.projects == nil {
		return project.Item{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.UpdateItem(itemID, meta)
}

// MoveBinderItem moves an item below a parent at an index.
func (s *AppService) MoveBinderItem(itemID, parentID string, index int) (project.Item, error) {
	if s.projects == nil {
		return project.Item{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.MoveItem(itemID, parentID, index)
}

// TrashBinderItem moves an item into the Trash.
func (s *AppService) TrashBinderItem(itemID string) (project.Item, error) {
	if s.projects == nil {
		return project.Item{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.TrashItem(itemID)
}

// RestoreBinderItem restores a trashed item.
func (s *AppService) RestoreBinderItem(itemID string) (project.Item, error) {
	if s.projects == nil {
		return project.Item{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.RestoreItem(itemID)
}

// DeleteBinderItem permanently removes a trashed item.
func (s *AppService) DeleteBinderItem(itemID string) error {
	if s.projects == nil {
		return fmt.Errorf("the writing store is not available")
	}
	return s.projects.DeleteItem(itemID)
}

// DuplicateBinderItem copies an item and its subtree.
func (s *AppService) DuplicateBinderItem(itemID string) (project.Item, error) {
	if s.projects == nil {
		return project.Item{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.DuplicateItem(itemID)
}

// ReadDocument returns a document with its Markdown body.
func (s *AppService) ReadDocument(itemID string) (project.Document, error) {
	if s.projects == nil {
		return project.Document{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.ReadDocument(itemID)
}

// SaveDocument writes a Markdown body, updates the counts and commits the
// revision.
func (s *AppService) SaveDocument(itemID, content string) (project.Document, error) {
	if s.projects == nil {
		return project.Document{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.SaveDocument(itemID, content)
}

// ReadScrivenings returns several documents as one reading surface.
func (s *AppService) ReadScrivenings(itemIDs []string) (project.Scrivenings, error) {
	if s.projects == nil {
		return project.Scrivenings{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.ReadScrivenings(itemIDs)
}

// SaveScrivenings writes the edited sections of a Scrivenings view back.
func (s *AppService) SaveScrivenings(sections []project.Section) (project.Scrivenings, error) {
	if s.projects == nil {
		return project.Scrivenings{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.SaveScrivenings(sections)
}

// CreateSnapshot commits the working tree as a named snapshot.
func (s *AppService) CreateSnapshot(projectID, itemID, message string) (project.Snapshot, error) {
	if s.projects == nil {
		return project.Snapshot{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.CreateSnapshot(projectID, itemID, message)
}

// ListSnapshots returns the recorded snapshots.
func (s *AppService) ListSnapshots(projectID, itemID string, limit int) ([]project.Snapshot, error) {
	if s.projects == nil {
		return nil, fmt.Errorf("the writing store is not available")
	}
	return s.projects.ListSnapshots(projectID, itemID, limit)
}

// ListProjectHistory returns the project's Git revisions, newest first.
func (s *AppService) ListProjectHistory(projectID string, limit int) ([]project.Revision, error) {
	if s.projects == nil {
		return nil, fmt.Errorf("the writing store is not available")
	}
	return s.projects.History(projectID, limit)
}

// DiffRevision returns the textual diff a revision made to a document.
func (s *AppService) DiffRevision(projectID, itemID, hash string) (project.RevisionDiff, error) {
	if s.projects == nil {
		return project.RevisionDiff{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.DiffRevision(projectID, itemID, hash)
}

// RestoreDocument restores a document to a revision.
func (s *AppService) RestoreDocument(itemID, hash string) (project.Document, error) {
	if s.projects == nil {
		return project.Document{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.RestoreDocument(itemID, hash)
}

// RestoreProject restores every document to its content at a revision.
func (s *AppService) RestoreProject(projectID, hash string) (project.Workspace, error) {
	if s.projects == nil {
		return project.Workspace{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.RestoreProject(projectID, hash)
}

// SearchProject searches the titles, synopses, notes and bodies.
func (s *AppService) SearchProject(projectID, query string, limit int) ([]project.SearchHit, error) {
	if s.projects == nil {
		return nil, fmt.Errorf("the writing store is not available")
	}
	return s.projects.Search(projectID, query, limit)
}

// ListReferenceSheets returns the character, location and item sheets.
func (s *AppService) ListReferenceSheets(projectID, kind string) ([]project.ReferenceSheet, error) {
	if s.projects == nil {
		return nil, fmt.Errorf("the writing store is not available")
	}
	return s.projects.ListReferenceSheets(projectID, kind)
}

// SaveReferenceSheet creates or updates a reference sheet.
func (s *AppService) SaveReferenceSheet(sheet project.ReferenceSheet) (project.ReferenceSheet, error) {
	if s.projects == nil {
		return project.ReferenceSheet{}, fmt.Errorf("the writing store is not available")
	}
	return s.projects.SaveReferenceSheet(sheet)
}

// DeleteReferenceSheet removes a reference sheet.
func (s *AppService) DeleteReferenceSheet(sheetID string) error {
	if s.projects == nil {
		return fmt.Errorf("the writing store is not available")
	}
	return s.projects.DeleteReferenceSheet(sheetID)
}
