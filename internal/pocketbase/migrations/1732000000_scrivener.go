package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// init registers the Scrivener-style writing schema.
//
// It is a separate migration from the scaffold's initial one so that an
// existing database picks it up: PocketBase records the file name and
// applies only the migrations it has not seen.
func init() {
	registerMigration("1732000000_scrivener.go", func(app core.App) error {
		return createScrivenerSchema(app)
	})
}

// createScrivenerSchema creates the collections that model a project, its
// binder, its reference material, its version history and its AI and
// extension records.
//
// Every collection created here has **no access rules**. That is
// deliberate: the frontend never talks to PocketBase directly, every
// read and write goes through a Wails binding, and a collection with no
// rule is invisible to PocketBase's HTTP API. It also closes the
// server-mode exposure described in docs/audit.md for the writing data:
// there is no public list rule to widen.
//
// The file name is the ordering key, so the collections are created in
// dependency order: a relation needs the collection it points at to
// exist first.
func createScrivenerSchema(app core.App) error {
	if exists, err := collectionExists(app, "projects"); err != nil {
		return err
	} else if exists {
		// The migration already ran. Every collection below is created
		// in one pass, so the presence of the first one is enough.
		return nil
	}

	// --- projects ---------------------------------------------------
	// One record per writing project. The workspace directory and the
	// Git repository are named after the record id, so the project can
	// be relocated without rewriting its children.
	projects := core.NewBaseCollection("projects")
	projects.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 200},
		&core.TextField{Name: "slug", Max: 200},
		// root is the workspace directory, relative to the data
		// directory.
		&core.TextField{Name: "root", Max: 500},
		// settings holds the per-project preferences: compile defaults,
		// autosnapshot interval and the like.
		&core.JSONField{Name: "settings"},
		// layout holds the restored session: the active view, the split
		// configuration and the AI panel state.
		&core.JSONField{Name: "layout"},
		&core.TextField{Name: "last_open_item", Max: 100},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnUpdate: true},
	)
	projects.Indexes = types.JSONArray[string]{
		"CREATE UNIQUE INDEX idx_projects_slug ON projects (slug)",
	}
	if err := app.Save(projects); err != nil {
		return err
	}

	return createScrivenerBinderSchema(app, projects)
}

// createScrivenerBinderSchema creates the binder tree and the reference
// sheets.
func createScrivenerBinderSchema(app core.App, projects *core.Collection) error {
	// --- binder_items -----------------------------------------------
	// The ring-binder tree. A row is either a folder (a container) or a
	// document (a writable section). parent is a self-relation; a root
	// item has no parent. position orders the siblings.
	binderItems := core.NewBaseCollection("binder_items")
	binderItems.Fields.Add(
		&core.RelationField{
			Name:          "project",
			CollectionId:  projects.Id,
			MaxSelect:     1,
			Required:      true,
			CascadeDelete: true,
		},
		&core.SelectField{
			Name:      "kind",
			Values:    []string{"folder", "document"},
			MaxSelect: 1,
			Required:  true,
		},
		// bucket is the binder area: the draft, the research folder or
		// the trash. It is stored explicitly rather than derived from
		// the root, so a subtree can be moved between areas in one write.
		&core.SelectField{
			Name:      "bucket",
			Values:    []string{"draft", "research", "trash"},
			MaxSelect: 1,
		},
		&core.TextField{Name: "title", Required: true, Max: 300},
		// file_path is the Markdown file, relative to the project root.
		// It is derived from the record id, so a rename never moves a
		// file.
		&core.TextField{Name: "file_path", Max: 500},
		&core.NumberField{Name: "position"},
		&core.TextField{Name: "synopsis"},
		&core.TextField{Name: "notes"},
		&core.JSONField{Name: "keywords"},
		&core.TextField{Name: "label", Max: 100},
		&core.TextField{Name: "status", Max: 100},
		&core.BoolField{Name: "include_in_compile"},
		&core.NumberField{Name: "word_count"},
		&core.NumberField{Name: "char_count"},
		// custom_meta holds the user-defined metadata fields. It is a
		// JSON object so a project can add fields without a migration.
		&core.JSONField{Name: "custom_meta"},
		&core.TextField{Name: "last_commit", Max: 100},
		&core.BoolField{Name: "trashed"},
		// original_parent and original_position record where a trashed
		// item came from, so Restore puts it back.
		&core.TextField{Name: "original_parent", Max: 100},
		&core.NumberField{Name: "original_position"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnUpdate: true},
	)
	if err := app.Save(binderItems); err != nil {
		return err
	}

	// The parent relation points at the collection being created, so it
	// can only be added once the collection exists: PocketBase validates
	// that the referenced collection id is in the database. The indexes
	// come with it, because idx_binder_items_parent names the column.
	binderItems.Fields.Add(&core.RelationField{
		Name:         "parent",
		CollectionId: binderItems.Id,
		MaxSelect:    1,
	})
	binderItems.Indexes = types.JSONArray[string]{
		"CREATE INDEX idx_binder_items_project ON binder_items (project)",
		"CREATE INDEX idx_binder_items_parent ON binder_items (parent)",
		"CREATE INDEX idx_binder_items_project_bucket ON binder_items (project, bucket)",
		"CREATE INDEX idx_binder_items_trashed ON binder_items (trashed)",
	}
	if err := app.Save(binderItems); err != nil {
		return err
	}

	return createScrivenerReferenceSchema(app, projects, binderItems)
}

