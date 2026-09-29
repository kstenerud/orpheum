// SPDX-License-Identifier: MPL-2.0

import { useCallback, useEffect, useState } from "react";

/**
 * Something loaded from the server: still loading, loaded, or failed with why. A failure keeps
 * its cause, which a screen tells the person in their words (problemText).
 */
export type Resource<T> =
  | { readonly state: "loading" }
  | { readonly state: "loaded"; readonly value: T }
  | { readonly state: "failed"; readonly cause: unknown };

/**
 * Loads a value when the component mounts and whenever key changes, and reloads when the returned
 * function is called. An answer that arrives after the key changed is dropped.
 *
 * Given refreshEveryMs, it also asks again that often for as long as the component shows it, and
 * keeps what it shows meanwhile: a reload or a new key does not go back to loading, so a view that
 * follows something moving (the jobs, a device code waiting to be used) does not flicker. This is
 * the one way the client polls the server.
 */
export function useResource<T>(
  key: string,
  load: () => Promise<T>,
  refreshEveryMs?: number,
): [Resource<T>, () => void] {
  const [resource, setResource] = useState<Resource<T>>({ state: "loading" });
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let isCurrent = true;
    const ask = (): void => {
      load().then(
        (value) => {
          if (isCurrent) {
            setResource({ state: "loaded", value });
          }
        },
        (cause: unknown) => {
          if (isCurrent) {
            setResource({ state: "failed", cause });
          }
        },
      );
    };
    if (refreshEveryMs === undefined) {
      setResource({ state: "loading" });
      ask();
      return () => {
        isCurrent = false;
      };
    }
    ask();
    const refreshTimer = setInterval(ask, refreshEveryMs);
    return () => {
      isCurrent = false;
      clearInterval(refreshTimer);
    };
  }, [key, attempt, refreshEveryMs]);
  const reload = useCallback(() => {
    setAttempt((count) => count + 1);
  }, []);
  return [resource, reload];
}
