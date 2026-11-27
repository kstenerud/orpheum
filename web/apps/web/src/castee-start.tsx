// SPDX-License-Identifier: MPL-2.0

// A cast's start on its Play page, from the choice of a TV until its first picture (D569): what the
// page says of it in its centre, the Cancel beside it, and how its controls wait.

import "./cast-words.ts";

import type { TogetherSnapshot } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { casteeStandingIn, isCasteeWaitingForOKIn } from "./castee-target.ts";
import { ActionPill, Pill } from "./pill.tsx";
import type { PlaybackTarget } from "./playback-target.ts";
import { StatusNote } from "./status-note.tsx";
import { useCastEnding } from "./stop-casting-here.tsx";

/**
 * What the castee keeps the remote waiting on (D569 §1, §6), read from the cast's session: "forOK"
 * while it is heard, over a connected session, holding the cast for a press on its own remote (D564
 * §2), first, as a castee waiting for its press plays nothing and so says no position; "connecting"
 * while it is not reached (useReceiverHeard, D550: this amends D548 §9's "Reaching ‹TV›…");
 * "loading" while, reached, it is yet to show its first picture (casteeStandingIn) or its session
 * holds no place for it; undefined once it plays.
 */
export type CasteeWait = "connecting" | "loading" | "forOK" | undefined;

function casteeWaitOf(
  target: Pick<PlaybackTarget, "isReady">,
  isHeard: boolean,
  snapshot: TogetherSnapshot | undefined,
): CasteeWait {
  if (isHeard && snapshot?.status === "connected" && isCasteeWaitingForOKIn(snapshot)) {
    return "forOK";
  }
  if (!isHeard) {
    return "connecting";
  }
  return casteeStandingIn(snapshot) === "loading" || !target.isReady ? "loading" : undefined;
}

/** How long a start may go without a picture before the page says the TV is slow to start (D569 §4). */
export const slowStartMs = 20_000;

/**
 * Where the cast's start stands on this page (D569): "unsaid" until the session shows how the castee
 * stands; "starting" where it shows the castee not reached, reached and yet to show its first picture
 * (casteeStandingIn), or waiting for its press (CasteeWait), until it is first ready; then
 * "pausedOn" while the TV, ready, stands paused, until it plays (§3), else "on". A castee the roster
 * first shows ready starts nothing, and one lost after its start and reached again (D550) is no start: no Cancel, no
 * slow notice. Read from the session (§6), a page opened over a start says the same.
 */
export type CastStart = "unsaid" | "starting" | "pausedOn" | "on";

export interface CastStartShown {
  /**
   * What the castee keeps the remote waiting on (CasteeWait): casteeWaitOf's, or, through the start,
   * "loading" while the roster is yet to show the TV reached, though the page hears it.
   */
  readonly wait: CasteeWait;
  readonly start: CastStart;
  /** Whether the start has gone slowStartMs without a picture, outside a wait for OK (D569 §4). */
  readonly isSlow: boolean;
}

/** Whether the session shows the castee starting: not reached, loading, or waiting for OK. */
function isStartingIn(castee: {
  readonly wait: CasteeWait;
  readonly standing: ReturnType<typeof casteeStandingIn>;
}): boolean {
  return castee.standing === "loading" || castee.wait === "connecting" || castee.wait === "forOK";
}

/** The start CastStart moves on to from start, as the castee waits, stands (casteeStandingIn) and is paused. */
function startAfter(
  start: CastStart,
  castee: {
    readonly wait: CasteeWait;
    readonly standing: ReturnType<typeof casteeStandingIn>;
    readonly isPaused: boolean;
  },
): CastStart {
  const { wait, standing } = castee;
  const isPlaying = wait === undefined && standing === "present";
  if (start === "unsaid") {
    if (isStartingIn(castee)) {
      return "starting";
    }
    return isPlaying ? "on" : "unsaid";
  }
  if (start === "starting" && isPlaying) {
    return castee.isPaused ? "pausedOn" : "on";
  }
  return start === "pausedOn" && !castee.isPaused ? "on" : start;
}

/** The cast's start as the castee moves it on (CastStart), Play focused as it ends paused. */
function useCastStart(
  wait: CasteeWait,
  castee: { readonly snapshot: TogetherSnapshot | undefined; readonly isPaused: boolean },
  playRef: RefObject<HTMLButtonElement | null>,
): CastStartShown {
  const [was, setWas] = useState<CastStart>("unsaid");
  const [isSlow, setIsSlow] = useState(false);
  const standing = casteeStandingIn(castee.snapshot);
  const start = startAfter(was, { wait, standing, isPaused: castee.isPaused });
  if (start !== was) {
    setWas(start); // kept as it renders, so this render already shows where the start moved to
  }
  useEffect(() => {
    if (start === "pausedOn") {
      playRef.current?.focus(); // a TV ready paused waits for Play (D569 §3)
    }
  }, [start, playRef]);
  const shownWait = start === "starting" && wait === undefined ? "loading" : wait;
  const isTimed = start === "starting" && wait !== "forOK";
  useEffect(() => {
    if (!isTimed) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setIsSlow(true);
    }, slowStartMs);
    return () => {
      clearTimeout(timer);
      setIsSlow(false);
    };
  }, [isTimed]);
  return { wait: shownWait, start, isSlow: isSlow && isTimed };
}

