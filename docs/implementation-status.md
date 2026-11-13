# Implementation status

This document records what the Scrivener-style writing workspace on top of
the Valkyrie template actually does today, the assumptions it makes, and
what is deliberately still missing. It is the contract for the next phase:
nothing below is "coming soon" unless it is listed under **Deferred**.

Last updated: 2026-11-13.

## Implemented

### Projects and the binder

- **Projects** (`internal/project/store.go`): create, list, rename, delete.
  A project is a PocketBase record, a workspace directory and a local Git
  repository. `DeleteProject` removes all three.
- **Binder tree** (`internal/project/items.go`): folders and documents,
  arbitrary nesting, integer sibling order, create, rename, duplicate,
  move (drag and drop or by index), trash, restore and permanent delete.
  The three top-level groups — Manuscript, Research and Trash — are
  created with the project.
- **Ordering** is a `position` integer renumbered on every move, so the
  order is dense and stable. A move validates against cycles: an item can
  never enter its own subtree.
- **Trash** remembers `original_parent` and `original_position`, so a
  restore returns the item to where it came from. If that parent is gone,
  the restore falls back to the Manuscript.
- **Duplicate** copies the subtree, the metadata and the Markdown bodies.
  Each copy gets its own file, because the file name is the record id.

### Documents

- Document bodies are **Markdown files** at
  `<data dir>/projects/<project id>/documents/<item id>.md`.
  The path is stored on the binder record (`file_path`); the file name is
  the record id so a rename never moves a file.
- `SaveDocument` writes through a temporary file and a rename, updates the
  word and character counts, commits the revision and appends an audit
  row. The editor autosaves after 900 ms of idle time, on `Ctrl+S`, and on
  unmount, so switching documents inside the pause does not lose a
  sentence.
- **Word counts** strip the Markdown syntax (`internal/project/markdown.go`),
  so `#`, `**` and link targets do not inflate the count.
- **Scrivenings**: selecting a folder opens every document below it, in
  reading order, as separately saved sections (`ReadScrivenings` /
  `SaveScrivenings`). The boundary is the section header, not a synthetic
  separator inside the text.
- **Find/replace** is client-side over the open buffer, with a match count
  and replace-all.

### Views

- **Binder** (`frontend/src/features/binder/`): tree, expand/collapse,
  filtering, drag-and-drop, inline rename, per-row restore/delete in the
  trash, word counts.
- **Corkboard** (`frontend/src/features/corkboard/`): one card per child
  of the selected folder (or per sibling of the selected document), with
  the synopsis editable on the card and drag-and-drop reordering.
- **Outliner** (`frontend/src/features/outliner/`): a sortable table with
  title, synopsis, label, status, word count and compile inclusion, all
  edited inline, with a target progress bar when a word goal is set.
- **Editor** (`frontend/src/features/editor/`): Markdown textarea,
  formatting toolbar (bold, italic, code, headings, quote, lists),
  find/replace, live counts, reading time, split view and a per-document
  or per-project snapshot button.
- **Inspector** (`frontend/src/features/inspector/`): synopsis, notes,
  keywords, label, status, compile inclusion, counts, revision, and the
  reference-sheet tab.
- **Composition mode**: a full-screen writing surface with `Esc` to leave,
  restored to the previous layout on exit.
- **Split view**: vertical or horizontal, with an independent document
  picker for the second pane.
- **Search**: titles, synopses, notes and Markdown bodies, with excerpts.
- **Session restore**: the project layout (view, split, panel state,
  expanded folders, split document), the last open item and the open
  project are in PocketBase. Closing and reopening the application returns
  to the same place.

### Reference sheets

- Characters, locations and items, with a name, summary, notes, a
  free-form `key = value` field set and links to documents. Sheets live in
  `reference_sheets`; the link is a JSON array of item ids, so deleting a
  document does not delete the sheet.

### Version control and snapshots

- Every project is a local Git repository (`internal/project/vcs/`).
  The implementation is pure Go (**go-git**), behind the `vcs.Repository`
  interface; the domain never imports go-git.
- Saving a document, creating, renaming, moving, duplicating, trashing,
  restoring or deleting an item all commit. A commit with an unchanged
  working tree returns the existing revision and is not pretended to be
  new.
- Every commit writes a `snapshots` row: hash, parent, message, author,
  trigger (`edit`, `structural`, `manual`, `restore`), changed files and
  time. The Git history is the content; the row is the queryable index.
- **History** lists the revisions newest first, shows a unified diff of a
  revision against its parent for the selected document, and restores the
  document or the whole project. A restore is a new commit, so it is
  itself undoable.
- The diff is computed in Go with a bounded LCS (`internal/project/diff.go`).
  A file over 2,000 lines reports that it is too large to diff inline
  rather than freezing the window.

### Data and audit

- The schema (`internal/pocketbase/migrations/1732000000_scrivener.go`)
  covers every metadata category the specification asks for: projects,
  binder items, reference sheets, snapshots, compile presets, AI sessions,
  AI messages, tool calls, extensions, extension logs and an append-only
  audit log, with indexes on the lookup fields.
- These collections have **no access rules**. The frontend never talks to
  PocketBase, so the Go backend is the only reader and writer, and the
  writing data is not exposed by the server-mode HTTP API.
- Every domain operation appends to `audit_log` (`category`, `action`,
  `actor`, target and detail). A failed audit write is logged to the debug
  console, not raised: the writing is more important than its record.

