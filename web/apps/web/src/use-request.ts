// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { useState } from "react";

/** A request a screen runs, and how it is going: running, or failed with why. */
export interface RequestState {
  readonly isBusy: boolean;
  /** Why the last request failed, in the person's words; undefined while one runs or it worked. */
  readonly problem: string | undefined;
  readonly runRequest: (request: Promise<void>) => void;
}

/**
 * One request at a time: whether it is running, and why it failed when it did, in the words
 * describeFailure gives the cause. The cause itself goes to the console (reportFailure).
 */
export function useRequest(describeFailure: (cause: unknown) => string): RequestState {
  const [isBusy, setIsBusy] = useState(false);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const runRequest = (request: Promise<void>): void => {
    setIsBusy(true);
    setProblem(undefined);
    request.then(
      () => {
        setIsBusy(false);
      },
      (cause: unknown) => {
        reportFailure(cause);
        setIsBusy(false);
        setProblem(describeFailure(cause));
      },
    );
  };
  return { isBusy, problem, runRequest };
}
