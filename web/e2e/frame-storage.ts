// SPDX-License-Identifier: MPL-2.0

/**
 * Gives a browser test's frame a sessionStorage of its own. Every test file runs in a frame of the one
 * test page, and the frames share that tab's sessionStorage, so a mark one file's page kept outlived
 * the file: a cast received (D383 §2) held each later file's Play page in doubt, drawing nothing, and
 * the failure moved from file to file with the order they ran in (2026-11-23). Self-contained, like
 * frame-focus.ts.
 */
export function giveTheFrameItsOwnStorage(view: Window): void {
  const kept = new Map<string, string>();
  const own: Storage = {
    get length() {
      return kept.size;
    },
    clear: () => {
      kept.clear();
    },
    getItem: (key) => kept.get(key) ?? null,
    key: (index) => [...kept.keys()][index] ?? null,
    removeItem: (key) => {
      kept.delete(key);
    },
    setItem: (key, value) => {
      kept.set(key, value);
    },
  };
  Object.defineProperty(view, "sessionStorage", { configurable: true, get: () => own });
}