### Shutdown and lifecycle

- **Closing is a save-then-close handshake.** The first window close is
  cancelled, `app:closing` is emitted, every mounted editor flushes its
  dirty buffer (`frontend/src/features/editor/flush.ts`), and the
  frontend calls `RequestClose`. If no answer arrives, a three-second
  fallback closes the window, so it can never become unclosable. This
  fixes the data loss the harness found: text typed inside the 900 ms
  autosave pause used to disappear.
- **The event bridge stops before the webview is destroyed.** A save or
  a cancelled job that finishes during teardown must not emit into a
  destroyed webview; that is a native crash, not an error. `bridge.Stop()`
  runs in the close hook and again in `OnShutdown`.
- **PocketBase is closed in `PostShutdown`**, after the windows are gone,
  so the WAL is checkpointed and the cron ticker stops instead of being
  left to the process exit. No binding call can race the close.
- **The DMA-BUF renderer is disabled on virtual GPUs.** WebKitGTK on
  VMware/VirtualBox/QEMU can abort in Mesa during webview teardown
  (`context mismatch in svga_surface_destroy`). On those guests the
  application sets `WEBKIT_DISABLE_DMABUF_RENDERER=1`; an explicit value
  always wins. Desktop writing does not need GPU compositing.
- Verified with a synthetic close on the X display: clean close ~120 ms,
  no SVGA teardown warnings, dirty buffers saved at close delays of 50,
  200 and 600 ms and on a double close, all exits 0. See
  [`troubleshooting.md`](troubleshooting.md#shutdown-and-closing).

## Assumptions

These are the decisions where the Scrivener behaviour is either not
observable or has more than one reasonable reading:

1. **The internal format is Markdown only.** There is no RTF, no rich-text
   blob and no Scrivener `.scriv` reader or converter.
2. **The bucket is metadata, not a path.** A document file never moves when
   an item is trashed, restored or moved between the Manuscript and
   Research. This keeps a trash operation a metadata write.
3. **A document is a leaf.** Folders contain folders and documents;
   documents contain text. A document cannot receive children.
4. **Permanent delete requires the Trash first.** The two-step delete is
   the Scrivener metaphor and the safety net. `DeleteItem` refuses an item
   that is not trashed.
5. **Trash contents are excluded from everything.** They are not compiled,
   not searched, not counted and not readable through `ReadDocument`.
6. **Scrivenings saves per section.** The merged view is an editing
   surface over separate documents; there is no synthetic buffer that
   would have to be split back on save.
7. **A folder is selected for Scrivenings, a document for the editor.**
   Selecting a folder with documents opens the merged view; selecting an
   empty folder shows a hint rather than an empty editor.
8. **Snapshots are Git commits.** A manual snapshot on an unchanged tree
   records a new `snapshots` row pointing at the existing commit; it does
   not create an empty commit.
9. **The app-level "last project" lives in `app_settings`**, not in a
   project record: it describes the application, not the project.

## Deferred

The following are specified in the product brief and are **not** in this
build. The schema and the interfaces are in place so they can be added
without a rewrite.

1. **AI engine and MCP server.** The right-side panel exists, is mounted
   across views, collapses without losing state, and shows the live editor
   context that would be sent. No model is connected and no tool is
   executed. `ai_sessions`, `ai_messages` and `tool_calls` are ready, and
   the next step is a tool registry over `internal/project` plus an
   OpenAI-compatible client, with MCP as a second source behind the same
   interface.
2. **Extension runtime.** `extensions` and `extension_logs` exist; the
   sandbox, manifest enforcement, lifecycle and permission policy do not.
3. **Compile and export.** Section assembly, presets and the
   plain-text/Markdown/HTML writers are the next slice; DOCX and PDF are
   explicitly deferred. `compile_presets` exists.
4. **Annotations, comments, footnotes and endnotes.** Markdown conventions
   and sidecar records are the intended representation; the editing UI is
   not built.
5. **Bookmarks, linked documents and navigation history.** The reference
   sheet links are the only cross-reference today.
6. **Outliner drag reordering.** Reordering is done from the binder and the
   corkboard; the outliner sorts but does not reorder rows.
7. **Typewriter scrolling and the fade-out chrome animation** in
   composition mode. The mode itself works; the optional behaviours do
   not.
8. **Remote Git.** No push, pull, branches or merges. The local commit
   history, diff and restore are complete.
9. **Multi-window and multi-project.** One window, one open project. The
   records are project-scoped, so this is a UI limitation, not a schema
   one.

## Verification performed

```bash
# Backend
go test ./internal/... .                 # all packages
go test -race ./internal/... .           # the race detector
go vet ./internal/... .
gofmt -l internal/ main.go main_test.go tools/   # prints nothing
go run ./tools/e2e-check                 # includes the writing pipeline

# Frontend
cd frontend && npm run typecheck && npm test && npm run build

# Desktop build
go build -tags "desktop,production" -o /tmp/valkyrie .
```

The end-to-end check creates a project, saves a chapter, takes a snapshot,
rewrites the chapter, diffs the snapshot, restores it, reads the history
and searches the body. It passes.

`go mod tidy` (run by `wails3 build`) needs the module proxy for go-git's
test-only dependencies. In a normal network environment it succeeds; in
the build environment used for this change the proxy was unavailable, so
the desktop binary was verified with `go build -tags "desktop,production"`
directly, which uses the same flags and produces `bin/valkyrie`.
