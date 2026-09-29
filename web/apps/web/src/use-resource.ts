// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState } from "react";

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
 */
export function useResource<T>(key: string, load: () => Promise<T>): [Resource<T>, () => void] {
  const [resource, setResource] = useState<Resource<T>>({ state: "loading" });
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let isCurrent = true;
    setResource({ state: "loading" });
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
    return () => {
      isCurrent = false;
    };
  }, [key, attempt]);
  return [
    resource,
    () => {
      setAttempt((count) => count + 1);
    },
  ];
}
