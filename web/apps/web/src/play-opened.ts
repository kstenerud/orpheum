// SPDX-License-Identifier: MPL-2.0

/**
 * How the Play page opens its playback: the decision asked for, asked for again when the page says
 * why, and opened again where a playback the server lost, or whose player gave the film's own
 * picture or sound up, stood (play.tsx's PlayHere).
 */

import { use, useCallback, useRef, useState, type RefObject } from "react";

import { OpenedAsContext } from "./around-file.ts";
import {
  openPlayback,
  type CopyGivenUpCause,
  type Opened,
  type Reopening,
} from "./play-start-timing.ts";
import { reopenAtMostEveryMs, type ReopenAt } from "./reopen.ts";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";
import { watchingOf, type StartCause } from "./watching.ts";

/** Why a playing page asks how the file plays again. */
export type DecideAgainCause = Extract<
  StartCause,
  | "subtitleFetched"
  | "audioChosen"
  | "originalChosen"
  | "linkCarriesOriginal"
  | "boostChosen"
  | "raiseWanted"
  | "dataCapStarted"
>;

/**
 * Asks the server again how the file plays, saying why, and, given at, from where the video stood
 * then rather than where the server last heard it was.
 */
export type DecideAgain = (cause: DecideAgainCause, at?: ReopenAt) => void;

/** Where the Play page's first playback starts, and why it is asked for. */
export interface FirstAsk {
  readonly cause: StartCause;
  readonly atMs: number;
  /** Whether it waits paused there (Opened's isStartPaused). */
  readonly isPaused?: boolean;
}

/** What the Play page shows: its decision, or, while the next is asked for, the one held. */
export function shownOf(opened: Resource<Opened>, held: Opened | undefined): Opened | undefined {
  if (opened.state === "loaded") {
    return opened.value;
  }
  return opened.state === "loading" ? held : undefined;
}

/**
 * How the Play page opens its playback again: a lost one at most every reopenAtMostEveryMs, saying
 * whether it does, and one whose player gave the film's own picture or sound up from where the video
 * stood, asked for again at what the server makes in its place (D323 §2, D533 §2). What played
 * before is held on screen meanwhile.
 */
function useReopening(parts: {
  readonly reopenRef: RefObject<Reopening | undefined>;
  readonly reload: () => void;
  readonly hold: () => void;
}): {
  readonly reopenPlayback: (whereNow: () => ReopenAt) => boolean;
  readonly startAtTheMade: (at: ReopenAt, cause: CopyGivenUpCause) => void;
  readonly reopenWhenBack: (at: ReopenAt) => void;
} {
  const { reopenRef, reload, hold } = parts;
  // When the last lost one was opened again, on the page's clock.
  const lastReopenMsRef = useRef<number | undefined>(undefined);
  const reopen = useCallback(
    (asked: Reopening): void => {
      reopenRef.current = asked;
      hold();
      reload();
    },
    [reopenRef, reload, hold],
  );
  const reopenPlayback = useCallback(
    (whereNow: () => ReopenAt): boolean => {
      const nowMs = performance.now();
      const lastMs = lastReopenMsRef.current;
      if (lastMs !== undefined && nowMs - lastMs < reopenAtMostEveryMs) {
        return false;
      }
      lastReopenMsRef.current = nowMs;
      reopen({ whereNow, cause: "playbackGone" });
      return true;
    },
    [reopen],
  );
  const startAtTheMade = useCallback(
    (at: ReopenAt, cause: CopyGivenUpCause): void => {
      reopen({ whereNow: () => at, cause });
    },
    [reopen],
  );
  const reopenWhenBack = useCallback(
    (at: ReopenAt): void => {
      reopen({ whereNow: () => at, cause: "mediaBack" });
    },
    [reopen],
  );
  return { reopenPlayback, startAtTheMade, reopenWhenBack };
}

/**
 * The Play page's decision for the file, asked for again when decideAgain says why, and opened again
 * where a playback the server lost is when its new one comes, once reopenPlayback is told so, which
 * says whether it is: not within reopenAtMostEveryMs of the last. held is what played before, shown
 * while the new one is asked for.
 */
export function useOpenedPlayback(
  fileId: number,
  first: FirstAsk | undefined,
): {
  readonly opened: Resource<Opened>;
  readonly held: Opened | undefined;
  readonly decideAgain: DecideAgain;
  readonly reopenPlayback: (whereNow: () => ReopenAt) => boolean;
  readonly startAtTheMade: (at: ReopenAt, cause: CopyGivenUpCause) => void;
  readonly reopenWhenBack: (at: ReopenAt) => void;
} {
  const { api, playableFormats } = useServices();
  const itemId = use(OpenedAsContext); // the title it plays as (B0 question 20)
  // Why the next decision is asked for: the page's opening, unless something asked it again; and
  // where the video stood when it did, for the next to start from, when it said.
  const nextCauseRef = useRef<StartCause>(first?.cause ?? "playPage");
  const nextAtRef = useRef<ReopenAt | undefined>(
    first === undefined ? undefined : { positionMs: first.atMs, isPaused: first.isPaused === true },
  );
  // Where a playback opened again is, read as its new one comes, for that to start from, and why it
  // is: the server lost it, or its player gave the film's own picture or sound up; and when the last
  // lost one
  // was opened again, on the page's clock.
  const reopenRef = useRef<Reopening | undefined>(undefined);
  const [held, setHeld] = useState<Opened | undefined>(undefined);
  const [opened, reload] = useResource(
    `play/${String(fileId)}/${String(itemId)}`,
    async (): Promise<Opened> => {
      const cause = nextCauseRef.current;
      const askedAt = nextAtRef.current;
      nextCauseRef.current = "playPage";
      nextAtRef.current = undefined;
      const reopen = reopenRef.current;
      reopenRef.current = undefined;
      if (reopen === undefined) {
        setHeld(undefined);
      }
      return await openPlayback({
        watching: watchingOf(api),
        fileId,
        itemId,
        playableFormats: playableFormats(),
        cause,
        askedAt,
        reopen,
      });
    },
    { isSingleUse: true },
  ); // a playback opened is this page's alone, never the tab's (D478)
  const decideAgain = useCallback(
    (cause: DecideAgainCause, at?: ReopenAt) => {
      nextCauseRef.current = cause;
      nextAtRef.current = at;
      reload();
    },
    [reload],
  );
  const { reopenPlayback, startAtTheMade, reopenWhenBack } = useReopening({
    reopenRef,
    reload,
    hold: () => {
      setHeld(opened.state === "loaded" ? opened.value : undefined);
    },
  });
  return { opened, held, decideAgain, reopenPlayback, startAtTheMade, reopenWhenBack };
}
