// Package project owns the Scrivener-style writing domain: the project,
// the binder tree, the Markdown documents, the reference material and the
// version history.
//
// It sits below the Wails service and above PocketBase and the version
// control package. The service is a thin adapter; the rules live here:
// where a new item goes, how the siblings are renumbered, what a move to
// the trash remembers, and which file a document's text lives in.
//
// The package keeps three representations of a project in step:
//
//   - PocketBase records, which hold every piece of metadata;
//   - Markdown files on disk, which hold the document bodies;
//   - Git commits, which hold the history of those files.
//
// A write that changes a document updates all three. The order is
// deliberate: the file first, then the metadata, then the commit, so a
// failure at any step leaves a recoverable state rather than a lost
// sentence.
package project

import (
	"github.com/incurl/Markuccino/internal/project/vcs"
)

// Collection names owned by this package.
//
// The schema that creates them is the migration in
// internal/pocketbase/migrations, which names them literally. Keep the
// two in step: a rename here without a migration renames nothing.
const (
	CollectionProjects        = "projects"
	CollectionItems           = "binder_items"
	CollectionReferenceSheets = "reference_sheets"
	CollectionSnapshots       = "snapshots"
	CollectionCompilePresets  = "compile_presets"
	CollectionAuditLog        = "audit_log"
)

// Binder item kinds, stored in binder_items.kind.
const (
	// KindFolder is a container. It has no body of its own.
	KindFolder = "folder"
	// KindDocument is a writable section with a Markdown body.
	KindDocument = "document"
)

// Binder buckets, stored in binder_items.bucket.
const (
	// BucketDraft is the manuscript: the items that compile.
	BucketDraft = "draft"
	// BucketResearch holds reference material that never compiles.
	BucketResearch = "research"
	// BucketTrash holds deleted items until they are restored or
	// permanently removed.
	BucketTrash = "trash"
)

// Default titles for a new binder item.
const (
	DefaultDocumentTitle = "Untitled Document"
	DefaultFolderTitle   = "New Folder"
	// DraftRootTitle, ResearchRootTitle and TrashRootTitle name the
	// three top-level groups every project starts with.
	DraftRootTitle    = "Manuscript"
	ResearchRootTitle = "Research"
	TrashRootTitle    = "Trash"
)

// Workspace directory layout, relative to the project root.
//
// Every document body lives in documents/<record id>.md. The file name is
// the record id rather than the title, so a rename never moves a file and
// a duplicate gets a file of its own. The bucket is recorded in
// PocketBase, not in the path, so trashing an item is a metadata write
// rather than a file move.
const (
	documentsDir = "documents"
	// documentExtension is the extension of a stored document.
	documentExtension = ".md"
)

// View names stored in a project's layout.
const (
	ViewEditor    = "editor"
	ViewCorkboard = "corkboard"
	ViewOutliner  = "outliner"
)

// Split modes stored in a project's layout.
const (
	SplitNone       = "none"
	SplitHorizontal = "horizontal"
	SplitVertical   = "vertical"
)

// Snapshot triggers, stored in snapshots.trigger.
const (
	TriggerManual     = "manual"
	TriggerAuto       = "auto"
	TriggerEdit       = "edit"
	TriggerStructural = "structural"
	TriggerCompile    = "compile"
	TriggerAI         = "ai"
	TriggerRestore    = "restore"
)

// Actor names recorded in the audit trail.
const (
	ActorUser   = "user"
	ActorAI     = "ai"
	ActorSystem = "system"
)

// Project is the frontend-facing view of a project.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	// Root is the workspace directory, relative to the data directory.
	Root string `json:"root"`
	// Settings holds the per-project preferences.
	Settings map[string]any `json:"settings"`
	// Layout is the restored session state.
	Layout Layout `json:"layout"`
	// LastOpenItemID is the binder item that was open.
	LastOpenItemID string `json:"last_open_item_id"`
	// WordCount is the draft's word count, the progress indicator the
	// outliner and the composition mode show.
	WordCount int    `json:"word_count"`
	Created   string `json:"created"`
	Updated   string `json:"updated"`
}

// Layout is the per-project session state that survives a restart.
type Layout struct {
	// View is editor, corkboard or outliner.
	View string `json:"view"`
	// Split is none, horizontal or vertical.
	Split string `json:"split"`
	// SplitItemID is the item shown in the second pane.
	SplitItemID string `json:"split_item_id"`
	// AIOpen reports whether the AI panel is expanded.
	AIOpen bool `json:"ai_open"`
	// InspectorOpen reports whether the inspector is docked.
	InspectorOpen bool `json:"inspector_open"`
	// Composition reports whether the distraction-free mode was active.
	Composition bool `json:"composition"`
	// ExpandedFolderIDs remembers which folders are open in the binder.
	ExpandedFolderIDs []string `json:"expanded_folder_ids"`
}

