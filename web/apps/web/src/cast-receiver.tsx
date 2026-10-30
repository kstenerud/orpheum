// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import {
  pageActivation,
  playedMutedEvent,
  soundOnAtTap,
  type Api,
  type CannotKeepUp,
  type CarriesSoundChoices,
  type CastToReceive,
  type PlaybackReport,
  type Press,
  type TogetherConnection,
  type TogetherEndReason,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import type { TFunction } from "i18next";
import {
  use,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type JSX,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";

import { pressOf, type CastAnswer } from "./cast-asked.tsx";
import { endCast, moveCastOn, playInCast } from "./cast.ts";
import { leaveUncontrolledCast } from "./cast-uncontrolled.ts";
import { fillsWindowClass, isFullScreenLeaveAsked } from "./film-fullscreen.ts";
import { commandOf, keyCodeOf } from "./input-buttons.ts";
import { onCommand } from "./input-commands.ts";
import { useServices, useToken } from "./services.tsx";
import { useLive } from "./live.tsx";
import type { Navigation } from "./navigation.ts";
import { answerPlayerCommand } from "./player-commands.ts";
import { HostPlaysTogether, useLeadingPlayer } from "./together-host.tsx";
import { SoundPanelContext, type SoundPanel } from "./sound-panel.ts";
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
  /** How the film is given to the player, which its session is told (D371 §4). */
  readonly delivery?: PlaybackReport["delivery"] | undefined;
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
 * (live.tsx plays it), its remote following (plans/up-next.md); where it cannot, waits ready for the
 * next video (readyForTheNext).
 */
export function moveOnOrReady(api: Api, receiving: Receiving): void {
  moveCastOn(api, receiving.cast.castId).catch(() => {
    readyForTheNext(api, receiving);
  });
}

/**
 * Has the cast this device receives play nothing, at its film's end with nothing after it: the
 * server hands it again so, and the receiver waits full screen on its ready page for the next video
 * the person plays (D364 §5), the session going on. A cast that cannot is over, and the receiver goes
 * back to the page it was on before it (D338 §1).
 */
export function readyForTheNext(api: Api, receiving: Receiving): void {
  playInCast(api, receiving.cast.castId, undefined).catch(() => {
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
   * casting stopped (D364 §10, D338 §1).
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
 * session's. While its person hosts Watch Together, its video leads that session instead, as their
 * player, so what they play plays for the group, the cast's session told only how it ends (D364 §11).
 * Back, or OK twice, leaves controlled mode, the film playing on here (D364 §9, D338 §6). The cast
 * stopped by the remote sends it back to the page it was on before (D364 §10); at the film's end it
 * plays the next episode as the sender would (Up next, plans/up-next.md), and where none plays next
 * it waits ready for the next video (D364 §5).
 */
export function CastReceiver(props: {
  readonly receiving: Receiving;
  readonly fileId: number;
  readonly video: HTMLVideoElement | null;
  readonly startPositionMs: number;
  /** Whether an episode plays after the film, whose Up next moves the cast on to it at its end. */
  readonly hasNext?: boolean;
  /** How the film is given to the player, which the remote shows (D371 §4). */
  readonly delivery?: PlaybackReport["delivery"] | undefined;
}): JSX.Element {
  const { api } = useServices();
  const { cast, onOver, onLeave } = props.receiving;
  const { video, fileId } = props;
  // The page's sound controls, which the remote shows and works (sound-panel.ts).
  const soundPanel = use(SoundPanelContext);
  const { connection, isWelcomed, snapshot } = useReceiversSessions(cast);
  useLeadingPlayer({
    connection,
    isWelcomed,
    fileId,
    video,
    startPositionMs: props.startPositionMs,
    putOnAtMs: cast.positionMs,
    delivery: props.delivery,
    readSound: soundPanel?.read,
  });
  useSoundWorkedByTheRemote(connection, soundPanel);
  useFillsTheWindow(video);
  useToldHowItStarted({ connection, isWelcomed, video, answer: props.receiving.answer });
  useEndsAsTheCastDoes(snapshot?.status === "ended" ? snapshot.endReason : undefined, {
    onOver,
    onLeave,
    onNoController: useLeavesForTheFilmsPage(onOver),
  });
  const hasNext = props.hasNext === true;
  const { receiving } = props;
  useEffect(() => {
    if (hasNext) {
      return undefined; // its Up next moves the cast on, or waits ready, as the person's autoplay says
    }
    const atTheEnd = (): void => {
      readyForTheNext(api, receiving);
    };
    video?.addEventListener("ended", atTheEnd);
    return () => {
      video?.removeEventListener("ended", atTheEnd);
    };
  }, [api, receiving, video, hasNext]);
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
 * Has the remote work the page's sound controls: each choice it sends carried out by the control it
 * is of, as that control's own press, and each change of how the controls stand told the session at
 * once, for the remote to show (sound-panel.ts). A Fire TV's own controls are hidden while it is
 * controlled, so its sound could only be changed after taking control back (the owner, VQ116's run).
 */
function useSoundWorkedByTheRemote(
  connection: (TogetherConnection & CarriesSoundChoices) | undefined,
  panel: SoundPanel | undefined,
): void {
  useEffect(() => {
    if (connection === undefined || panel === undefined) {
      return undefined;
    }
    const unsubscribe = connection.subscribeToSoundChoices(panel.choose);
    const unwatch = panel.subscribe(connection.reportState);
    return () => {
      unsubscribe();
      unwatch();
    };
  }, [connection, panel]);
}

/**
 * The sessions a cast's receiver is in: the cast's own, which says how it ends, and the one its video
 * leads, the cast's or, while its person hosts Watch Together, that session, as their player (D364
 * §11), the cast's then joined as a panel, playing nothing. Gives the led session's connection and
 * whether it is welcomed, and the cast's own snapshot.
 */
function useReceiversSessions(cast: CastToReceive): {
  readonly connection: (TogetherConnection & CarriesSoundChoices) | undefined;
  readonly isWelcomed: boolean;
  readonly snapshot: TogetherSnapshot | undefined;
} {
  const token = useToken();
  // A guest's part is followed from the ready page (cast-ready.tsx); a film of the cast's own leads it.
  const isLeadingHosted = cast.togetherCredential !== undefined && cast.isTogetherFollower !== true;
  const castSession = useTogether({
    credential: cast.membershipCredential,
    sessionToken: token,
    role: isLeadingHosted ? "panel" : undefined,
  });
  const hosted = useTogether({ credential: cast.togetherCredential, sessionToken: token });
  const led = isLeadingHosted ? hosted : castSession;
  return {
    connection: led.connection,
    isWelcomed: led.snapshot?.status === "connected",
    snapshot: castSession.snapshot,
  };
}

/**
 * What a receiver's Play page does as its cast is gone with no one controlling it (D383 §2): it goes
 * to the film's page, or home (leaveUncontrolledCast), then ends receiving here (onOver), so the film
 * never plays on as the device's own with no one having asked it to.
 */
function useLeavesForTheFilmsPage(onOver: () => void): () => void {
  const { api, navigation } = useServices();
  return useCallback(() => {
    void leaveUncontrolledCast(api, navigation).finally(onOver);
  }, [api, navigation, onOver]);
}

/**
 * Ends receiving as the cast ends: stopped on the sender, the receiver goes back to the page it was on
 * before the session (D364 §10, D338 §1); a cast replacing it is received already; its control taken
 * back here leaves what shows as it is, an ordinary player (onOver, D364 §9); any other end, the cast
 * gone with no one controlling it (a session that ended unseen, as its receiver was gone too long or
 * the server restarted), goes by onNoController where given (D383 §2), else as taken back.
 */
export function useEndsAsTheCastDoes(
  endReason: TogetherEndReason | undefined,
  ways: Pick<Receiving, "onOver" | "onLeave"> & { readonly onNoController?: () => void },
): void {
  const { onOver, onLeave, onNoController } = ways;
  useEffect(() => {
    if (endReason === undefined || endReason === "replaced") {
      return;
    }
    if (endReason === "stopped") {
      onLeave();
    } else if (endReason === "takenBack" || onNoController === undefined) {
      onOver();
    } else {
      onNoController();
    }
  }, [endReason, onOver, onLeave, onNoController]);
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
 * twice, or a tap twice, within doublePressWithinMs, leave it, a held OK's repeats no second press;
 * one press of OK, a tap and every other key are the receiver's own controls'. Full screen left by the browser, not by the page, leaves it
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

/** A ref that always holds value as of the last render, for listeners set up once. */
function useLatest<T>(value: T): RefObject<T> {
  const ref = useRef(value);
  useEffect(() => {
    ref.current = value;
  });
  return ref;
}

/** The press that took a cast's control back, as its note says it. */
type TakenBackBy = "back" | "select" | "tap" | "historyBack";

/** Takes control back once, telling the cast's session by which press (and how, for OK) first. */
function takingBackOnce(
  connectionRef: RefObject<Pick<TogetherConnection, "note"> | undefined>,
  takeBackRef: RefObject<() => void>,
): (by: TakenBackBy, how?: string) => void {
  let hasLeft = false; // a held key's presses would each note it
  return (by, how = "") => {
    if (hasLeft) {
      return;
    }
    hasLeft = true;
    connectionRef.current?.note({ kind: "takenBack", by, ...(how === "" ? {} : { how }) });
    takeBackRef.current();
  };
}

/**
 * What the keys did between two presses of OK, for the log: the gap between them and how many keys
 * went up between them, which says whether a browser repeats a held OK as fresh presses (VQ122).
 */
function selectGaps(target: Window): {
  readonly heard: (now: number) => void;
  readonly how: (now: number) => string;
  /** Whether a key went up since the last OK: a held OK's repeats have none between them. */
  readonly isUpSince: () => boolean;
  readonly stop: () => void;
} {
  let previousAt = Number.NEGATIVE_INFINITY;
  let upsSince = 0;
  const keyUp = (): void => {
    upsSince += 1;
  };
  target.addEventListener("keyup", keyUp, true);
  return {
    heard: (now) => {
      previousAt = now;
      upsSince = 0;
    },
    how: (now) => `gapMs=${String(Math.round(now - previousAt))} upsBetween=${String(upsSince)}`,
    isUpSince: () => upsSince > 0,
    stop: () => {
      target.removeEventListener("keyup", keyUp, true);
    },
  };
}

/**
 * Hears taps on the page: a first pauses or plays the video (pauseOrPlayAtAClick), a second within
 * doublePressWithinMs undoes it and leaves. A click a key made (detail 0), or one following OK as the
 * same press, is not a tap. Returns the stopping.
 */
function listenForTaps(
  presses: { readonly lastSelectAt: number },
  isSecondPress: (now: number) => boolean,
  leave: () => void,
): () => void {
  let pausedOrPlayedByFirst: HTMLVideoElement | undefined;
  const onClick = (event: MouseEvent): void => {
    const now = performance.now();
    if (event.detail === 0 || now - presses.lastSelectAt <= samePressWithinMs) {
      return;
    }
    if (!isSecondPress(now)) {
      pausedOrPlayedByFirst = pauseOrPlayAtAClick(event);
      return;
    }
    undoPauseOrPlay(pausedOrPlayedByFirst);
    event.preventDefault();
    event.stopPropagation();
    leave();
  };
  window.addEventListener("click", onClick, true);
  return () => {
    window.removeEventListener("click", onClick, true);
  };
}

/**
 * Hears the presses that leave controlled mode: Back as a command, before anything else on the page
 * hears it, or as the person's step back from the receiver's own history entry, kept as it starts,
 * never a step the app makes itself (navigation's onPersonsStep, D384); OK or a tap
 * twice within doublePressWithinMs, the second press taken from the page (a held OK's repeats no
 * second press: Android's first comes 400-500 ms after the press, inside the window,
 * starts-chunks-remote-2026-10-29.md §3.3); and full screen left
 * unasked (useFullScreenLeftLeaves). Control is taken back once, however many of them come. A
 * mouse's click on the video pauses or plays it (pauseOrPlayAtAClick), which a second click making
 * it a double undoes, the film going on as it was.
 */
function useLeavingPresses(
  navigation: Navigation,
  onTakeBack: () => void,
  connection: Pick<TogetherConnection, "note"> | undefined,
): void {
  const takeBackRef = useOnce(onTakeBack);
  useFullScreenLeftLeaves(takeBackRef, connection);
  const connectionRef = useLatest(connection);
  useEffect(() => {
    // Which press took control back goes to the log: a held OK on the Fire TV took it, and the log
    // said only that the cast ended (VQ122, 2026-10-29).
    const leave = takingBackOnce(connectionRef, takeBackRef);
    const gaps = selectGaps(window);
    const presses = {
      lastPressAt: Number.NEGATIVE_INFINITY,
      lastSelectAt: Number.NEGATIVE_INFINITY,
    };
    const isSecondPress = (now: number): boolean => {
      const isSecond = now - presses.lastPressAt <= doublePressWithinMs;
      presses.lastPressAt = isSecond ? Number.NEGATIVE_INFINITY : now;
      return isSecond;
    };
    const stopCommands = onCommand(
      window,
      (detail, event) => {
        const now = performance.now();
        const isSelect = detail.command === "select";
        // Silk on a Fire TV sends a held OK's repeats as presses of their own, unmarked and with no key
        // up between them (VQ122: gapMs=433 upsBetween=0, then every ~35 ms): a second OK counts only
        // with a key up since the first.
        const isSecondOk =
          isSelect && detail.isRepeat !== true && isSecondPress(now) && gaps.isUpSince();
        if (detail.command === "back" || isSecondOk) {
          event.preventDefault();
          leave(isSelect ? "select" : "back", isSelect ? gaps.how(now) : "");
        }
        if (isSelect) {
          presses.lastSelectAt = now;
          gaps.heard(now);
        }
      },
      { isCapture: true },
    );
    const stopTaps = listenForTaps(presses, isSecondPress, () => {
      leave("tap");
    });
    navigation.go(navigation.route());
    // The person's own step alone: the step closing the Use question, which the page may be shown
    // before, took control back 16 ms after Use (staging, 2026-10-30; D384).
    const stopSteps = navigation.onPersonsStep(() => {
      leave("historyBack");
    });
    return () => {
      stopCommands();
      stopTaps();
      gaps.stop();
      stopSteps();
    };
  }, [navigation, takeBackRef, connectionRef]);
}

/**
 * Pauses or plays the video a mouse's click lands on, as a click on an ordinary page's video does,
 * whose browser controls answer it; the receiver's video has a television's controls instead (D338
 * §5), and a video with no browser controls does nothing at a click (the owner, Chrome, 2026-10-27).
 * A tap opens the controls alone, as an ordinary page's on a phone does. Returns the video it paused
 * or played, if any.
 */
function pauseOrPlayAtAClick(event: MouseEvent): HTMLVideoElement | undefined {
  const { target } = event;
  if (!(target instanceof HTMLVideoElement) || pressOf(event) !== "click") {
    return undefined;
  }
  answerPlayerCommand(target, "playPause");
  return target;
}

/**
 * Undoes the pause or play a double click's first click made (pauseOrPlayAtAClick), so the double
 * click that takes control back leaves the film as it was, playing on here (D338 §6).
 */
function undoPauseOrPlay(video: HTMLVideoElement | undefined): void {
  if (video !== undefined) {
    answerPlayerCommand(video, "playPause");
  }
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
 * Escape. The log is told first, as the press that asked for full screen was (D358 §2). Full screen
 * left because the page took its element away, the last video's stage as the next one opens (D364
 * §3), is no one's leaving: the receiver took control back as the second film of a session started
 * (the cast test, 2026-10-27).
 */
function useFullScreenLeftLeaves(
  takeBackRef: RefObject<() => void>,
  connection: Pick<TogetherConnection, "note"> | undefined,
): void {
  const connectionRef = useLatest(connection);
  useEffect(() => {
    let shown = document.fullscreenElement;
    const changed = (): void => {
      const wasShown = shown;
      shown = document.fullscreenElement;
      if (shown instanceof Element || isFullScreenLeaveAsked(document)) {
        return;
      }
      if (wasShown instanceof Element && !wasShown.isConnected) {
        return; // the page took it away
      }
      connectionRef.current?.note({ kind: "fullScreen", why: "left" });
      takeBackRef.current();
    };
    document.addEventListener("fullscreenchange", changed);
    return () => {
      document.removeEventListener("fullscreenchange", changed);
    };
  }, [takeBackRef, connectionRef]);
}
