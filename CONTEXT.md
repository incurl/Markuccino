# Valkyrie vocabulary

The terms below are shared between the Go backend, the React frontend and the documentation. The same terms appear in code as
collection names (`jobs`, `artifacts`, `app_settings`) and as JSON field names.

Use the **Avoid** synonyms as a signal that the term is not what you want.

## Language

**Job**:
A unit of work in the queue. It carries a title, a prompt, arbitrary parameters and an arbitrary result. It says nothing about
what the work actually is.
_Avoid_: Task, Request, Generation, Operation

**Runner**:
The implementation that executes a job. It is the extension point of the scaffold: an application replaces the shipped echo
runner with its own. A runner receives a `jobs.Job` and a `context.Context`, and returns a `jobs.Result` or an error.
_Avoid_: Worker, Executor, Handler, Processor

**Echo runner**:
The reference runner in `internal/runner/echo.go`. It waits briefly, honours the context, and returns the prompt back. It exists
so the scaffold runs with no external service and so the queue can be exercised end to end.
_Avoid_: Mock runner, Test runner, Default runner

**Reconcile**:
The startup pass that resolves the jobs a crash left behind, marking them failed or re-enqueueing them. The scaffold does not
do this yet: a job left in `running` is never resumed and cannot be deleted.
_Avoid_: Recover, Replay, Resume

**Retry**:
The action that runs a finished job again with the same parameters. It creates a **new** job; it must not take the original's
outputs with it.
_Avoid_: Repeat, Rerun, Restart

**Job status**:
The lifecycle state of a job: `pending`, `running`, `completed`, `failed` or `cancelled`. It is a fixed enumeration, not a free
string.
_Avoid_: State, Phase, Stage

**Artifact**:
A stored file, either one the user supplied (`input`) or one the job produced (`output`). It carries the bytes, the detected
media type and the dimensions when the content is an image.
_Avoid_: Asset, File, Upload, Attachment, Resource

**Artifact kind**:
Whether an artifact is an `input` or an `output`. It is stored in the `artifacts.kind` select field, so adding a kind means
changing the migration.
_Avoid_: Direction, Role, Type, Category

**Job title**:
The short label of a job, shown in the queue. It defaults to the first line of the prompt, truncated.
_Avoid_: Name, Label, Summary, Subject

**Job prompt**:
The free-form instruction of a job. The scaffold does not interpret it; the runner does.
_Avoid_: Query, Message, Input, Description

**Job params**:
The arbitrary `map[string]any` of parameters of a job. The scaffold stores them verbatim and hands them to the runner.
_Avoid_: Options, Settings, Config, Arguments, Payload

**Job result**:
The arbitrary `map[string]any` a successful job produced, stored verbatim and returned to the frontend.
_Avoid_: Output, Response, Payload, Data

**Settings**:
The key/value preferences in the `app_settings` collection. A setting is either plain or encrypted.
_Avoid_: Preferences, Config, Configuration, Options

**Secret**:
A setting stored encrypted, such as the API key. It is written and read only by the Go backend, and the frontend learns only
whether one is present.
_Avoid_: Credential, Token, API key (when used for the stored value)

**Fingerprint**:
A short, non-reversible digest of a secret, shown in the debug console so that two keys can be told apart without revealing
either.
_Avoid_: Hash, Mask, Redacted key

**Debug console**:
The in-memory log of the application traffic, shown in a panel at the bottom of the window. It is capped at 500 entries and is
not persisted: it is a debugging aid, not an audit trail. It is hidden until it is asked for, with Ctrl+Shift+I or with the
button in the header.
_Avoid_: Log viewer, Audit log, DevTools

**Console entry**:
One record of the debug console: a level, a source, a message and an optional verbatim detail.
_Avoid_: Log line, Record

**Binding**:
A TypeScript wrapper around the Wails-generated bindings, exported from `frontend/src/bindings/index.ts`. It is the only place
the frontend is allowed to import generated code from.
_Avoid_: Generated binding (that is the file under `frontend/bindings/`), API client, Service

**Generated binding**:
The machine-written TypeScript under `frontend/bindings/`, produced by `wails3 generate bindings`. It is gitignored and must
never be edited by hand.
_Avoid_: Wrapper, Client, Types