// DefaultLayout returns the layout of a new project.
func DefaultLayout() Layout {
	return Layout{
		View:              ViewEditor,
		Split:             SplitNone,
		AIOpen:            true,
		InspectorOpen:     true,
		ExpandedFolderIDs: []string{},
	}
}

// Item is the frontend-facing view of a binder item.
type Item struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	ParentID  string `json:"parent_id"`
	// Kind is folder or document.
	Kind string `json:"kind"`
	// Bucket is draft, research or trash.
	Bucket string `json:"bucket"`
	Title  string `json:"title"`
	// FilePath is the Markdown file, relative to the project root.
	FilePath string `json:"file_path"`
	// Position is the item's index among its siblings.
	Position int    `json:"position"`
	Synopsis string `json:"synopsis"`
	Notes    string `json:"notes"`
	// Keywords is normalised to an array at the bindings wrapper.
	Keywords []string `json:"keywords"`
	Label    string   `json:"label"`
	Status   string   `json:"status"`
	// IncludeInCompile reports whether the item is part of the draft.
	IncludeInCompile bool `json:"include_in_compile"`
	WordCount        int  `json:"word_count"`
	CharCount        int  `json:"char_count"`
	// CustomMeta holds the user-defined fields.
	CustomMeta map[string]any `json:"custom_meta"`
	LastCommit string         `json:"last_commit"`
	Trashed    bool           `json:"trashed"`
	Created    string         `json:"created"`
	Updated    string         `json:"updated"`
}

// Workspace is everything the UI needs to render a project at once: the
// project and its flat item list.
//
// The tree is assembled in the frontend by buildTree, which is a pure
// function with its own test. Sending a flat list keeps the binding
// surface simple and avoids a recursive type in the generated TypeScript.
type Workspace struct {
	Project Project `json:"project"`
	Items   []Item  `json:"items"`
}

// ItemMeta is the metadata the inspector writes back.
//
// It is a full replacement rather than a patch: the inspector holds the
// complete state of the item, and a full write makes "clear the notes"
// expressible without a pointer for every field.
type ItemMeta struct {
	Title            string         `json:"title"`
	Synopsis         string         `json:"synopsis"`
	Notes            string         `json:"notes"`
	Keywords         []string       `json:"keywords"`
	Label            string         `json:"label"`
	Status           string         `json:"status"`
	IncludeInCompile bool           `json:"include_in_compile"`
	CustomMeta       map[string]any `json:"custom_meta"`
}

// Document is a binder item together with its Markdown body.
type Document struct {
	Item     Item   `json:"item"`
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

// Section is one part of a Scrivenings view.
type Section struct {
	ItemID  string `json:"item_id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// Scrivenings is several contiguous documents merged into one reading
// surface. The boundaries are kept so the editor can show them.
type Scrivenings struct {
	Title    string    `json:"title"`
	Sections []Section `json:"sections"`
}

// Revision is one commit, as the history panel shows it.
type Revision struct {
	Hash    string   `json:"hash"`
	Parent  string   `json:"parent"`
	Message string   `json:"message"`
	Author  string   `json:"author"`
	When    string   `json:"when"`
	Files   []string `json:"files"`
}

// Snapshot is a recorded revision with the metadata that explains it.
type Snapshot struct {
	ID           string   `json:"id"`
	ProjectID    string   `json:"project_id"`
	ItemID       string   `json:"item_id"`
	CommitHash   string   `json:"commit_hash"`
	ParentHash   string   `json:"parent_hash"`
	Message      string   `json:"message"`
	Author       string   `json:"author"`
	Trigger      string   `json:"trigger"`
	ChangedFiles []string `json:"changed_files"`
	Created      string   `json:"created"`
}

// RevisionDiff is the textual diff between a revision and its parent for
// one document.
type RevisionDiff struct {
	ItemID string `json:"item_id"`
	From   string `json:"from"`
	To     string `json:"to"`
	// Text is a unified diff. It is empty when the file did not change.
	Text string `json:"text"`
}

// SearchHit is one match of a project search.
type SearchHit struct {
	ItemID string `json:"item_id"`
	Title  string `json:"title"`
	// Field is title, synopsis, notes or body.
	Field   string `json:"field"`
	Excerpt string `json:"excerpt"`
	Bucket  string `json:"bucket"`
}

// ReferenceSheet is a character, location or item sheet.
type ReferenceSheet struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"project_id"`
	Kind        string         `json:"kind"`
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	Notes       string         `json:"notes"`
	Fields      map[string]any `json:"fields"`
	DocumentIDs []string       `json:"document_ids"`
	Created     string         `json:"created"`
	Updated     string         `json:"updated"`
}

// mapRevision converts a commit from the version-control package into the
// frontend-facing shape.
func mapRevision(commit vcs.Commit) Revision {
	return Revision{
		Hash:    commit.Hash,
		Parent:  commit.Parent,
		Message: commit.Message,
		Author:  commit.Author,
		When:    commit.When,
		Files:   commit.Files,
	}
}
