// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import {
  pageActivation,
  playedMutedEvent,
  soundOnAtTap,
  type Api,
  type CannotKeepUp,
  type CastToReceive,
  type Press,
  type TogetherConnection,
  type TogetherEndReason,
} from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useCallback, useEffect, useMemo, useRef, useState, type JSX, type RefObject } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";

import { pressOf, type CastAnswer } from "./cast-asked.tsx";
import { endCast, moveCastOn } from "./cast.ts";
import { fillsWindowClass, isFullScreenLeaveAsked } from "./film-fullscreen.ts";
import { commandOf, keyCodeOf } from "./input-buttons.ts";
import { onCommand } from "./input-commands.ts";
import { useServices, useToken } from "./services.tsx";
import { useLive } from "./live.tsx";
import type { Navigation } from "./navigation.ts";
import { HostPlaysTogether, useLeadingPlayer } from "./together-host.tsx";
import { isTelevision } from "./tv-device.ts";
import { useTogether } from "./use-together.ts";

/**
 * How long the "Controlled from" line stays before it fades, in milliseconds: long enough to read a
 * short line across a room. A justified constant.
 */
export const castNoteShownMs = 5000;

/** The cast this device receives of fileId, while it does, and how it ends here (D337). */
export function useReceiving(fileId: number): Receiving | undefined {
  const { receiving, answer, endReceiving, leaveReceiving } = useLive();
  return useMemo(() => {
    if (receiving?.fileId !== fileId) {
      return undefined;
    }
    const its = answer?.castId === receiving.castId ? answer : undefined;
    return {
      cast: receiving,
      onOver: endReceiving,
      onLeave: leaveReceiving,
      ...(its === undefined ? {} : { answer: its }),
    };
  }, [receiving, answer, fileId, endReceiving, leaveReceiving]);
}

/**
 * What plays along with the Play page's video: the host's Watch Together session while this tab
 * hosts one, or the cast this device receives (D88: either is a session the video leads).
 */
export function PlaysAlong(props: {
  readonly receiving: Receiving | undefined;
  readonly fileId: number;
  readonly video: HTMLVideoElement | null;
  readonly startPositionMs: number;
  readonly cannotKeepUp: CannotKeepUp | undefined;
  /** Whether an episode plays after the film, which its Up next plays (plans/up-next.md). */
  readonly hasNext: boolean;
}): JSX.Element {
  const { receiving, hasNext, ...player } = props;
  return receiving === undefined ? (
    <HostPlaysTogether {...player} />
  ) : (
    <CastReceiver key={receiving.cast.castId} receiving={receiving} hasNext={hasNext} {...player} />
  );
}

/**
 * Moves the cast this device receives on to the next episode, which the server hands it again
 * (live.tsx plays it), its remote following; where it cannot, the cast ends at its film's end and
 * the receiver goes back to the page it was on before the cast, the remote told (D337's §9 answers,
 * Q4; D338 §1; plans/up-next.md).
 */
export function moveOnOrLeave(api: Api, receiving: Receiving): void {
  moveCastOn(api, receiving.cast.castId).catch(() => {
    endCast(api, receiving.cast.castId, "filmEnded").catch(() => undefined);
    receiving.onLeave();
  });
}

/** What the Play page is given while this device receives a cast. */
export interface Receiving {
  readonly cast: CastToReceive;
  /** Told that receiving is over here, the film playing on as the device's own: control taken back. */
  readonly onOver: () => void;
  /**
   * Told that receiving is over and the device goes back to the page it was on before the cast:
   * casting stopped, or the film over with nothing after it (D338 §1).
   */
  readonly onLeave: () => void;
  /**
   * The press that answered Play to the cast, where this tab was asked (D358 §2); absent for a cast
   * received at once, as one replacing another this tab received.
   */
  readonly answer?: CastAnswer;
}

