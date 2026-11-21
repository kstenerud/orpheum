// SPDX-License-Identifier: MPL-2.0

import { reportFailure, type components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useContext, useMemo, useRef } from "react";

import { InterestSaidEveryContext } from "./page-interest.ts";
import { shownAndEveryPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** The work a file's viewer waits on, as the server says it (D324 §6). */
export type FileWaits = components["schemas"]["FileWaitsOutputBody"];

/** One piece of that work. */
export type FileWait = components["schemas"]["WaitBody"];

/**
 * The work a file's viewer waits on and about how long each will take, asked for while isWanted, at
 * once and again every interestSaidEverySeconds while the page shows, through shownAndEveryPoller:
 * one ask at a time, an ask with no answer given up, none while the page is hidden. Undefined before
 * the server answers, and when it does not; an answer stands through a read that fails after it,
 * the words without an estimate standing until one comes.
 */
export function useFileWaits(fileId: number | undefined, isWanted: boolean): FileWaits | undefined {
  const { api } = useServices();
  const everySeconds = useContext(InterestSaidEveryContext);
  const poller = useMemo(() => shownAndEveryPoller(everySeconds * 1000), [everySeconds]);
  const lastReadRef = useRef<{ readonly fileId: number; readonly waits: FileWaits }>(undefined);
  const isAsked = fileId !== undefined && isWanted;
  const [waits] = useResource(
    isAsked ? `file-waits-${String(fileId)}` : "file-waits-not-asked",
    async (signal) => {
      if (!isAsked) {
        return undefined;
      }
      try {
        const read = dataOrThrow(
          await api.GET("/files/{id}/waits", {
            params: { path: { id: fileId } },
            signal: signal ?? null,
          }),
        );
        lastReadRef.current = { fileId, waits: read };
        return read;
      } catch (cause) {
        const last = lastReadRef.current;
        if (signal?.aborted === true || last?.fileId !== fileId) {
          throw cause;
        }
        reportFailure(cause);
        return last.waits;
      }
    },
    isAsked ? { poller } : {},
  );
  return isAsked && waits.state === "loaded" ? waits.value : undefined;
}

/**
 * About how long seconds is, in words: under a minute, a minute, minutes, or hours from an hour and
 * a half, when two is the fewest it rounds to.
 */
export function waitText(t: TFunction, seconds: number): string {
  const minutes = Math.round(seconds / 60);
  if (minutes < 1) {
    return t("waits.underAMinute");
  }
  if (minutes === 1) {
    return t("waits.aMinute");
  }
  if (minutes < 90) {
    return t("waits.minutes", { count: minutes });
  }
  return t("waits.hours", { count: Math.round(minutes / 60) });
}
