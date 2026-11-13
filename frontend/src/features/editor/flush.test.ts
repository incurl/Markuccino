import { describe, expect, it } from 'vitest';

import { flushEditors, registerFlusher, registeredFlushers } from './flush';

describe('flushEditors', () => {
  it('runs every registered flusher, even when one fails', async () => {
    const calls: string[] = [];
    const unregisterA = registerFlusher(async () => {
      calls.push('a');
    });
    const unregisterB = registerFlusher(async () => {
      calls.push('b');
      throw new Error('the save failed');
    });
    const unregisterC = registerFlusher(async () => {
      calls.push('c');
    });

    await expect(flushEditors()).resolves.toBeUndefined();
    expect(calls).toEqual(['a', 'b', 'c']);

    unregisterA();
    unregisterB();
    unregisterC();
    expect(registeredFlushers()).toBe(0);
  });

  it('resolves immediately when no editor is mounted', async () => {
    await expect(flushEditors()).resolves.toBeUndefined();
  });

  it('waits for a slow save before resolving', async () => {
    let finished = false;
    const unregister = registerFlusher(
      () =>
        new Promise<void>((resolve) => {
          setTimeout(() => {
            finished = true;
            resolve();
          }, 10);
        }),
    );
    await flushEditors();
    expect(finished).toBe(true);
    unregister();
  });
});
