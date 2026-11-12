// SPDX-License-Identifier: MPL-2.0

import { useCallback, useState } from "react";

/** How many lines of a probe's log its page keeps and sends. */
const logKept = 200;

/** A probe's log of what happened, each line by its seconds since the page opened, newest last, and its adding. */
export function useLog(): readonly [readonly string[], (line: string) => void] {
  const [lines, setLines] = useState<readonly string[]>([]);
  const [startedAt] = useState(() => performance.now());
  const add = useCallback(
    (line: string): void => {
      const at = ((performance.now() - startedAt) / 1000).toFixed(1);
      setLines((before) => [...before, `${at} s ${line}`].slice(-logKept));
    },
    [startedAt],
  );
  return [lines, add];
}
