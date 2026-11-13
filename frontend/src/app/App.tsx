/**
 * The application shell.
 *
 * The layout is the Scrivener workspace: the binder on the left, the
 * main view (editor, corkboard or outliner) in the centre, the
 * inspector on the right and the AI panel docked beside it. The bottom
 * console and the settings dialog are the scaffold's debugging aids and
 * stay available.
 *
 * Every change goes through the Wails bindings; the frontend never talks
 * to PocketBase. The binder event keeps the workspace in step with the
 * backend, and React Query owns the cache.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Events } from '@wailsio/runtime';

import '../styles.css';

import * as api from '../bindings';
import type { BinderItem, ItemMeta, ProjectLayout } from '../bindings';
import { applyZoom } from '../ui/zoom';
import { isConsoleShortcut } from '../ui/shortcuts';
import { ConsolePanel, countErrors } from '../features/console/ConsolePanel';
import { SettingsDialog } from '../features/settings/SettingsDialog';
import { BinderPanel } from '../features/binder/BinderPanel';
import { descendantIDs } from '../features/binder/binder';
import { EditorPanel } from '../features/editor/EditorPanel';
import { flushEditors } from '../features/editor/flush';
import { CorkboardPanel } from '../features/corkboard/CorkboardPanel';
import { OutlinerPanel } from '../features/outliner/OutlinerPanel';
import { InspectorPanel } from '../features/inspector/InspectorPanel';
import { metaFromItem } from '../features/inspector/inspector';
import { HistoryDialog } from '../features/history/HistoryDialog';
import { ProjectPicker } from '../features/projects/ProjectPicker';
import { AIPanel } from '../features/ai/AIPanel';

interface Toast {
  message: string;
  kind: 'info' | 'error' | 'success';
}

/** draftRootID finds the manuscript group of a project. */
function draftRootID(items: BinderItem[]): string {
  return items.find((item) => item.parent_id === '' && item.bucket === 'draft')?.id ?? '';
}

/** firstDocumentID returns the first document of the draft, in order. */
function firstDocumentID(items: BinderItem[]): string {
  const docs = items
    .filter((item) => item.kind === 'document' && item.bucket === 'draft' && !item.trashed)
    .sort((a, b) =>
      a.position !== b.position ? a.position - b.position : a.created.localeCompare(b.created),
    );
  return docs[0]?.id ?? '';
}

/**
 * The application root.
 */
