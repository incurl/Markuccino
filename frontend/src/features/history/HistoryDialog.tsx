/**
 * The history browser: the project's revisions, the diff of a revision
 * for the selected document, and the restore actions.
 *
 * A revision is a Git commit. Restoring writes the old content back and
 * commits it as a new revision, so the history stays append-only and a
 * restore can itself be undone.
 */

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as api from '../../bindings';
import type { BinderItem } from '../../bindings';

interface HistoryDialogProps {
  open: boolean;
  projectId: string;
  /** The selected item, when a document is selected. */
  item: BinderItem | null;
  onClose: () => void;
  onChanged: (message: string) => void;
  notify: (message: string, kind?: 'info' | 'error' | 'success') => void;
}

/**
 * The history dialog.
 */
export function HistoryDialog({ open, projectId, item, onClose, onChanged, notify }: HistoryDialogProps) {
  const queryClient = useQueryClient();
  const [selectedHash, setSelectedHash] = useState('');

  const historyQuery = useQuery({
    queryKey: ['history', projectId],
    queryFn: () => api.listProjectHistory(projectId, 100),
    enabled: open,
    refetchInterval: open ? 10000 : false,
  });

  const diffQuery = useQuery({
    queryKey: ['history-diff', projectId, item?.id ?? '', selectedHash],
    queryFn: () => api.diffRevision(projectId, item ? item.id : '', selectedHash),
    enabled: open && selectedHash !== '' && item !== null && item.kind === 'document',
  });

  const invalidation = () => {
    void queryClient.invalidateQueries({ queryKey: ['workspace', projectId] });
    void queryClient.invalidateQueries({ queryKey: ['document'] });
    void queryClient.invalidateQueries({ queryKey: ['history', projectId] });
    void queryClient.invalidateQueries({ queryKey: ['snapshots', projectId] });
  };

  const restoreDocument = useMutation({
    mutationFn: (hash: string) => api.restoreDocument(item ? item.id : '', hash),
    onSuccess: () => {
      invalidation();
      onChanged('The document was restored. The restore is a new revision.');
    },
    onError: (error: Error) => notify(error.message, 'error'),
  });

  const restoreProjectMutation = useMutation({
    mutationFn: (hash: string) => api.restoreProject(projectId, hash),
    onSuccess: () => {
      invalidation();
      onChanged('The project was restored. The restore is a new revision.');
    },
    onError: (error: Error) => notify(error.message, 'error'),
  });

  if (!open) {
    return null;
  }

  const revisions = historyQuery.data ?? [];
  const selected = revisions.find((revision) => revision.hash === selectedHash);

  return (
    <div className="dialog-backdrop" role="presentation" onClick={onClose}>
      <section
        className="dialog history-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="History"
        onClick={(event) => event.stopPropagation()}
      >
        <header className="dialog-header">
          <h2>History</h2>
          <button type="button" className="btn btn-sm" onClick={onClose}>
            Close
          </button>
        </header>

        <div className="history-body">
          <ul className="history-list">
            {revisions.map((revision) => (
              <li key={revision.hash}>
                <button
                  type="button"
                  className={`history-entry${revision.hash === selectedHash ? ' is-selected' : ''}`}
                  onClick={() => setSelectedHash(revision.hash)}
                >
                  <strong>{revision.message}</strong>
                  <span className="text-dim">
                    {revision.hash.slice(0, 7)} · {revision.author} ·{' '}
                    {new Date(revision.when).toLocaleString()}
                  </span>
                  {revision.files.length > 0 && (
                    <span className="history-files text-dim">{revision.files.join(', ')}</span>
                  )}
                </button>
              </li>
            ))}
            {revisions.length === 0 && (
              <li className="text-dim">{historyQuery.isLoading ? 'Loading…' : 'No revisions yet.'}</li>
            )}
          </ul>

          <div className="history-detail">
            {selected ? (
              <>
                <h3 className="editor-title">{selected.message}</h3>
                <p className="text-dim">
                  {selected.hash} · parent {selected.parent ? selected.parent.slice(0, 7) : '—'}
                </p>

                {item && item.kind === 'document' ? (
                  <pre className="diff-view">
                    {diffQuery.isLoading
                      ? 'Loading the diff…'
                      : diffQuery.data?.text || 'This revision did not change the selected document.'}
                  </pre>
                ) : (
                  <p className="text-dim">
                    Select a document in the binder to see its diff for a revision.
                  </p>
                )}

                <div className="inspector-actions">
                  <button
                    type="button"
                    className="btn btn-sm btn-primary"
                    disabled={!item || item.kind !== 'document' || restoreDocument.isPending}
                    onClick={() => restoreDocument.mutate(selected.hash)}
                  >
                    Restore this document
                  </button>
                  <button
                    type="button"
                    className="btn btn-sm"
                    disabled={restoreProjectMutation.isPending}
                    onClick={() => restoreProjectMutation.mutate(selected.hash)}
                  >
                    Restore the project
                  </button>
                </div>
              </>
            ) : (
              <p className="text-dim">Select a revision to inspect it.</p>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