/** The cast's start on the castee's page (D569), and how its controls wait through it. */
export interface CasteeStart extends CastStartShown {
  /** Whether play/pause, seeking, Next and the menus wait: the castee not ready, or its start (§2). */
  readonly isDisabled: boolean;
  /** The remote pad's part in it (RemotePad's props). */
  readonly pad: {
    /** The pad's Play, focused as a TV first ready stands paused (§3). */
    readonly playRef: RefObject<HTMLButtonElement | null>;
    /** Whether the volume waits too: as isDisabled, but through the start it does not (§2). */
    readonly isSoundDisabled: boolean;
  };
}

export function useCasteeStart(
  target: Pick<PlaybackTarget, "isReady" | "isPaused">,
  isHeard: boolean,
  snapshot: TogetherSnapshot | undefined,
): CasteeStart {
  const playRef = useRef<HTMLButtonElement>(null);
  const shown = useCastStart(
    casteeWaitOf(target, isHeard, snapshot),
    { snapshot, isPaused: target.isPaused },
    playRef,
  );
  const isDisabled = !target.isReady || shown.wait !== undefined;
  const isSoundDisabled = isDisabled && shown.start !== "starting";
  return { ...shown, isDisabled, pad: { playRef, isSoundDisabled } };
}

/** The centre's words while the castee keeps the remote waiting (CasteeCentre). */
function waitWords(
  t: ReturnType<typeof useTranslation>["t"],
  wait: Exclude<CasteeWait, undefined>,
  told: { readonly name: string; readonly isServerLost: boolean; readonly isSlow: boolean },
): string {
  const { name } = told;
  if (wait === "forOK") {
    return t("cast.waitingForOK", { name });
  }
  if (told.isServerLost) {
    return t("cast.serverLost");
  }
  if (told.isSlow) {
    return t("cast.slowToStart", { name });
  }
  return wait === "connecting" ? t("cast.connecting", { name }) : t("cast.loading", { name });
}

/**
 * The offer area's centre (D569 §1, §5): the cast's own notices, where its other notices show. While
 * the castee is not reached, "Connecting to ‹TV›…", or the server lost; reached, "Loading on ‹TV›…";
 * after slowStartMs of either, that the TV is slow to start; while it waits for a press on its own
 * remote (D564 §2), "Waiting for someone to press OK on ‹TV›", with no spinner, as nothing here moves
 * it on. Through the start, Cancel under it ends the cast as Stop casting does (useCastEnding), and
 * the film plays here again where it stood. Ready and paused, "Paused on ‹TV›" (§3). Else children,
 * the volume a moment after it was set.
 */
export function CasteeCentre(props: {
  readonly sent: { readonly castId: number; readonly receiverName: string };
  readonly shown: CastStartShown;
  readonly isServerLost: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const { shown } = props;
  const { wait } = shown;
  const name = props.sent.receiverName;
  if (wait === undefined) {
    return shown.start === "pausedOn" ? (
      <span className="castee-play__centre">
        <Pill>{t("cast.pausedOn", { name })}</Pill>
      </span>
    ) : (
      <>{props.children}</>
    );
  }
  const told = { name, isServerLost: props.isServerLost, isSlow: shown.isSlow };
  return (
    <span className="castee-play__centre">
      <Pill {...(wait === "forOK" ? {} : { icon: <span className="cast-remote__spinner" /> })}>
        {waitWords(t, wait, told)}
      </Pill>
      {shown.start === "starting" ? <CancelTheCast castId={props.sent.castId} /> : null}
    </span>
  );
}

/** The start's Cancel (D569 §5): the cast's end as Stop casting asks it, why it failed under it. */
function CancelTheCast(props: { readonly castId: number }): JSX.Element {
  const { t } = useTranslation();
  const ending = useCastEnding(props.castId, "stopped");
  return (
    <>
      <ActionPill isDangerous isDisabled={ending.isBusy} onPress={ending.end}>
        {t("cast.cancel")}
      </ActionPill>
      {ending.problem === undefined ? null : (
        <StatusNote told={{ kind: "problem", text: ending.problem }} />
      )}
    </>
  );
}