function App() {
  const queryClient = useQueryClient();

  // --- Chrome state -------------------------------------------------
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [consoleOpen, setConsoleOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [toast, setToast] = useState<Toast | null>(null);
  const toastTimer = useRef<number | null>(null);

  const notify = useCallback((message: string, kind: Toast['kind'] = 'info') => {
    setToast({ message, kind });
    if (toastTimer.current !== null) {
      window.clearTimeout(toastTimer.current);
    }
    toastTimer.current = window.setTimeout(() => {
      toastTimer.current = null;
      setToast(null);
    }, 4000);
  }, []);

  useEffect(
    () => () => {
      if (toastTimer.current !== null) {
        window.clearTimeout(toastTimer.current);
      }
    },
    [],
  );

  // --- Workspace state ----------------------------------------------
  const [projectId, setProjectId] = useState<string | null>(null);
  const [booted, setBooted] = useState(false);
  const [selectedId, setSelectedId] = useState('');
  const [layout, setLayout] = useState<ProjectLayout | null>(null);
  const [binderQuery, setBinderQuery] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [splitItemId, setSplitItemId] = useState('');

  // True once the close handshake has started. The save that flushes the
  // editors must not start query refetches that would outlive the window.
  const closingRef = useRef(false);

  // The layout and the selection are initialised once per project, from
  // the record, and then written back as the user works.
  const initialisedFor = useRef('');

  const settingsQuery = useQuery({ queryKey: ['settings'], queryFn: () => api.getSettings() });
  const consoleQuery = useQuery({
    queryKey: ['console'],
    queryFn: () => api.listConsoleEntries(),
    refetchInterval: 5000,
  });

  const workspaceQuery = useQuery({
    queryKey: ['workspace', projectId],
    queryFn: () => api.getWorkspace(projectId ?? ''),
    enabled: projectId !== null,
    refetchInterval: 15000,
  });

  const workspace = workspaceQuery.data;

  // --- Boot: reopen the last project --------------------------------
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const last = await api.getLastProjectID();
        if (!cancelled && last) {
          setProjectId(last);
        }
      } catch {
        // A missing setting is not a failure: the picker opens.
      } finally {
        if (!cancelled) {
          setBooted(true);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // --- Initialise the layout and the selection ----------------------
  useEffect(() => {
    if (!workspace) {
      return;
    }
    if (initialisedFor.current === workspace.project.id) {
      return;
    }
    initialisedFor.current = workspace.project.id;

    const stored = workspace.project.layout;
    const expanded = stored.expanded_folder_ids.length
      ? stored.expanded_folder_ids
      : [draftRootID(workspace.items)].filter(Boolean);
    setLayout({ ...stored, expanded_folder_ids: expanded });
    setSplitItemId(stored.split_item_id);

    const remembered = workspace.project.last_open_item_id;
    const rememberedExists = workspace.items.some((item) => item.id === remembered && !item.trashed);
    setSelectedId(
      rememberedExists
        ? remembered
        : draftRootID(workspace.items) || firstDocumentID(workspace.items),
    );
  }, [workspace]);

  // Persist the layout after a short idle pause, so dragging a split or
  // toggling a folder does not write on every frame.
  useEffect(() => {
    if (!layout || !projectId) {
      return;
    }
    const timer = window.setTimeout(() => {
      void api.saveLayout(projectId, layout).catch((error: Error) => notify(error.message, 'error'));
    }, 500);
    return () => window.clearTimeout(timer);
  }, [layout, projectId, notify]);

  // Persist the open item.
  useEffect(() => {
    if (!projectId || !selectedId) {
      return;
    }
    const timer = window.setTimeout(() => {
      void api.setLastOpenItem(projectId, selectedId).catch(() => undefined);
    }, 600);
    return () => window.clearTimeout(timer);
  }, [projectId, selectedId]);

  // --- Live updates -------------------------------------------------
  useEffect(() => {
    const unsubscribe = Events.On(api.EVENT_BINDER_UPDATED, () => {
      void queryClient.invalidateQueries({ queryKey: ['workspace'] });
    });
    return () => {
      unsubscribe();
    };
  }, [queryClient]);

  useEffect(() => {
    const unsubscribe = Events.On(api.EVENT_CONSOLE_ENTRY, () => {
      void queryClient.invalidateQueries({ queryKey: ['console'] });
    });
    return () => {
      unsubscribe();
    };
  }, [queryClient]);

  // The close handshake. The backend cancels the window close and sends
  // app:closing; this saves every dirty editor, then asks the backend to
  // close for real. Without it, the text typed inside the autosave pause
  // is lost, because the webview is destroyed before the timer fires.
  useEffect(() => {
    const unsubscribe = Events.On(api.EVENT_APP_CLOSING, () => {
      closingRef.current = true;
      void flushEditors().finally(() => {
        void api.requestClose().catch(() => undefined);
      });
    });
    return () => {
      unsubscribe();
    };
  }, []);

  // --- Zoom ---------------------------------------------------------
  useEffect(() => {
    if (settingsQuery.data) {
      applyZoom(settingsQuery.data.ui.zoom);
    }
  }, [settingsQuery.data]);

  // The console shortcut is global.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (isConsoleShortcut(event)) {
        event.preventDefault();
        setConsoleOpen((value) => !value);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  // --- Derived selection --------------------------------------------
  const items = workspace?.items ?? [];
  const selectedItem = useMemo(
    () => items.find((item) => item.id === selectedId) ?? null,
    [items, selectedId],
  );

  const documentQuery = useQuery({
    queryKey: ['document', selectedId],
    queryFn: () => api.readDocument(selectedId),
    enabled: selectedItem?.kind === 'document' && !selectedItem.trashed,
  });

  const sectionIDs = useMemo(
    () => (selectedItem && selectedItem.kind === 'folder' ? descendantIDs(items, selectedItem.id) : []),
    [items, selectedItem],
  );

  const scriveningsQuery = useQuery({
    queryKey: ['scrivenings', selectedId, sectionIDs.join('|')],
    queryFn: () => api.readScrivenings(sectionIDs),
    enabled: (layout?.view === 'editor' || layout?.composition) && sectionIDs.length > 0,
  });

  const searchResults = useQuery({
    queryKey: ['search', projectId, searchQuery],
    queryFn: () => api.searchProject(projectId ?? '', searchQuery, 30),
    enabled: projectId !== null && searchQuery.trim().length >= 2,
  });

  const splitDocumentQuery = useQuery({
    queryKey: ['document', splitItemId],
    queryFn: () => api.readDocument(splitItemId),
    enabled:
      layout?.split !== 'none' &&
      splitItemId !== '' &&
      items.some((item) => item.id === splitItemId && item.kind === 'document'),
  });

  const openProject = (id: string) => {
    setProjectId(id);
    setSelectedId('');
    setLayout(null);
    setBinderQuery('');
    setSearchQuery('');
    setPickerOpen(false);
    initialisedFor.current = '';
    void api.setLastProjectID(id).catch(() => undefined);
  };

  // --- Mutations ----------------------------------------------------
  const refreshBinder = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['workspace', projectId] });
    void queryClient.invalidateQueries({ queryKey: ['document'] });
    void queryClient.invalidateQueries({ queryKey: ['scrivenings'] });
  }, [queryClient, projectId]);

  const onMutationError = useCallback(
    (error: Error) => notify(error.message, 'error'),
    [notify],
  );

  const createItemMutation = useMutation({
    mutationFn: (input: { parentId: string; kind: 'folder' | 'document' }) =>
      api.createBinderItem(projectId ?? '', input.parentId, input.kind),
    onSuccess: (item) => {
      setSelectedId(item.id);
      refreshBinder();
    },
    onError: onMutationError,
  });

  const renameItemMutation = useMutation({
    mutationFn: (input: { itemId: string; title: string }) =>
      api.renameBinderItem(input.itemId, input.title),
    onSuccess: refreshBinder,
    onError: onMutationError,
  });

  const updateItemMutation = useMutation({
    mutationFn: (input: { itemId: string; meta: ItemMeta }) =>
      api.updateBinderItem(input.itemId, input.meta),
    onSuccess: refreshBinder,
    onError: onMutationError,
  });

  const moveItemMutation = useMutation({
    mutationFn: (input: { itemId: string; parentId: string; index: number }) =>
      api.moveBinderItem(input.itemId, input.parentId, input.index),
    onSuccess: refreshBinder,
    onError: onMutationError,
  });

  const trashItemMutation = useMutation({
    mutationFn: (itemId: string) => api.trashBinderItem(itemId),
    onSuccess: () => {
      refreshBinder();
      notify('The item was moved to the Trash.');
    },
    onError: onMutationError,
  });

  const restoreItemMutation = useMutation({
    mutationFn: (itemId: string) => api.restoreBinderItem(itemId),
    onSuccess: () => {
      refreshBinder();
      notify('The item was restored.', 'success');
    },
    onError: onMutationError,
  });

  const deleteItemMutation = useMutation({
    mutationFn: (itemId: string) => api.deleteBinderItem(itemId),
    onSuccess: () => {
      setSelectedId('');
      refreshBinder();
      notify('The item was deleted permanently.');
    },
    onError: onMutationError,
  });

  const duplicateItemMutation = useMutation({
    mutationFn: (itemId: string) => api.duplicateBinderItem(itemId),
    onSuccess: (item) => {
      setSelectedId(item.id);
      refreshBinder();
    },
    onError: onMutationError,
  });

  const saveDocumentMutation = useMutation({
    mutationFn: (input: { itemId: string; content: string }) =>
      api.saveDocument(input.itemId, input.content),
    onSuccess: (document) => {
      if (closingRef.current) {
        // The window is closing: the save is the last thing that has to
        // reach the backend, not a query refresh.
        return;
      }
      queryClient.setQueryData(['document', document.item.id], document);
      void queryClient.invalidateQueries({ queryKey: ['workspace', projectId] });
      void queryClient.invalidateQueries({ queryKey: ['scrivenings'] });
      void queryClient.invalidateQueries({ queryKey: ['history', projectId] });
    },
    onError: onMutationError,
  });

  const snapshotMutation = useMutation({
    mutationFn: (message: string) =>
      api.createSnapshot(projectId ?? '', selectedItem ? selectedItem.id : '', message),
    onSuccess: (snapshot) => {
      notify(`Snapshot ${snapshot.commit_hash.slice(0, 7)} recorded.`, 'success');
      void queryClient.invalidateQueries({ queryKey: ['history', projectId] });
      void queryClient.invalidateQueries({ queryKey: ['snapshots', projectId] });
    },
    onError: onMutationError,
  });

  const saveCredentialsMutation = useMutation({
    mutationFn: (input: { apiKey: string; baseUrl: string }) =>
      api.saveCredentials(input.apiKey, input.baseUrl),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
      notify('The credentials were saved.', 'success');
    },
    onError: onMutationError,
  });

  const saveUIMutation = useMutation({
    mutationFn: (ui: { zoom: number; theme: string }) => api.saveUI(ui),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
    onError: onMutationError,
  });

  const clearConsoleMutation = useMutation({
    mutationFn: () => api.clearConsoleEntries(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['console'] });
    },
    onError: onMutationError,
  });

  const saveDocument = useCallback(
    (itemId: string, content: string) =>
      saveDocumentMutation.mutateAsync({ itemId, content }),
    [saveDocumentMutation],
  );

  const patchMeta = useCallback(
    (itemId: string, patch: Partial<ItemMeta>) => {
      const item = items.find((candidate) => candidate.id === itemId);
      if (!item) {
        return;
      }
      updateItemMutation.mutate({ itemId, meta: { ...metaFromItem(item), ...patch } });
    },
    [items, updateItemMutation],
  );

  const saveMeta = useCallback(
    (itemId: string, meta: ItemMeta) => {
      updateItemMutation.mutate({ itemId, meta });
    },
    [updateItemMutation],
  );

  const busy =
    createItemMutation.isPending ||
    renameItemMutation.isPending ||
    updateItemMutation.isPending ||
    moveItemMutation.isPending ||
    trashItemMutation.isPending ||
    restoreItemMutation.isPending ||
    deleteItemMutation.isPending ||
    duplicateItemMutation.isPending;

  const entries = consoleQuery.data ?? [];
  const consoleErrors = countErrors(entries);
  const view = layout?.view ?? 'editor';

  // Leave the composition mode with Escape, like a full-screen surface.
  useEffect(() => {
    if (!layout?.composition) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setLayout({ ...layout, composition: false });
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [layout]);

  const toastView = toast ? (
    <div className={`toast toast-${toast.kind}`} role="status">
      {toast.message}
    </div>
  ) : null;

  if (!booted) {
    return <div className="boot text-dim">Opening your workspace…</div>;
  }

  if (!projectId || pickerOpen || workspaceQuery.error) {
    return (
      <div className="boot-picker">
        <ProjectPicker onOpen={openProject} notify={notify} />
        {toastView}
      </div>
    );
  }

  if (!workspace || !layout) {
    return <div className="boot text-dim">Loading the project…</div>;
  }

  const project = workspace.project;
  const documents = items.filter((item) => item.kind === 'document' && !item.trashed);

  const renderEditor = (
    paneItem: BinderItem | null,
    paneDocument: typeof documentQuery.data | null,
    paneScrivenings: typeof scriveningsQuery.data | null,
    loading: boolean,
  ) => (
    <EditorPanel
      item={paneItem}
      document={paneDocument ?? null}
      scrivenings={paneScrivenings ?? null}
      loading={loading}
      saving={saveDocumentMutation.isPending}
      onOpenItem={(itemId) => {
        setSelectedId(itemId);
        setLayout((current) => (current ? { ...current, view: 'editor' } : current));
      }}
      onSave={saveDocument}
      onSnapshot={(message) => snapshotMutation.mutate(message)}
    />
  );

  if (layout.composition) {
    return (
      <div className="composition">
        <button
          type="button"
          className="btn btn-sm composition-exit"
          onClick={() => setLayout({ ...layout, composition: false })}
        >
          Exit composition (Esc)
        </button>
        <div className="composition-pane">
          {renderEditor(
            selectedItem,
            documentQuery.data ?? null,
            selectedItem?.kind === 'folder' ? (scriveningsQuery.data ?? null) : null,
            documentQuery.isFetching && !documentQuery.data,
          )}
        </div>
        {toastView}
      </div>
    );
  }

  return (
    <div
      className={`app${layout.inspector_open ? ' with-inspector' : ''}${
        layout.ai_open ? ' with-ai' : ''
      }${consoleOpen ? ' with-console' : ''}`}
    >
      <header className="app-header">
        <button type="button" className="app-brand" onClick={() => setPickerOpen(true)} title="Switch project">
          Markuccino
        </button>
        <span className="app-project" title="The open project">
          {project.name}
          <span className="text-dim"> · {project.word_count} words</span>
        </span>

        <nav className="view-tabs" aria-label="View">
          {(['editor', 'corkboard', 'outliner'] as const).map((name) => (
            <button
              key={name}
              type="button"
              className={`view-tab${view === name ? ' is-active' : ''}`}
              aria-pressed={view === name}
              onClick={() => setLayout({ ...layout, view: name })}
            >
              {name === 'editor' ? 'Editor' : name === 'corkboard' ? 'Corkboard' : 'Outliner'}
            </button>
          ))}
        </nav>

        <button
          type="button"
          className={`btn btn-sm${layout.split !== 'none' ? ' btn-active' : ''}`}
          title="Toggle the split editor"
          onClick={() => {
            const nextSplit = layout.split === 'none' ? 'vertical' : 'none';
            setLayout({ ...layout, split: nextSplit });
            if (nextSplit !== 'none' && !splitItemId) {
              setSplitItemId(selectedItem?.kind === 'document' ? selectedItem.id : (documents[0]?.id ?? ''));
            }
          }}
        >
          Split
        </button>

        <button
          type="button"
          className="btn btn-sm"
          title="Enter the distraction-free composition mode"
          onClick={() => setLayout({ ...layout, composition: true })}
        >
          Compose
        </button>

        <span className="app-header-spacer" />

        <div className="app-search">
          <input
            type="search"
            value={searchQuery}
            placeholder="Search the project…"
            aria-label="Search the project"
            onChange={(event) => setSearchQuery(event.target.value)}
          />
          {searchQuery.trim().length >= 2 && (
            <ul className="search-results">
              {(searchResults.data ?? []).map((hit) => (
                <li key={`${hit.item_id}-${hit.field}`}>
                  <button
                    type="button"
                    onClick={() => {
                      setSelectedId(hit.item_id);
                      setSearchQuery('');
                      setLayout((current) => (current ? { ...current, view: 'editor' } : current));
                    }}
                  >
                    <strong>{hit.title}</strong>
                    <span className="text-dim"> · {hit.field}</span>
                    <p>{hit.excerpt}</p>
                  </button>
                </li>
              ))}
              {searchResults.data?.length === 0 && !searchResults.isFetching && (
                <li className="text-dim">No match.</li>
              )}
            </ul>
          )}
        </div>

        <label className="app-goal" title="The draft word target">
          <span className="text-dim">Goal</span>
          <input
            key={project.id}
            type="number"
            min={0}
            step={1000}
            defaultValue={Number(project.settings.word_target ?? 0) || ''}
            aria-label="Word target"
            onBlur={(event) => {
              const target = Math.max(0, Math.floor(Number(event.target.value) || 0));
              void api
                .saveProjectSettings(projectId, { ...project.settings, word_target: target })
                .then(() => queryClient.invalidateQueries({ queryKey: ['workspace', projectId] }))
                .catch((error: Error) => notify(error.message, 'error'));
            }}
          />
        </label>

        <button type="button" className="btn btn-sm" onClick={() => setHistoryOpen(true)}>
          History
        </button>
        <button
          type="button"
          className={`btn btn-sm${layout.inspector_open ? ' btn-active' : ''}`}
          aria-pressed={layout.inspector_open}
          title="Show or hide the inspector"
          onClick={() => setLayout({ ...layout, inspector_open: !layout.inspector_open })}
        >
          Inspector
        </button>
        <button
          type="button"
          className={`btn btn-sm${layout.ai_open ? ' btn-active' : ''}`}
          aria-pressed={layout.ai_open}
          title="Show or hide the AI panel"
          onClick={() => setLayout({ ...layout, ai_open: !layout.ai_open })}
        >
          AI
        </button>
        <button
          type="button"
          className={`btn btn-sm${consoleOpen ? ' btn-active' : ''}`}
          onClick={() => setConsoleOpen((value) => !value)}
          title="Show or hide the debug console (Ctrl+Shift+I)"
          aria-pressed={consoleOpen}
        >
          Console
          {consoleErrors > 0 && <span className="console-error-count"> · {consoleErrors}</span>}
        </button>
        <button type="button" className="btn btn-sm" onClick={() => setSettingsOpen(true)}>
          Settings
        </button>
      </header>

      <BinderPanel
        items={items}
        selectedId={selectedId}
        expanded={new Set(layout.expanded_folder_ids)}
        query={binderQuery}
        busy={busy}
        onQueryChange={setBinderQuery}
        onSelect={(item) => setSelectedId(item.id)}
        onToggle={(itemId) => {
          const expanded = new Set(layout.expanded_folder_ids);
          if (expanded.has(itemId)) {
            expanded.delete(itemId);
          } else {
            expanded.add(itemId);
          }
          setLayout({ ...layout, expanded_folder_ids: [...expanded] });
        }}
        onMove={(itemId, parentId, index) => moveItemMutation.mutate({ itemId, parentId, index })}
        onCreate={(parentId, kind) => createItemMutation.mutate({ parentId, kind })}
        onRename={(itemId, title) => renameItemMutation.mutate({ itemId, title })}
        onDuplicate={(itemId) => duplicateItemMutation.mutate(itemId)}
        onTrash={(itemId) => trashItemMutation.mutate(itemId)}
        onRestore={(itemId) => restoreItemMutation.mutate(itemId)}
        onDelete={(itemId) => deleteItemMutation.mutate(itemId)}
      />

      <main className="workspace-main">
        {view === 'editor' && (
          <div className={`workspace-split is-${layout.split}`}>
            <div className="workspace-pane">
              {renderEditor(
                selectedItem,
                documentQuery.data ?? null,
                selectedItem?.kind === 'folder' ? (scriveningsQuery.data ?? null) : null,
                documentQuery.isFetching && !documentQuery.data,
              )}
            </div>

            {layout.split !== 'none' && (
              <div className="workspace-pane">
                <div className="pane-picker">
                  <label className="field field-inline">
                    <span className="field-label">Second pane</span>
                    <select
                      value={splitItemId}
                      onChange={(event) => {
                        setSplitItemId(event.target.value);
                        setLayout({ ...layout, split_item_id: event.target.value });
                      }}
                    >
                      <option value="">Choose a document…</option>
                      {documents.map((item) => (
                        <option key={item.id} value={item.id}>
                          {item.title}
                        </option>
                      ))}
                    </select>
                  </label>
                </div>
                {renderEditor(
                  items.find((item) => item.id === splitItemId) ?? null,
                  splitDocumentQuery.data ?? null,
                  null,
                  splitDocumentQuery.isFetching && !splitDocumentQuery.data,
                )}
              </div>
            )}
          </div>
        )}

        {view === 'corkboard' && (
          <CorkboardPanel
            items={items}
            selectedId={selectedId}
            busy={busy}
            onSelect={setSelectedId}
            onOpen={(itemId) => {
              setSelectedId(itemId);
              setLayout({ ...layout, view: 'editor' });
            }}
            onSynopsis={(itemId, synopsis) => patchMeta(itemId, { synopsis })}
            onMove={(itemId, parentId, index) => moveItemMutation.mutate({ itemId, parentId, index })}
          />
        )}

        {view === 'outliner' && (
          <OutlinerPanel
            items={items}
            selectedId={selectedId}
            busy={busy}
            target={Number(project.settings.word_target ?? 0) || 0}
            onSelect={setSelectedId}
            onOpen={(itemId) => {
              setSelectedId(itemId);
              setLayout({ ...layout, view: 'editor' });
            }}
            onMeta={patchMeta}
          />
        )}
      </main>

      {layout.inspector_open && (
        <InspectorPanel
          projectId={projectId}
          item={selectedItem}
          busy={busy}
          onMeta={saveMeta}
          onSnapshot={(message) => snapshotMutation.mutate(message)}
          onOpenHistory={() => setHistoryOpen(true)}
          notify={notify}
        />
      )}

      {layout.ai_open && (
        <AIPanel
          projectName={project.name}
          item={selectedItem}
          onCollapse={() => setLayout({ ...layout, ai_open: false })}
          onOpenHistory={() => setHistoryOpen(true)}
        />
      )}

      {consoleOpen && (
        <ConsolePanel entries={entries} onClear={() => clearConsoleMutation.mutate()} />
      )}

      <HistoryDialog
        open={historyOpen}
        projectId={projectId}
        item={selectedItem}
        onClose={() => setHistoryOpen(false)}
        onChanged={(message) => {
          refreshBinder();
          notify(message, 'success');
        }}
        notify={notify}
      />

      <SettingsDialog
        open={settingsOpen}
        settings={settingsQuery.data ?? null}
        onClose={() => setSettingsOpen(false)}
        onSave={(apiKey, baseUrl) => saveCredentialsMutation.mutate({ apiKey, baseUrl })}
        onSaveUI={(ui) => saveUIMutation.mutate(ui)}
      />

      {toastView}
    </div>
  );
}

export default App;
