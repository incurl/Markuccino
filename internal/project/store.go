package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/incurl/Markuccino/internal/console"
	"github.com/incurl/Markuccino/internal/project/vcs"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Store owns the writing domain.
//
// One Store serves the whole application. The mutex serialises the writes
// that span a file, a record and a commit: two concurrent saves of the
// same chapter would otherwise interleave their writes and their
// revisions. The desktop application has one user and a handful of
// writes per second, so a single lock is both sufficient and easier to
// reason about than a per-project one.
type Store struct {
	app     core.App
	dataDir string
	log     *console.Log

	mu    sync.Mutex
	repos map[string]*vcs.Repo

	// notify pushes a structural change to the frontend. It is nil in
	// tests, and until the Wails application exists. It is an atomic
	// value because the notifier is called from methods that already
	// hold mu: taking the lock again just to read it would deadlock.
	notify atomic.Value
}

// NewStore creates the writing store.
func NewStore(app core.App, dataDir string, log *console.Log) *Store {
	return &Store{
		app:     app,
		dataDir: dataDir,
		log:     log,
		repos:   make(map[string]*vcs.Repo),
	}
}

// SetNotifier attaches the callback that tells the frontend a project's
// binder changed.
func (s *Store) SetNotifier(notify func(projectID string)) {
	if notify == nil {
		return
	}
	s.notify.Store(notify)
}

// notifier returns the current callback, or nil.
func (s *Store) notifier() func(string) {
	value := s.notify.Load()
	if value == nil {
		return nil
	}
	fn, _ := value.(func(string))
	return fn
}

// --- Paths ------------------------------------------------------------

// projectRoot is the workspace directory of a project, relative to the
// data directory.
func projectRoot(projectID string) string {
	return filepath.ToSlash(filepath.Join("projects", projectID))
}

// projectDir is the absolute workspace directory.
func (s *Store) projectDir(projectID string) string {
	return filepath.Join(s.dataDir, filepath.FromSlash(projectRoot(projectID)))
}

// documentRelPath is the Markdown file of a binder item, relative to the
// project root.
func documentRelPath(itemID string) string {
	return documentsDir + "/" + itemID + documentExtension
}

// absolute turns a project-relative path into a path on disk, refusing
// anything that leaves the project.
func (s *Store) absolute(projectID, rel string) (string, error) {
	if err := vcs.ValidatePath(rel); err != nil {
		return "", err
	}
	root := s.projectDir(projectID)
	target := filepath.Join(root, filepath.FromSlash(rel))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("project: path %q escapes the project workspace", rel)
	}
	return target, nil
}

// repo returns the open repository of a project, opening it on first use.
// The caller must hold the mutex.
func (s *Store) repo(projectID string) (*vcs.Repo, error) {
	if cached, ok := s.repos[projectID]; ok {
		return cached, nil
	}
	repository, err := vcs.Open(s.projectDir(projectID))
	if err != nil {
		return nil, err
	}
	s.repos[projectID] = repository
	return repository, nil
}

// --- Projects ---------------------------------------------------------