/**
 * The receiver of a cast (D337 §3–§7, D338; plans/cast-to-device.md §4, §6): its video leads the
 * cast's session of one, from where the sender stood, filling the window, with a television's player
 * controls over it (the Play page's composition) and a "Controlled from" line that fades; its own
 * controls and media keys go on working, and every change of theirs reaches the remote as the
 * session's. Back, or OK twice, leaves controlled mode, the film playing on here (D338 §6). The cast
 * stopped by the remote sends it back to the page it was on before (D338 §1); at the film's end it
 * plays the next episode as the sender would (Up next, plans/up-next.md), and where none plays next
 * it goes back to that page too.
 */
export function CastReceiver(props: {
  readonly receiving: Receiving;
  readonly fileId: number;
  readonly video: HTMLVideoElement | null;
  readonly startPositionMs: number;
  /** Whether an episode plays after the film, whose Up next moves the cast on to it at its end. */
  readonly hasNext?: boolean;
}): JSX.Element {
  const { api } = useServices();
  const token = useToken();
  const { cast, onOver, onLeave } = props.receiving;
  const { video, fileId } = props;
  const { connection, snapshot } = useTogether({
    credential: cast.membershipCredential,
    sessionToken: token,
  });
  useLeadingPlayer({
    connection,
    isWelcomed: snapshot?.status === "connected",
    fileId,
    video,
    startPositionMs: props.startPositionMs,
    putOnAtMs: cast.positionMs,
  });
  useFillsTheWindow(video);
  const isWelcomed = snapshot?.status === "connected";
  useToldHowItStarted({ connection, isWelcomed, video, answer: props.receiving.answer });
  useEndsAsTheCastDoes(snapshot?.status === "ended" ? snapshot.endReason : undefined, {
    onOver,
    onLeave,
  });
  const hasNext = props.hasNext === true;
  useEffect(() => {
    if (hasNext) {
      return undefined; // its Up next moves the cast on, or ends it, as the person's autoplay says
    }
    const atTheEnd = (): void => {
      endCast(api, cast.castId, "filmEnded").catch(() => undefined);
      onLeave();
    };
    video?.addEventListener("ended", atTheEnd);
    return () => {
      video?.removeEventListener("ended", atTheEnd);
    };
  }, [api, cast.castId, onLeave, video, hasNext]);
  const takeBack = useCallback(() => {
    endCast(api, cast.castId, "takenBack").catch(() => undefined);
    onOver();
  }, [api, cast.castId, onOver]);
  return (
    <>
      <SoundAtAnyPress video={video} connection={connection} />
      <ControlledMode
        fromName={cast.fromDeviceName}
        onTakeBack={takeBack}
        connection={connection}
      />
    </>
  );
}

/**
 * Ends receiving as the cast ends: stopped by the remote, or its film over, the receiver goes back to
 * the page it was on (D338 §1); a cast replacing it is received already; any other end leaves an
 * ordinary player.
 */
function useEndsAsTheCastDoes(
  endReason: TogetherEndReason | undefined,
  ways: Pick<Receiving, "onOver" | "onLeave">,
): void {
  const { onOver, onLeave } = ways;
  useEffect(() => {
    if (endReason === undefined || endReason === "replaced") {
      return;
    }
    if (endReason === "stopped" || endReason === "filmEnded") {
      onLeave();
    } else {
      onOver();
    }
  }, [endReason, onOver, onLeave]);
}

/** Has the video's stage fill the window while it receives, by layout, as no press asked (§4). */
function useFillsTheWindow(video: HTMLVideoElement | null): void {
  useEffect(() => {
    const stage = video?.closest(".film-player__stage");
    if (!(stage instanceof HTMLElement) || stage.classList.contains(fillsWindowClass)) {
      return undefined;
    }
    stage.classList.add(fillsWindowClass);
    return () => {
      stage.classList.remove(fillsWindowClass);
    };
  }, [video]);
}

/**
 * Tells the server's log how the cast started here (D358 §2): what the browser answered the full
 * screen the Play press asked for, and by what press, once the session is connected; and whether the
 * film started with its sound on, as it first plays (a refusal is told by playOrPlayMuted).
 */
