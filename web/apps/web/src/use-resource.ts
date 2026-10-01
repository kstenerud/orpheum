// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { useCallback, useEffect, useState } from "react";

/**
 * Something loaded from the server: still loading, loaded, or failed with why. A failure keeps
 * its cause, which a screen tells the person in their words (problemText), and the console is
 * told it as it is (reportFailure).
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
 * keeps what it shows meanwhile: a reload does not go back to loading, so a view that follows
 * something moving (the jobs, a device code waiting to be used) does not flicker. A new key always
 * starts from loading. This is the one way the client polls the server.
 */
export function useResource<T>(
  key: string,
  load: () => Promise<T>,
  refreshEveryMs?: number,
): [Resource<T>, () => void] {
  const [held, setHeld] = useState<{ readonly key: string; readonly resource: Resource<T> }>({
    key,
    resource: { state: "loading" },
  });
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let isCurrent = true;
    const ask = (): void => {
      load().then(
        (value) => {
          if (isCurrent) {
            setHeld({ key, resource: { state: "loaded", value } });
          }
        },
        (cause: unknown) => {
          reportFailure(cause);
          if (isCurrent) {
            setHeld({ key, resource: { state: "failed", cause } });
          }
        },
      );
    };
    if (refreshEveryMs === undefined) {
      setHeld({ key, resource: { state: "loading" } });
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
  return [held.key === key ? held.resource : { state: "loading" }, reload];
}
