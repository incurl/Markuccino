/**
 * The editor flush registry.
 *
 * Closing the window is a two-step handshake: the window close is
 * cancelled, the frontend saves every dirty buffer, and only then does it
 * ask the backend to close the window. The registry is how the shell
 * reaches the buffers, which live inside the editor components.
 */

/** A function that saves one dirty buffer and resolves when it is done. */
export type EditorFlusher = () => Promise<unknown>;

const flushers = new Set<EditorFlusher>();

/** Registers a flusher and returns the function that unregisters it. */
export function registerFlusher(flusher: EditorFlusher): () => void {
  flushers.add(flusher);
  return () => {
    flushers.delete(flusher);
  };
}

/**
 * flushEditors runs every registered flusher.
 *
 * A failing save must not prevent the others, nor the close: the caller
 * gets the settled result and can decide. The buffers that failed stay
 * dirty, so the next attempt (or the unmount save) retries them.
 */
export async function flushEditors(): Promise<void> {
  const pending = Array.from(flushers, (flusher) => flusher());
  await Promise.allSettled(pending);
}

/** registeredFlushers reports how many editors are mounted. */
export function registeredFlushers(): number {
  return flushers.size;
}
