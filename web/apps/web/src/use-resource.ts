// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { useCallback, useEffect, useState } from "react";

import type { AskOutcome, Poller } from "./polling.ts";

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
 * Given a poller, it also asks again for as long as the component shows it, as the poller's
 * schedule says (D237): backgroundChangesPoller backs off while the answer is unchanged. It keeps what it shows meanwhile: a
 * reload does not go back to loading, so a view that follows something moving (the jobs, a scan's
 * films) does not flicker. A new key always starts from loading. This is the one way the client
 * polls the server; usePages polls a paged list the same way.
 */
export function useResource<T>(
  key: string,
  load: () => Promise<T>,
  poller?: Poller,
): [Resource<T>, () => void] {
  const [held, setHeld] = useState<{ readonly key: string; readonly resource: Resource<T> }>({
    key,
    resource: { state: "loading" },
  });
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let isCurrent = true;
    let lastAnswer: string | undefined;
    // One read; answered, when polling, hears what it came to.
    const read = (answered?: (outcome: AskOutcome) => void): void => {
      load().then(
        (value) => {
          const answer = JSON.stringify(value);
          const isChanged = answer !== lastAnswer;
          lastAnswer = answer;
          if (isCurrent && isChanged) {
            setHeld({ key, resource: { state: "loaded", value } });
          }
          answered?.(isChanged ? "changed" : "unchanged");
        },
        (cause: unknown) => {
          reportFailure(cause);
          lastAnswer = undefined;
          if (isCurrent) {
            setHeld({ key, resource: { state: "failed", cause } });
          }
          answered?.("failed");
        },
      );
    };
    if (poller === undefined) {
      setHeld({ key, resource: { state: "loading" } });
      read();
      return () => {
        isCurrent = false;
      };
    }
    read();
    const stopPolling = poller.start(read);
    return () => {
      isCurrent = false;
      stopPolling();
    };
  }, [key, attempt, poller]);
  const reload = useCallback(() => {
    setAttempt((count) => count + 1);
  }, []);
  return [held.key === key ? held.resource : { state: "loading" }, reload];
}