// createScrivenerReferenceSchema creates the reference sheets and the
// version-control index.
func createScrivenerReferenceSchema(app core.App, projects, binderItems *core.Collection) error {
	// --- reference_sheets -------------------------------------------
	// Characters, locations and items. fields is a JSON object keyed by
	// the sheet kind's field names, so the sheet kind can evolve without
	// a migration.
	referenceSheets := core.NewBaseCollection("reference_sheets")
	referenceSheets.Fields.Add(
		&core.RelationField{
			Name:          "project",
			CollectionId:  projects.Id,
			MaxSelect:     1,
			Required:      true,
			CascadeDelete: true,
		},
		&core.SelectField{
			Name:      "kind",
			Values:    []string{"character", "location", "item"},
			MaxSelect: 1,
			Required:  true,
		},
		&core.TextField{Name: "name", Required: true, Max: 300},
		&core.TextField{Name: "summary"},
		&core.TextField{Name: "notes"},
		&core.JSONField{Name: "fields"},
		// document_ids links the sheet to the binder items it appears
		// in. It is a JSON array rather than a relation so a sheet
		// survives an item being permanently deleted.
		&core.JSONField{Name: "document_ids"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnUpdate: true},
	)
	referenceSheets.Indexes = types.JSONArray[string]{
		"CREATE INDEX idx_reference_sheets_project ON reference_sheets (project)",
		"CREATE INDEX idx_reference_sheets_kind ON reference_sheets (project, kind)",
	}
	if err := app.Save(referenceSheets); err != nil {
		return err
	}

	// --- snapshots ---------------------------------------------------
	// A snapshot is a Git commit plus the metadata that explains it. The
	// commit itself lives in the project's repository; this table is the
	// queryable index of it.
	snapshots := core.NewBaseCollection("snapshots")
	snapshots.Fields.Add(
		&core.RelationField{
			Name:          "project",
			CollectionId:  projects.Id,
			MaxSelect:     1,
			Required:      true,
			CascadeDelete: true,
		},
		&core.RelationField{
			Name:         "item",
			CollectionId: binderItems.Id,
			MaxSelect:    1,
		},
		&core.TextField{Name: "commit_hash", Max: 100},
		&core.TextField{Name: "parent_hash", Max: 100},
		&core.TextField{Name: "message"},
		&core.TextField{Name: "author", Max: 200},
		&core.SelectField{
			Name:      "trigger",
			Values:    []string{"manual", "auto", "edit", "structural", "compile", "ai", "restore"},
			MaxSelect: 1,
		},
		&core.JSONField{Name: "changed_files"},
		// session and tool_call link a snapshot to the AI activity that
		// produced it, when there was one.
		&core.TextField{Name: "session", Max: 100},
		&core.TextField{Name: "tool_call", Max: 100},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	snapshots.Indexes = types.JSONArray[string]{
		"CREATE INDEX idx_snapshots_project ON snapshots (project, created)",
		"CREATE INDEX idx_snapshots_item ON snapshots (item, created)",
		"CREATE INDEX idx_snapshots_hash ON snapshots (commit_hash)",
	}
	if err := app.Save(snapshots); err != nil {
		return err
	}

	// --- compile_presets --------------------------------------------
	// The saved assembly settings: which sections are included, how the
	// titles are treated and which format is the default.
	compilePresets := core.NewBaseCollection("compile_presets")
	compilePresets.Fields.Add(
		&core.RelationField{
			Name:          "project",
			CollectionId:  projects.Id,
			MaxSelect:     1,
			Required:      true,
			CascadeDelete: true,
		},
		&core.TextField{Name: "name", Required: true, Max: 200},
		&core.JSONField{Name: "config"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnUpdate: true},
	)
	compilePresets.Indexes = types.JSONArray[string]{
		"CREATE INDEX idx_compile_presets_project ON compile_presets (project)",
	}
	if err := app.Save(compilePresets); err != nil {
		return err
	}

	return createScrivenerAISchema(app, projects, binderItems)
}

// createScrivenerAISchema creates the AI session, tool-call and snapshot
// link tables.
func createScrivenerAISchema(app core.App, projects, binderItems *core.Collection) error {
	// --- ai_sessions -------------------------------------------------
	// One record per conversation. context is the snapshot of what the
	// assistant was looking at when the session last ran, so a restored
	// session knows which document it belongs to.
	aiSessions := core.NewBaseCollection("ai_sessions")
	aiSessions.Fields.Add(
		&core.RelationField{
			Name:          "project",
			CollectionId:  projects.Id,
			MaxSelect:     1,
			CascadeDelete: true,
		},
		&core.TextField{Name: "title", Max: 300},
		&core.SelectField{
			Name:      "status",
			Values:    []string{"active", "archived"},
			MaxSelect: 1,
		},
		&core.JSONField{Name: "context"},
		&core.DateField{Name: "last_message_at"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnUpdate: true},
	)
	aiSessions.Indexes = types.JSONArray[string]{
		"CREATE INDEX idx_ai_sessions_project ON ai_sessions (project, updated)",
	}
	if err := app.Save(aiSessions); err != nil {
		return err
	}

	// --- ai_messages -------------------------------------------------
	// The transcript. tool_calls holds the calls the assistant requested
	// in that message; the tool_calls collection holds their execution.
	aiMessages := core.NewBaseCollection("ai_messages")
	aiMessages.Fields.Add(
		&core.RelationField{
			Name:          "session",
			CollectionId:  aiSessions.Id,
			MaxSelect:     1,
			Required:      true,
			CascadeDelete: true,
		},
		&core.SelectField{
			Name:      "role",
			Values:    []string{"system", "user", "assistant", "tool"},
			MaxSelect: 1,
			Required:  true,
		},
		&core.TextField{Name: "content"},
		&core.JSONField{Name: "tool_calls"},
		&core.TextField{Name: "tool_call_id", Max: 100},
		&core.JSONField{Name: "context"},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	aiMessages.Indexes = types.JSONArray[string]{
		"CREATE INDEX idx_ai_messages_session ON ai_messages (session, created)",
	}
	if err := app.Save(aiMessages); err != nil {
		return err
	}

	// --- tool_calls --------------------------------------------------
	// Every built-in tool and every MCP server call lands here, whether
	// it succeeded or failed. This is the audit trail the prompt asks
	// for: a tool call that fails must still be queryable.
	toolCalls := core.NewBaseCollection("tool_calls")
	toolCalls.Fields.Add(
		&core.RelationField{
			Name:          "project",
			CollectionId:  projects.Id,
			MaxSelect:     1,
			CascadeDelete: true,
		},
		&core.RelationField{
			Name:         "session",
			CollectionId: aiSessions.Id,
			MaxSelect:    1,
		},
		&core.RelationField{
			Name:         "message",
			CollectionId: aiMessages.Id,
			MaxSelect:    1,
		},
		&core.SelectField{
			Name:      "source",
			Values:    []string{"builtin", "mcp", "extension"},
			MaxSelect: 1,
		},
		&core.TextField{Name: "tool_name", Required: true, Max: 200},
		&core.JSONField{Name: "arguments"},
		&core.JSONField{Name: "result"},
		&core.TextField{Name: "result_summary"},
		&core.SelectField{
			Name:      "status",
			Values:    []string{"ok", "error", "cancelled"},
			MaxSelect: 1,
		},
		&core.TextField{Name: "error_message"},
		&core.NumberField{Name: "duration_ms"},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	toolCalls.Indexes = types.JSONArray[string]{
		"CREATE INDEX idx_tool_calls_project ON tool_calls (project, created)",
		"CREATE INDEX idx_tool_calls_session ON tool_calls (session, created)",
		"CREATE INDEX idx_tool_calls_name ON tool_calls (tool_name)",
	}
	if err := app.Save(toolCalls); err != nil {
		return err
	}

	return createScrivenerExtensionSchema(app, projects)
}

// createScrivenerExtensionSchema creates the extension registry, its
// runtime log, and the append-only audit trail.
func createScrivenerExtensionSchema(app core.App, projects *core.Collection) error {
	// --- extensions --------------------------------------------------
	// The installed JavaScript/HTML extensions. The files live on disk
	// under the data directory; this record holds the manifest, the
	// granted permissions and the enabled state.
	extensions := core.NewBaseCollection("extensions")
	extensions.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 200},
		&core.TextField{Name: "extension_id", Required: true, Max: 200},
		&core.TextField{Name: "version", Max: 100},
		&core.TextField{Name: "description"},
		&core.TextField{Name: "author", Max: 200},
		&core.JSONField{Name: "manifest"},
		&core.JSONField{Name: "permissions"},
		&core.TextField{Name: "install_path", Max: 500},
		&core.TextField{Name: "source", Max: 200},
		&core.BoolField{Name: "enabled"},
		&core.SelectField{
			Name:      "status",
			Values:    []string{"installed", "loaded", "error", "disabled"},
			MaxSelect: 1,
		},
		&core.TextField{Name: "error_message"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnUpdate: true},
	)
	extensions.Indexes = types.JSONArray[string]{
		"CREATE UNIQUE INDEX idx_extensions_extension_id ON extensions (extension_id)",
	}
	if err := app.Save(extensions); err != nil {
		return err
	}

	// --- extension_logs ----------------------------------------------
	// The runtime log of an extension. An extension that throws must not
	// take the application down, and the failure must still be readable
	// after a restart, so this is in PocketBase rather than in the
	// in-memory debug console.
	extensionLogs := core.NewBaseCollection("extension_logs")
	extensionLogs.Fields.Add(
		&core.RelationField{
			Name:          "extension",
			CollectionId:  extensions.Id,
			MaxSelect:     1,
			Required:      true,
			CascadeDelete: true,
		},
		&core.SelectField{
			Name:      "level",
			Values:    []string{"debug", "info", "warn", "error"},
			MaxSelect: 1,
		},
		&core.TextField{Name: "message"},
		&core.JSONField{Name: "detail"},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	extensionLogs.Indexes = types.JSONArray[string]{
		"CREATE INDEX idx_extension_logs_extension ON extension_logs (extension, created)",
	}
	if err := app.Save(extensionLogs); err != nil {
		return err
	}

	// --- audit_log ---------------------------------------------------
	// The operation trail shared by the writing domain, the AI, the
	// tools and the extension runtime. It is append-only by convention:
	// nothing in the application deletes a row.
	auditLog := core.NewBaseCollection("audit_log")
	auditLog.Fields.Add(
		&core.RelationField{
			Name:          "project",
			CollectionId:  projects.Id,
			MaxSelect:     1,
			CascadeDelete: true,
		},
		&core.TextField{Name: "category", Max: 100},
		&core.TextField{Name: "action", Max: 100},
		&core.TextField{Name: "actor", Max: 200},
		&core.TextField{Name: "target_type", Max: 100},
		&core.TextField{Name: "target_id", Max: 100},
		&core.JSONField{Name: "detail"},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	auditLog.Indexes = types.JSONArray[string]{
		"CREATE INDEX idx_audit_log_project ON audit_log (project, created)",
		"CREATE INDEX idx_audit_log_category ON audit_log (category, created)",
		"CREATE INDEX idx_audit_log_target ON audit_log (target_type, target_id)",
	}
	if err := app.Save(auditLog); err != nil {
		return err
	}

	return nil
}

// collectionExists reports whether a collection has already been created.
//
// FindCollectionByNameOrId returns an error both for a missing collection
// and for a real database failure. Asking the collection list
// distinguishes the two: the list only fails when the database does.
func collectionExists(app core.App, name string) (bool, error) {
	collections, err := app.FindAllCollections()
	if err != nil {
		return false, err
	}
	for _, collection := range collections {
		if collection.Name == name {
			return true, nil
		}
	}
	return false, nil
}