function useToldHowItStarted(started: {
  readonly connection: Pick<TogetherConnection, "note"> | undefined;
  readonly isWelcomed: boolean;
  readonly video: HTMLVideoElement | null;
  readonly answer: CastAnswer | undefined;
}): void {
  const { connection, isWelcomed, video, answer } = started;
  const hasToldFullScreenRef = useRef(false);
  useEffect(() => {
    if (
      connection === undefined ||
      !isWelcomed ||
      answer?.fullScreen === undefined ||
      hasToldFullScreenRef.current
    ) {
      return;
    }
    hasToldFullScreenRef.current = true;
    const { by } = answer;
    answer.fullScreen
      .then((why) => {
        connection.note({ kind: "fullScreen", why, by });
      })
      .catch(() => undefined);
  }, [connection, isWelcomed, answer]);
  useEffect(() => {
    if (video === null || connection === undefined) {
      return undefined;
    }
    const playing = (): void => {
      video.removeEventListener("playing", playing);
      if (!video.muted) {
        const page = pageActivation();
        connection.note({
          kind: "playedWithSound",
          positionMs: video.currentTime * 1000,
          ...(page === undefined ? {} : { page }),
        });
      }
    };
    video.addEventListener("playing", playing);
    return () => {
      video.removeEventListener("playing", playing);
    };
  }, [video, connection]);
}

/**
 * The sound turned on at the receiver's first press while the browser plays its film muted (D358
 * §5): it refused the film's sound, though Play was pressed (a policy, or a browser that wants its
 * press on the video itself). A line over the film says so ("Press OK to turn the sound on" on a
 * television), and the first press answers it: any key but those that are no press to a browser
 * (Escape, a modifier, a shortcut) or that leave (Back), and a click or a tap anywhere. That press
 * does nothing else, so OK neither pauses the film nor counts towards the double OK that leaves
 * controlled mode, nor does a click OK makes after it (samePressWithinMs).
 */
