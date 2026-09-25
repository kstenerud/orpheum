// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState } from "react";

/** Something loaded from the server: still loading, loaded, or failed with why. */
export type Resource<T> =
  | { readonly state: "loading" }
  | { readonly state: "loaded"; readonly value: T }
  | { readonly state: "failed"; readonly message: string };

/** What a failed load says: the server's message, else what went wrong in the browser. */
export function failure(cause: unknown): Resource<never> {
  if (cause instanceof Error) {
    return { state: "failed", message: cause.message };
  }
  if (typeof cause === "object" && cause !== null && "error" in cause) {
    const detail = cause.error;
    if (
      typeof detail === "object" &&
      detail !== null &&
      "message" in detail &&
      typeof detail.message === "string"
    ) {
      return { state: "failed", message: detail.message };
    }
  }
  return { state: "failed", message: String(cause) };
}

/**
 * Loads a value when the component mounts and whenever key changes, and reloads on retry. An
 * answer that arrives after the key changed is dropped.
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
          setResource(failure(cause));
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