// CreateProject creates the record, the workspace directory, the Git
// repository and the three binder roots.
func (s *Store) CreateProject(name string) (Project, error) {
	name = CleanTitle(name, "Untitled Project")

	s.mu.Lock()
	defer s.mu.Unlock()

	collection, err := s.app.FindCollectionByNameOrId(CollectionProjects)
	if err != nil {
		return Project{}, fmt.Errorf("find the projects collection: %w", err)
	}

	record := core.NewRecord(collection)
	record.Set("name", name)
	record.Set("slug", s.uniqueSlug(name))
	record.Set("root", projectRoot(record.Id))
	record.Set("settings", map[string]any{})
	record.Set("layout", DefaultLayout())
	if err := s.app.Save(record); err != nil {
		return Project{}, fmt.Errorf("create the project: %w", err)
	}

	// The workspace path depends on the record id, so the directory and
	// the repository can only be created after the record exists.
	if err := os.MkdirAll(filepath.Join(s.projectDir(record.Id), documentsDir), 0o700); err != nil {
		return Project{}, fmt.Errorf("create the project workspace: %w", err)
	}
	if _, err := s.repo(record.Id); err != nil {
		return Project{}, err
	}

	// The three top-level groups. They are ordinary folders with a
	// bucket, so the tree code treats them like any other folder.
	roots := []struct {
		title    string
		bucket   string
		position int
	}{
		{DraftRootTitle, BucketDraft, 0},
		{ResearchRootTitle, BucketResearch, 1},
		{TrashRootTitle, BucketTrash, 2},
	}
	for _, root := range roots {
		if _, err := s.createItem(record.Id, "", root.bucket, KindFolder, root.title, root.position); err != nil {
			return Project{}, err
		}
	}

	commit, err := s.commit(record.Id, "Create project \u201c"+name+"\u201d", TriggerStructural, "")
	if err != nil {
		return Project{}, err
	}
	s.audit(record.Id, "project", "create", ActorUser, "project", record.Id,
		map[string]any{"name": name, "commit": commit.Hash})

	s.logInfo("project", "created project "+record.Id+" ("+name+")")
	return s.toProject(record)
}

// ListProjects returns the projects, most recently updated first.
func (s *Store) ListProjects() ([]Project, error) {
	records, err := s.app.FindRecordsByFilter(CollectionProjects, "", "-updated", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("list the projects: %w", err)
	}
	projects := make([]Project, 0, len(records))
	for _, record := range records {
		project, mapErr := s.toProject(record)
		if mapErr != nil {
			return nil, mapErr
		}
		projects = append(projects, project)
	}
	return projects, nil
}

// GetProject returns one project.
func (s *Store) GetProject(projectID string) (Project, error) {
	record, err := s.projectRecord(projectID)
	if err != nil {
		return Project{}, err
	}
	return s.toProject(record)
}

// RenameProject changes the display name. The slug and the workspace path
// stay put: they identify the project on disk, and a rename must not move
// its files.
func (s *Store) RenameProject(projectID, name string) (Project, error) {
	name = CleanTitle(name, "Untitled Project")

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.projectRecord(projectID)
	if err != nil {
		return Project{}, err
	}
	record.Set("name", name)
	if err := s.app.Save(record); err != nil {
		return Project{}, fmt.Errorf("rename the project: %w", err)
	}
	s.audit(projectID, "project", "rename", ActorUser, "project", projectID, map[string]any{"name": name})
	return s.toProject(record)
}

// DeleteProject removes the record, the binder and the workspace
// directory, including the Git repository.
//
// A project deletion is not reversible: it is the one operation the user
// has to confirm. The trash protects an item, not a project.
func (s *Store) DeleteProject(projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.projectRecord(projectID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(s.projectDir(projectID)); err != nil {
		return fmt.Errorf("remove the project workspace: %w", err)
	}
	if err := s.app.Delete(record); err != nil {
		return fmt.Errorf("delete the project: %w", err)
	}
	delete(s.repos, projectID)
	s.logInfo("project", "deleted project "+projectID)
	return nil
}

// Workspace returns a project and its flat binder list.
func (s *Store) Workspace(projectID string) (Workspace, error) {
	project, err := s.GetProject(projectID)
	if err != nil {
		return Workspace{}, err
	}
	items, err := s.Items(projectID)
	if err != nil {
		return Workspace{}, err
	}
	return Workspace{Project: project, Items: items}, nil
}

// Items returns the binder items of a project, ordered by position.
func (s *Store) Items(projectID string) ([]Item, error) {
	records, err := s.itemRecords(projectID, "", nil)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(records))
	for _, record := range records {
		items = append(items, toItem(record))
	}
	sortItems(items)
	return items, nil
}