**Wails service**:
A Go type bound to the frontend, exposed through `application.NewService(...)` in `main.go`. There is one: `AppService`.
_Avoid_: Controller, Handler, API

**Event**:
A Wails event that pushes a backend change to the frontend. Two exist: `job:updated` and `console:entry`. Both are registered
in `main.go` with `application.RegisterEvent`, which is what makes the generator emit a typed TS API for them.
_Avoid_: Message, Signal, Subscription

**Data directory**:
The directory holding the database, the stored files and the encryption key. It is `pb_data` next to the executable, and
`VALKYRIE_DATA_DIR` relocates it.
_Avoid_: Database directory, Storage path, App data

**Migration**:
A Go function registered with PocketBase's migration list, which creates or evolves the schema. The file name is the identity
and the ordering key.
_Avoid_: Schema file, Upgrade, Patch

**Server mode**:
The headless build of the same binary, compiled with `-tags server`. It serves the frontend and the PocketBase API over HTTP
with no GUI dependencies.
_Avoid_: Web mode, Headless mode, Docker mode

## The writing workspace

**Project**:
A writing workspace: one `projects` record, one directory under `projects/` in the data directory, and one local Git
repository. Documents, metadata and history are all scoped to it.
_Avoid_: Book, Document (for the project), Workspace, Repository

**Binder item**:
One node of the ring binder: a folder or a document. It carries the title, the synopsis, the notes, the keywords, the label,
the status, the custom metadata, the counts and the position among its siblings.
_Avoid_: Node, File, Entry, Card

**Folder**:
A binder item that contains other items and has no body of its own.
_Avoid_: Group, Container, Directory

**Document**:
A binder item with a Markdown body. The body is a file; the metadata is the record.
_Avoid_: Section, Text, File, Chapter (a chapter is a title, not a kind)

**Bucket**:
The binder area an item belongs to: `draft` (the Manuscript), `research`, or `trash`. It is stored on the item, not derived
from the path, and it travels with a moved subtree.
_Avoid_: Area, Zone, Root, Group

**Manuscript**:
The `draft` bucket: the items that compile, count towards the word total and appear in the outliner.
_Avoid_: Draft folder, Main text, Body

**Trash**:
The `trash` bucket. Trashing is a metadata move that remembers where the item came from; the file never moves. A trashed item
is invisible to the editor, the search and the counts, and only a trashed item can be deleted permanently.
_Avoid_: Bin, Recycle, Archive

**Position**:
The integer that orders an item among its siblings. A move renumbers the affected siblings, so the order is always dense.
_Avoid_: Index, Rank, Order, Sort key

**Markdown body**:
The document text, stored as `documents/<item id>.md` inside the project. It is the only document format; there is no RTF or
rich-text blob.
_Avoid_: Content, Text, RTF, Body blob

**Scrivenings**:
The view that merges the documents below a folder into one reading and writing surface, with the sections still saved
separately.
_Avoid_: Merged view, Continuous mode, Draft view

**Composition mode**:
The full-screen writing surface that hides the binder and the inspector. Exiting restores the previous layout.
_Avoid_: Focus mode, Zen mode, Full screen

**Snapshot**:
A Git commit plus the `snapshots` row that explains it: message, author, trigger, changed files and time. The commit holds
the content; the row is the queryable index.
_Avoid_: Save point, Backup, Version (a version is a revision)

**Revision**:
One commit as the history panel shows it: hash, parent, message, author, time and the changed files.
_Avoid_: Version, Snapshot (a snapshot is the recorded pair), Commit (in UI text)

**Trigger**:
Why a snapshot exists: `manual`, `auto`, `edit`, `structural`, `compile`, `ai` or `restore`.
_Avoid_: Reason, Cause, Source

**Restore**:
Writing the content of a revision back and committing it as a new revision. It never rewrites the history, so a restore is
itself undoable.
_Avoid_: Revert, Rollback, Reset

**Reference sheet**:
A character, location or item record with a name, a summary, notes, free-form fields and links to documents.
_Avoid_: Card, Note, Entity, Character sheet (it covers all three kinds)

**Layout**:
The restored session state of a project: the active view, the split mode and document, the panel state, the expanded folders
and the composition flag. It lives in the project record.
_Avoid_: Session, UI state, Preferences