export function SoundAtAnyPress(props: {
  readonly video: HTMLVideoElement | null;
  readonly connection: Pick<TogetherConnection, "note"> | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { video } = props;
  const muted = useMutedByTheBrowser(video);
  const latestRef = useRef(props);
  useEffect(() => {
    latestRef.current = props;
  });
  const { setIsMuted } = muted;
  const turnOn = useCallback(
    (by: Press): void => {
      const { video: shown, connection } = latestRef.current;
      if (shown === null || connection === undefined) {
        return;
      }
      setIsMuted(false);
      soundOnAtTap(shown, connection, by);
    },
    [setIsMuted],
  );
  useFirstPressTurnsSoundOn(muted.isMutedRef, turnOn);
  if (!muted.isMuted || video === null) {
    return null;
  }
  const line = <output className="cast-receiver__sound-on">{soundOnText(t)}</output>;
  // Over the film, inside its stage, so it shows in full screen too.
  const stage = video.closest(".film-player__stage");
  return stage instanceof HTMLElement ? createPortal(line, stage) : line;
}

/** Whether the browser plays video muted, having refused its sound (playedMutedEvent). */
function useMutedByTheBrowser(video: HTMLVideoElement | null): {
  readonly isMuted: boolean;
  /** isMuted as a press's listener reads it, between renders. */
  readonly isMutedRef: RefObject<boolean>;
  readonly setIsMuted: (isMuted: boolean) => void;
} {
  const [isShownMuted, setIsShownMuted] = useState(false);
  const isMutedRef = useRef(false);
  const setIsMuted = useCallback((isNow: boolean) => {
    isMutedRef.current = isNow;
    setIsShownMuted(isNow);
  }, []);
  useEffect(() => {
    if (video === null) {
      return undefined;
    }
    const onPlayedMuted = (): void => {
      setIsMuted(true);
    };
    video.addEventListener(playedMutedEvent, onPlayedMuted);
    return () => {
      video.removeEventListener(playedMutedEvent, onPlayedMuted);
    };
  }, [video, setIsMuted]);
  return { isMuted: isShownMuted, isMutedRef, setIsMuted };
}

/**
 * Hears, before anything else on the page, the first press while isMutedRef says the film plays
 * muted, and takes it for turnOn: a key that is a press (isNoSoundPress), or a click or a tap, with
 * the click a TV browser makes of the same OK.
 */
function useFirstPressTurnsSoundOn(
  isMutedRef: RefObject<boolean>,
  turnOn: (by: Press) => void,
): void {
  useEffect(() => {
    let keyTurnedItOnAt = Number.NEGATIVE_INFINITY;
    const take = (event: Event): void => {
      event.preventDefault();
      event.stopImmediatePropagation();
    };
    const onKey = (event: KeyboardEvent): void => {
      if (!isMutedRef.current || isNoSoundPress(event)) {
        return;
      }
      take(event);
      keyTurnedItOnAt = performance.now();
      turnOn("key");
    };
    const onClick = (event: MouseEvent): void => {
      if (performance.now() - keyTurnedItOnAt <= samePressWithinMs) {
        take(event); // the click a TV browser makes of the OK that turned the sound on
        return;
      }
      if (isMutedRef.current) {
        take(event);
        turnOn(pressOf(event));
      }
    };
    window.addEventListener("keydown", onKey, true);
    window.addEventListener("click", onClick, true);
    return () => {
      window.removeEventListener("keydown", onKey, true);
      window.removeEventListener("click", onClick, true);
    };
  }, [isMutedRef, turnOn]);
}

/**
 * Whether a key press is one that turns no sound on: Escape and a modifier, which no browser counts
 * as a press that lets a page play sound (the HTML standard's activation-triggering input events), a
 * shortcut, and Back, which leaves controlled mode as it always does.
 */
function isNoSoundPress(event: KeyboardEvent): boolean {
  if (event.ctrlKey || event.altKey || event.metaKey || modifierKeys.has(event.key)) {
    return true;
  }
  // Escape is Back too.
  return commandOf({ kind: "key", key: event.key, keyCode: keyCodeOf(event) }) === "back";
}

/** The keys that only modify another. */
const modifierKeys = new Set(["Shift", "Control", "Alt", "Meta", "AltGraph", "CapsLock", "Fn"]);

/** The line that asks for a press to turn the sound on, in this device's words for one. */
function soundOnText(t: TFunction): string {
  if (isTelevision(navigator.userAgent)) {
    return t("cast.soundOn.television");
  }
  const isTouch = typeof matchMedia === "function" && matchMedia("(pointer: coarse)").matches;
  return isTouch ? t("cast.soundOn.touch") : t("cast.soundOn.pointer");
}

/**
 * How close together two presses of OK, or two taps, must come to leave controlled mode, in
 * milliseconds: the double-click time desktop systems default to (Windows' 500 ms), which a person
 * pressing twice on purpose stays well within. A justified constant.
 */
export const doublePressWithinMs = 500;

/**
 * How soon after an OK a click is the same press, in milliseconds: a TV browser may follow OK's key
 * with the click it makes on what has focus, which is not a second press. A justified constant, well
 * under any two presses a hand makes.
 */
const samePressWithinMs = 100;

/**
 * The receiver's way out of controlled mode (D338 §6): Back, a remote's (a Fire TV's is a history
 * step, never a key: the page keeps an entry of its own to step back to) or a key's, and OK pressed
 * twice, or a tap twice, within doublePressWithinMs, leave it; one press of OK, a tap and every other
 * key are the receiver's own controls'. Full screen left by the browser, not by the page, leaves it
 * too, told to the log: in full screen a Fire TV's Back only leaves full screen, never reaching the
 * page, and a desktop's Escape does the same, each meant as leaving (the owner, 2026-10-27: Back took
 * two presses). A "Controlled from" line, which fades after castNoteShownMs, says who controls it and
 * how to leave.
 */
export function ControlledMode(props: {
  readonly fromName: string;
  readonly onTakeBack: () => void;
  /** The cast's session, told that the browser left full screen unasked. */
  readonly connection?: Pick<TogetherConnection, "note"> | undefined;
}): JSX.Element {
  const { navigation } = useServices();
  useLeavingPresses(navigation, props.onTakeBack, props.connection);
  return (
    <div className="cast-receiver">
      <ControlledFrom name={props.fromName} />
    </div>
  );
}

/** "Controlled from" the sender, and how to leave, which fades and goes after castNoteShownMs. */
function ControlledFrom(props: { readonly name: string }): JSX.Element | null {
  const { t } = useTranslation();
  const [isShown, setIsShown] = useState(true);
  useEffect(() => {
    const timer = setTimeout(() => {
      setIsShown(false);
    }, castNoteShownMs);
    return () => {
      clearTimeout(timer);
    };
  }, []);
  return isShown ? (
    <output className="cast-receiver__note">
      {t("cast.controlledFrom", { name: props.name })}
    </output>
  ) : null;
}

/**
 * Hears the presses that leave controlled mode: Back as a command, before anything else on the page
 * hears it, or as a step back from the receiver's own history entry, kept as it starts; OK or a tap
 * twice within doublePressWithinMs, the second press taken from the page; and full screen left
 * unasked (useFullScreenLeftLeaves). Control is taken back once, however many of them come.
 */
function useLeavingPresses(
  navigation: Navigation,
  onTakeBack: () => void,
  connection: Pick<TogetherConnection, "note"> | undefined,
): void {
  const takeBackRef = useOnce(onTakeBack);
  useFullScreenLeftLeaves(takeBackRef, connection);
  useEffect(() => {
    let lastPressAt = Number.NEGATIVE_INFINITY;
    let lastSelectAt = Number.NEGATIVE_INFINITY;
    const isSecondPress = (now: number): boolean => {
      const isSecond = now - lastPressAt <= doublePressWithinMs;
      lastPressAt = isSecond ? Number.NEGATIVE_INFINITY : now;
      return isSecond;
    };
    const stopCommands = onCommand(
      window,
      (detail, event) => {
        const now = performance.now();
        if (detail.command === "select") {
          lastSelectAt = now;
        }
        const isLeaving =
          detail.command === "back" || (detail.command === "select" && isSecondPress(now));
        if (isLeaving) {
          event.preventDefault();
          takeBackRef.current();
        }
      },
      { isCapture: true },
    );
    const onClick = (event: MouseEvent): void => {
      const now = performance.now();
      // A click a key made (detail 0), or one following OK as the same press, is not a tap.
      if (event.detail === 0 || now - lastSelectAt <= samePressWithinMs || !isSecondPress(now)) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      takeBackRef.current();
    };
    window.addEventListener("click", onClick, true);
    navigation.go(navigation.route());
    const onBack = (): void => {
      takeBackRef.current();
    };
    window.addEventListener("popstate", onBack);
    return () => {
      stopCommands();
      window.removeEventListener("click", onClick, true);
      window.removeEventListener("popstate", onBack);
    };
  }, [navigation, takeBackRef]);
}

/** What calls once the latest given call, however often it is called itself. */
function useOnce(call: () => void): RefObject<() => void> {
  const hasCalledRef = useRef(false);
  const onceRef = useRef(call);
  useEffect(() => {
    onceRef.current = () => {
      if (!hasCalledRef.current) {
        hasCalledRef.current = true;
        call();
      }
    };
  });
  return onceRef;
}

/**
 * Takes control back where the browser leaves full screen and the page did not ask it to
 * (isFullScreenLeaveAsked): a Fire TV's Back in full screen, which Silk keeps from the page, and a desktop's
 * Escape. The log is told first, as the press that asked for full screen was (D358 §2).
 */
function useFullScreenLeftLeaves(
  takeBackRef: RefObject<() => void>,
  connection: Pick<TogetherConnection, "note"> | undefined,
): void {
  const connectionRef = useRef(connection);
  useEffect(() => {
    connectionRef.current = connection;
  });
  useEffect(() => {
    const changed = (): void => {
      if (document.fullscreenElement instanceof Element || isFullScreenLeaveAsked(document)) {
        return;
      }
      connectionRef.current?.note({ kind: "fullScreen", why: "left" });
      takeBackRef.current();
    };
    document.addEventListener("fullscreenchange", changed);
    return () => {
      document.removeEventListener("fullscreenchange", changed);
    };
  }, [takeBackRef]);
}