// SaveLayout stores the session state of a project.
func (s *Store) SaveLayout(projectID string, layout Layout) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if layout.View == "" {
		layout.View = ViewEditor
	}
	if layout.Split == "" {
		layout.Split = SplitNone
	}
	if layout.ExpandedFolderIDs == nil {
		layout.ExpandedFolderIDs = []string{}
	}

	record, err := s.projectRecord(projectID)
	if err != nil {
		return err
	}
	record.Set("layout", layout)
	if err := s.app.Save(record); err != nil {
		return fmt.Errorf("save the layout: %w", err)
	}
	return nil
}

// SaveSettings replaces the per-project settings object.
//
// The settings are deliberately an open map: a new preference (the word
// target, the autosnapshot interval) does not need a migration.
func (s *Store) SaveSettings(projectID string, settings map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if settings == nil {
		settings = map[string]any{}
	}
	record, err := s.projectRecord(projectID)
	if err != nil {
		return err
	}
	record.Set("settings", settings)
	if err := s.app.Save(record); err != nil {
		return fmt.Errorf("save the project settings: %w", err)
	}
	return nil
}

// SetLastOpenItem remembers which binder item was open, so the next start
// reopens it.
func (s *Store) SetLastOpenItem(projectID, itemID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.projectRecord(projectID)
	if err != nil {
		return err
	}
	record.Set("last_open_item", itemID)
	if err := s.app.Save(record); err != nil {
		return fmt.Errorf("remember the open item: %w", err)
	}
	return nil
}

// projectRecord loads a project record.
func (s *Store) projectRecord(projectID string) (*core.Record, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("project: an id is required")
	}
	record, err := s.app.FindRecordById(CollectionProjects, projectID)
	if err != nil {
		return nil, fmt.Errorf("find the project %q: %w", projectID, err)
	}
	return record, nil
}

// uniqueSlug returns a slug no project uses yet.
func (s *Store) uniqueSlug(name string) string {
	base := Slugify(name)
	slug := base
	for attempt := 2; attempt < 100; attempt++ {
		_, err := s.app.FindFirstRecordByFilter(CollectionProjects, "slug = {:slug}",
			dbx.Params{"slug": slug})
		if err != nil {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, attempt)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

// toProject converts a record into the frontend shape.
func (s *Store) toProject(record *core.Record) (Project, error) {
	project := Project{
		ID:             record.Id,
		Name:           record.GetString("name"),
		Slug:           record.GetString("slug"),
		Root:           record.GetString("root"),
		LastOpenItemID: record.GetString("last_open_item"),
		Settings:       decodeMap(record, "settings"),
		Layout:         decodeLayout(record),
		Created:        formatTime(record, "created"),
		Updated:        formatTime(record, "updated"),
	}

	// The word count is the draft's, not the whole project's: research
	// never enters the manuscript.
	records, err := s.itemRecords(record.Id, "bucket = {:bucket} && trashed = false",
		dbx.Params{"bucket": BucketDraft})
	if err != nil {
		return Project{}, err
	}
	total := 0
	for _, item := range records {
		total += item.GetInt("word_count")
	}
	project.WordCount = total

	return project, nil
}

// decodeLayout reads the layout JSON, filling defaults for a project that
// has none.
func decodeLayout(record *core.Record) Layout {
	layout := DefaultLayout()
	raw := record.Get("layout")
	if raw == nil {
		return layout
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return layout
	}
	if err := json.Unmarshal(data, &layout); err != nil {
		return layout
	}
	if layout.View == "" {
		layout.View = ViewEditor
	}
	if layout.Split == "" {
		layout.Split = SplitNone
	}
	if layout.ExpandedFolderIDs == nil {
		layout.ExpandedFolderIDs = []string{}
	}
	return layout
}

// decodeMap reads a JSON object field.
func decodeMap(record *core.Record, field string) map[string]any {
	result := map[string]any{}
	raw := record.Get(field)
	if raw == nil {
		return result
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return result
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return map[string]any{}
	}
	return result
}

// decodeStrings reads a JSON array of strings field.
func decodeStrings(record *core.Record, field string) []string {
	result := []string{}
	raw := record.Get(field)
	if raw == nil {
		return result
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return result
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return []string{}
	}
	return result
}

// formatTime renders a PocketBase date field as RFC 3339.
func formatTime(record *core.Record, field string) string {
	value := record.GetDateTime(field)
	if value.IsZero() {
		return ""
	}
	return value.Time().UTC().Format(time.RFC3339)
}

// itemRecords loads the items of a project. The extra filter is appended
// to the project filter; an empty one selects every item.
func (s *Store) itemRecords(projectID, extra string, params dbx.Params) ([]*core.Record, error) {
	filter := "project = {:project}"
	merged := dbx.Params{"project": projectID}
	for key, value := range params {
		merged[key] = value
	}
	if extra != "" {
		filter += " && " + extra
	}
	records, err := s.app.FindRecordsByFilter(CollectionItems, filter, "position,created", 0, 0, merged)
	if err != nil {
		return nil, fmt.Errorf("list the binder items: %w", err)
	}
	return records, nil
}

// sortItems orders a slice by position and then by creation time.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Position != items[j].Position {
			return items[i].Position < items[j].Position
		}
		return items[i].Created < items[j].Created
	})
}

