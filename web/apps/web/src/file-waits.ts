// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useContext, useEffect, useState } from "react";

import { InterestSaidEveryContext } from "./page-interest.ts";
import { useServices } from "./services.tsx";

/** The work a file's viewer waits on, as the server says it (D324 §6). */
export type FileWaits = components["schemas"]["FileWaitsOutputBody"];

/** One piece of that work. */
export type FileWait = components["schemas"]["WaitBody"];

/**
 * The work a file's viewer waits on and about how long each will take, asked for while isWanted, at
 * once and again every interestSaidEverySeconds; undefined before the server answers, and when it
 * does not.
 */
export function useFileWaits(fileId: number | undefined, isWanted: boolean): FileWaits | undefined {
  const { api } = useServices();
  const everySeconds = useContext(InterestSaidEveryContext);
  const [waits, setWaits] = useState<FileWaits | undefined>(undefined);
  useEffect(() => {
    if (fileId === undefined || !isWanted) {
      return undefined;
    }
    let isCurrent = true;
    const ask = (): void => {
      api.GET("/files/{id}/waits", { params: { path: { id: fileId } } }).then(
        ({ data }) => {
          if (isCurrent && data !== undefined) {
            setWaits(data);
          }
        },
        () => undefined, // the words without an estimate stand
      );
    };
    ask();
    const timer = setInterval(ask, everySeconds * 1000);
    return () => {
      isCurrent = false;
      clearInterval(timer);
    };
  }, [api, fileId, isWanted, everySeconds]);
  return waits;
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
