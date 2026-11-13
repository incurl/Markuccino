package bindings

import (
	"sync/atomic"

	"github.com/incurl/Markuccino/internal/console"
	"github.com/incurl/Markuccino/internal/jobs"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventJobUpdated is emitted whenever a job changes state. The payload
// is a jobs.Job.
const EventJobUpdated = "job:updated"

// EventConsoleEntry is emitted whenever a debug console entry is added.
// The payload is a console.Entry.
const EventConsoleEntry = "console:entry"

// EventBinderUpdated is emitted whenever a project's binder or a
// document changed. The payload is a BinderEvent.
//
// It carries only the project id: the frontend invalidates the workspace
// query and refetches. Pushing the whole tree would send a large payload
// for a one-character edit and would race with the in-flight query.
const EventBinderUpdated = "binder:updated"

// EventAppClosing asks the frontend to flush its unsaved editors before
// the window closes. The window close is cancelled, the event is sent,
// and the frontend calls RequestClose once the saves have completed.
//
// The event carries a payload because Wails validates the data against
// the registered type: a registered `struct{}` would reject the nil data
// of a payload-less emit and cancel the event.
const EventAppClosing = "app:closing"

// AppClosingEvent is the payload of EventAppClosing.
type AppClosingEvent struct {
	// Reason names what asked for the close, for the log and for future
	// close sources (a menu item, a signal, an update).
	Reason string `json:"reason"`
}

// BinderEvent is the payload of EventBinderUpdated.
type BinderEvent struct {
	// ProjectID is the project whose binder changed.
	ProjectID string `json:"project_id"`
}

// EventBridge pushes job updates to the frontend as Wails events.
//
// The frontend also polls via React Query, so a missed event only delays
// a refresh rather than losing an update.
//
// The bridge is stopped before the application tears the webview down.
// A background goroutine (a job finishing, a save completing) may still
// call an Emit method while the window is being destroyed; emitting into
// a dying webview is a native crash, not an error we can recover from.
// After Stop the emits are dropped, and the next start refetches.
type EventBridge struct {
	app     *application.App
	stopped atomic.Bool
}

// NewEventBridge creates an event bridge.
func NewEventBridge(app *application.App) *EventBridge {
	return &EventBridge{app: app}
}

// Stop makes every later emit a no-op.
//
// It is called first in the shutdown hook, before the running jobs are
// stopped: stopping a job makes it emit its final status.
func (b *EventBridge) Stop() {
	if b == nil {
		return
	}
	b.stopped.Store(true)
}

// EmitJobUpdated implements jobs.EventEmitter.
func (b *EventBridge) EmitJobUpdated(job jobs.Job) {
	if b == nil || b.app == nil || b.stopped.Load() {
		return
	}
	b.app.Event.Emit(EventJobUpdated, job)
}

// EmitConsoleEntry implements console.Emitter.
func (b *EventBridge) EmitConsoleEntry(entry console.Entry) {
	if b == nil || b.app == nil || b.stopped.Load() {
		return
	}
	b.app.Event.Emit(EventConsoleEntry, entry)
}

// EmitBinderUpdated tells the frontend that a project's binder changed.
func (b *EventBridge) EmitBinderUpdated(projectID string) {
	if b == nil || b.app == nil || b.stopped.Load() {
		return
	}
	b.app.Event.Emit(EventBinderUpdated, BinderEvent{ProjectID: projectID})
}

// EmitAppClosing asks the frontend to flush the editors before the window
// closes.
func (b *EventBridge) EmitAppClosing() {
	if b == nil || b.app == nil || b.stopped.Load() {
		return
	}
	b.app.Event.Emit(EventAppClosing, AppClosingEvent{Reason: "window-close"})
}