// --- Logging and audit ------------------------------------------------

// logInfo records a normal event for the debug console.
func (s *Store) logInfo(source, message string) {
	if s.log == nil {
		return
	}
	s.log.Info("project "+source, message)
}

// logError records a failure for the debug console.
func (s *Store) logError(source string, cause error) {
	if s.log == nil {
		return
	}
	s.log.Error("project "+source, cause.Error())
}

// audit appends an operation to the audit trail.
//
// A failed audit write must not fail the operation it describes: the
// writing workflow is more important than its record, and the debug
// console still gets the failure.
func (s *Store) audit(projectID, category, action, actor, targetType, targetID string, detail map[string]any) {
	collection, err := s.app.FindCollectionByNameOrId(CollectionAuditLog)
	if err != nil {
		s.logError("audit", err)
		return
	}
	record := core.NewRecord(collection)
	if projectID != "" {
		record.Set("project", projectID)
	}
	record.Set("category", category)
	record.Set("action", action)
	record.Set("actor", actor)
	record.Set("target_type", targetType)
	record.Set("target_id", targetID)
	if detail == nil {
		detail = map[string]any{}
	}
	record.Set("detail", detail)
	if err := s.app.Save(record); err != nil {
		s.logError("audit", err)
	}
}

// commit records the working tree and the snapshot row.
//
// Every commit is a snapshot: the commit is the version-controlled
// content, the row is the queryable metadata. They are written together
// so the history panel never shows a revision without its explanation.
func (s *Store) commit(projectID, message, trigger, itemID string) (vcs.Commit, error) {
	repository, err := s.repo(projectID)
	if err != nil {
		return vcs.Commit{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	commit, err := repository.Commit(ctx, message)
	if err != nil {
		return vcs.Commit{}, err
	}
	if err := s.saveSnapshot(projectID, itemID, commit, message, trigger); err != nil {
		// The content is committed; a missing index row is a
		// bookkeeping problem, not a reason to lose the write.
		s.logError("snapshot", err)
	}
	return commit, nil
}

// saveSnapshot records the commit in the snapshots collection.
func (s *Store) saveSnapshot(projectID, itemID string, commit vcs.Commit, message, trigger string) error {
	collection, err := s.app.FindCollectionByNameOrId(CollectionSnapshots)
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	record.Set("project", projectID)
	if itemID != "" {
		record.Set("item", itemID)
	}
	record.Set("commit_hash", commit.Hash)
	record.Set("parent_hash", commit.Parent)
	record.Set("message", message)
	record.Set("author", commit.Author)
	record.Set("trigger", trigger)
	record.Set("changed_files", commit.Files)
	return s.app.Save(record)
}

// notifyBinder tells the frontend that a project's binder changed.
func (s *Store) notifyBinder(projectID string) {
	if notify := s.notifier(); notify != nil {
		notify(projectID)
	}
}

// errNotFound is returned when a binder item or a project does not exist.
var errNotFound = errors.New("not found")
