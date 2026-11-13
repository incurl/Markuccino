/**
 * The AI panel.
 *
 * The panel is mounted at the shell level, so switching the binder view
 * or the document never unmounts it. This build ships the panel, the
 * context binding and the persisted session schema; the tool-calling
 * engine and the model connection are the next phase, and the panel
 * says so rather than pretending to be a chat that goes nowhere.
 *
 * The context brief is real: it is exactly what the editor would send to
 * the assistant — the active document, its binder location, its metadata
 * and its counts.
 */

import { useMemo, useState } from 'react';

import type { BinderItem } from '../../bindings';
import { writeClipboard } from '../console/ConsolePanel';

interface AIPanelProps {
  projectName: string;
  /** The selected binder item, or null. */
  item: BinderItem | null;
  onCollapse: () => void;
  onOpenHistory: () => void;
}

/**
 * contextBrief builds the text the assistant receives with a message.
 *
 * It is deliberately plain text: it is what the user can copy into any
 * assistant today, and what the Go tool-calling engine will assemble
 * once the session layer is wired.
 */
export function contextBrief(projectName: string, item: BinderItem | null): string {
  if (!item) {
    return `Project: ${projectName}\nActive item: none`;
  }
  const lines = [
    `Project: ${projectName}`,
    `Active item: ${item.title} (${item.kind}, ${item.bucket})`,
    `Words: ${item.word_count} · Characters: ${item.char_count}`,
  ];
  if (item.synopsis) {
    lines.push(`Synopsis: ${item.synopsis}`);
  }
  if (item.notes) {
    lines.push(`Notes: ${item.notes}`);
  }
  if (item.keywords.length > 0) {
    lines.push(`Keywords: ${item.keywords.join(', ')}`);
  }
  if (item.label) {
    lines.push(`Label: ${item.label}`);
  }
  if (item.status) {
    lines.push(`Status: ${item.status}`);
  }
  return lines.join('\n');
}

/**
 * The AI panel.
 */
export function AIPanel({ projectName, item, onCollapse, onOpenHistory }: AIPanelProps) {
  const [copied, setCopied] = useState(false);
  const brief = useMemo(() => contextBrief(projectName, item), [projectName, item]);

  return (
    <aside className="ai-panel" aria-label="AI assistant">
      <div className="ai-header">
        <span className="ai-title">AI assistant</span>
        <button
          type="button"
          className="binder-row-action"
          title="Collapse the AI panel"
          onClick={onCollapse}
        >
          ›
        </button>
      </div>

      <div className="ai-body">
        <section className="ai-context">
          <h3 className="ai-section-title">Editor context</h3>
          <pre className="ai-brief">{brief}</pre>
          <div className="ai-context-actions">
            <button
              type="button"
              className="btn btn-sm"
              onClick={() => {
                void writeClipboard(brief).then((ok) => setCopied(ok));
              }}
            >
              {copied ? 'Copied' : 'Copy context'}
            </button>
            <button type="button" className="btn btn-sm" onClick={onOpenHistory}>
              History
            </button>
          </div>
        </section>

        <section className="ai-conversation" aria-live="polite">
          <h3 className="ai-section-title">Conversation</h3>
          <div className="ai-message ai-message-system">
            <p>
              The assistant is not connected in this build. The panel stays mounted across views,
              the context above is live, and the session, message and tool-call tables are in place
              in PocketBase.
            </p>
            <p className="text-dim">
              The next phase wires an OpenAI-compatible model and the built-in writing tools
              (create and move documents, read and replace text, update metadata, search, snapshot,
              compile), then the MCP server as a second tool source.
            </p>
          </div>
        </section>

        <form
          className="ai-composer"
          onSubmit={(event) => {
            event.preventDefault();
          }}
        >
          <textarea
            rows={3}
            placeholder="Ask the assistant… (not connected yet)"
            aria-label="Message the assistant"
            disabled
          />
          <button type="submit" className="btn btn-sm btn-primary" disabled>
            Send
          </button>
        </form>
      </div>
    </aside>
  );
}
