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
import { useTranslation } from "react-i18next";

import { pressOf, type CastAnswer } from "./cast-asked.tsx";
import { moveCastOn, playInCast } from "./cast.ts";
import { fillsWindowClass, leaveFullScreenAsked, setFillsWindow } from "./film-fullscreen.ts";
import { commandOf, keyCodeOf } from "./input-buttons.ts";
import { onCommand } from "./input-commands.ts";
import {
  useFilmChannelWorkedByTheRemote,
  useSoundWorkedByTheRemote,
  useToldTheCastHowItPlays,
} from "./cast-receiver-sound.ts";
import { useLatest } from "./latest.ts";
import { useServices, useToken } from "./services.tsx";
import { useLive } from "./live.tsx";
import type { Navigation } from "./navigation.ts";
import { showNotice } from "./notice.ts";
import { answerPlayerCommand } from "./player-commands.ts";
import { HostPlaysTogether, useLeadingPlayer } from "./together-host.tsx";
import { onTheFilmsStage } from "./together-over-the-film.tsx";
import { SoundPanelContext } from "./sound-panel.ts";
import { usePictureWorkedByTheRemote } from "./cast-receiver-picture.ts";
import { isTelevision } from "./tv-device.ts";
import { useTogether } from "./use-together.ts";

/**
 * How long the "Controlled from" line stays before it fades, in milliseconds: long enough to read a
 * short line across a room. A justified constant.
 */
export const castNoteShownMs = 5000;

/** The cast this device receives of fileId, while it does, and how it ends here (D337). */
export function useReceiving(fileId: number): Receiving | undefined {
  const { receiving, answer, leaveReceiving } = useLive();
  return useMemo(() => {
    if (receiving?.fileId !== fileId) {
      return undefined;
    }
    const its = answer?.castId === receiving.castId ? answer : undefined;
    return {
      cast: receiving,
      onLeave: leaveReceiving,
      ...(its === undefined ? {} : { answer: its }),
    };
  }, [receiving, answer, fileId, leaveReceiving]);
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
 * home (D384 §1).
 */
export function readyForTheNext(api: Api, receiving: Receiving): void {
  playInCast(api, receiving.cast.castId, undefined).catch(() => {
    receiving.onLeave();
  });
}

/** What the Play page is given while this device receives a cast. */
export interface Receiving {
  readonly cast: CastToReceive;
  /**
   * Told that receiving is over and the device goes to its home page: the cast ended, however it
   * ended, control taken back here among them (D384 §1, D385 §1).
   */
  readonly onLeave: () => void;
  /**
   * The press that answered Play to the cast, where this tab was asked (D358 §2); absent for a cast
   * received at once, as one replacing another this tab received.
   */
  readonly answer?: CastAnswer;
}

/**
 * At a film's end with no episode after it, the receiver waits ready for the next video (D364 §5);
 * with one, its Up next moves the cast on, or waits ready, as the person's autoplay says.
 */
function useReadyAtTheEnd(at: {
  readonly api: ReturnType<typeof useServices>["api"];
  readonly receiving: Receiving;
  readonly video: HTMLVideoElement | null;
  readonly hasNext: boolean;
}): void {
  const { api, receiving, video, hasNext } = at;
  useEffect(() => {
    if (hasNext) {
      return undefined;
    }
    const atTheEnd = (): void => {
      readyForTheNext(api, receiving);
    };
    video?.addEventListener("ended", atTheEnd);
    return () => {
      video?.removeEventListener("ended", atTheEnd);
    };
  }, [api, receiving, video, hasNext]);
}

/**
 * The receiver of a cast (D337 §3–§7, D338; plans/cast-to-device.md §4, §6): its video leads the
 * cast's session of one, from where the sender stood, filling the window, with a television's player
 * controls over it (the Play page's composition) and a "Controlled from" line that fades; its own
 * controls and media keys go on working, and every change of theirs reaches the remote as the
 * session's. While its person hosts Watch Together, its video leads that session instead, as their
 * player, so what they play plays for the group, the cast's session told only how it ends (D364 §11).
 * Nothing pressed here ends the cast, nor does leaving full screen: the device that cast ends it
 * (ControlledMode). Every end, stopped by the remote among them, sends it home (D384 §1); at the film's end it
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
  const { cast, onLeave } = props.receiving;
  const { video, fileId } = props;
  const soundPanel = use(SoundPanelContext); // its sound controls, the remote's too (sound-panel.ts)
  const { connection, isWelcomed, snapshot, castConnection } = useReceiversSessions(cast);
  const picturePanel = usePictureWorkedByTheRemote({ cast: castConnection, led: connection });
  useLeadingPlayer({
    connection,
    isWelcomed,
    fileId,
    video,
    startPositionMs: props.startPositionMs,
    putOnAtMs: cast.positionMs,
    delivery: props.delivery,
    readSound: soundPanel?.read,
    readPicture: picturePanel?.read,
  });
  useSoundWorkedByTheRemote({ cast: castConnection, led: connection, panel: soundPanel });
  useFilmChannelWorkedByTheRemote(video);
  useToldTheCastHowItPlays({
    cast: castConnection === connection ? undefined : castConnection,
    video,
    delivery: props.delivery,
    panel: soundPanel,
  });
  useFillsTheWindow(video);
  useToldHowItStarted({ connection, isWelcomed, video, answer: props.receiving.answer });
  useEndsAsTheCastDoes(snapshot?.status === "ended" ? snapshot.endReason : undefined, onLeave);
  useReadyAtTheEnd({ api, receiving: props.receiving, video, hasNext: props.hasNext === true });
  return (
    <>
      <SoundAtAnyPress video={video} connection={connection} />
      <ControlledMode fromName={cast.fromDeviceName} />
    </>
  );
}

/**
 * The sessions a cast's receiver is in: the cast's own, which says how it ends, and the one its video
 * leads, the cast's or, while its person hosts Watch Together, that session, as their player (D364
 * §11), the cast's then joined as a panel, playing nothing. Gives the led session's connection and
 * whether it is welcomed, and the cast's own snapshot and connection, which its remote works.
 */
function useReceiversSessions(cast: CastToReceive): {
  readonly connection: (TogetherConnection & CarriesSoundChoices) | undefined;
  readonly isWelcomed: boolean;
  readonly snapshot: TogetherSnapshot | undefined;
  readonly castConnection: ReturnType<typeof useTogether>["connection"];
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
    castConnection: castSession.connection,
  };
}

/**
 * Ends receiving as the cast ends, however it ends, sending the receiver home (onLeave, D384 §1):
 * control taken back here (D385 §1), Stop casting on the sender, its session's end or its controller
 * gone (as its receiver was gone too long). No other cast takes it over (D386 §1). A cast the server
 * no longer knows ("gone", as after a restart) is said in a pop-up there (D386 §2).
 */
export function useEndsAsTheCastDoes(
  endReason: TogetherEndReason | undefined,
  onLeave: () => void,
): void {
  const { t } = useTranslation();
  useEffect(() => {
    if (endReason === undefined) {
      return;
    }
    onLeave();
    if (endReason === "gone") {
      showNotice(t("cast.over.lost")); // after onLeave's way home, the screen it stays on
    }
  }, [endReason, onLeave, t]);
}

/** Has the video's stage fill the window while it receives, by layout, as no press asked (§4). */
function useFillsTheWindow(video: HTMLVideoElement | null): void {
  useEffect(() => {
    const stage = video?.closest(".film-player__stage");
    if (!(stage instanceof HTMLElement) || stage.classList.contains(fillsWindowClass)) {
      return undefined;
    }
    setFillsWindow(stage, true);
    return () => {
      setFillsWindow(stage, false);
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
 * (Escape, a modifier, a shortcut) or Back, which leaves full screen, and a click or a tap anywhere.
 * That press does nothing else, so OK does not pause the film, nor does a click OK makes after it
 * (samePressWithinMs).
 */
export function SoundAtAnyPress(props: {
  readonly video: HTMLVideoElement | null;
  readonly connection: Pick<TogetherConnection, "note"> | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { video } = props;
  const muted = useMutedByTheBrowser(video);
  const latestRef = useLatest(props);
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
    [latestRef, setIsMuted],
  );
  useFirstPressTurnsSoundOn(muted.isMutedRef, turnOn);
  if (!muted.isMuted || video === null) {
    return null;
  }
  const line = <output className="cast-receiver__sound-on">{soundOnText(t)}</output>;
  return onTheFilmsStage(video, line);
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
 * shortcut, and Back, which leaves full screen as it always does (ControlledMode).
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
 * How soon after an OK a click is the same press, in milliseconds: a TV browser may follow OK's key
 * with the click it makes on what has focus, which is not a second press. A justified constant, well
 * under any two presses a hand makes.
 */
const samePressWithinMs = 100;

/**
 * The receiver's controlled mode: a "Controlled from" line, which fades after castNoteShownMs, says
 * who controls it. Nothing pressed here ends the cast, and neither does leaving full screen, a film
 * playing or not: ending it stays with the device that cast (the owner, 2026-11-07, the 11-20
 * batch's answer 4, amending D385 §1 and D338 §6, by which Back, Escape, OK twice and full screen
 * left took control back: Escape on a receiver with nothing playing ended the cast). Back leaves full
 * screen where the page is in it, as a Fire TV's Back and a desktop's Escape do there unheard, and is
 * the receiver's own otherwise, so it stays on its page; a click on the video pauses or plays it.
 */
export function ControlledMode(props: { readonly fromName: string }): JSX.Element {
  const { navigation } = useServices();
  useBackStaysHere(navigation);
  useClicksPauseOrPlay();
  return (
    <div className="cast-receiver">
      <ControlledFrom name={props.fromName} />
    </div>
  );
}

/** "Controlled from" the sender, which fades and goes after castNoteShownMs. */
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
 * Keeps Back on the receiver's page: Back as a command, heard before anything else on the page,
 * leaves full screen where the page is in it and otherwise does nothing, where the Play page's would
 * leave the page; and a Fire TV's Back, a step of the history and never a key (idiosyncrasies.md),
 * steps off an entry held over the page (holdBack), held again each time, never off the page.
 */
function useBackStaysHere(navigation: Navigation): void {
  useEffect(() => {
    const stopCommands = onCommand(
      window,
      (detail, event) => {
        if (detail.command !== "back") {
          return;
        }
        event.preventDefault();
        if (document.fullscreenElement instanceof Element) {
          leaveFullScreenAsked(document);
        }
      },
      { isCapture: true },
    );
    let release = (): void => undefined;
    const hold = (): void => {
      release = navigation.holdBack(hold);
    };
    hold();
    return () => {
      stopCommands();
      release();
    };
  }, [navigation]);
}

/**
 * Pauses or plays the video a mouse's click lands on (pauseOrPlayAtAClick); a click a key made
 * (detail 0), or one following OK as the same press, is the player's OK, not a click.
 */
function useClicksPauseOrPlay(): void {
  useEffect(() => {
    let lastSelectAt = Number.NEGATIVE_INFINITY;
    const stopCommands = onCommand(
      window,
      (detail) => {
        if (detail.command === "select") {
          lastSelectAt = performance.now();
        }
      },
      { isCapture: true },
    );
    const onClick = (event: MouseEvent): void => {
      if (event.detail === 0 || performance.now() - lastSelectAt <= samePressWithinMs) {
        return;
      }
      pauseOrPlayAtAClick(event);
    };
    window.addEventListener("click", onClick, true);
    return () => {
      stopCommands();
      window.removeEventListener("click", onClick, true);
    };
  }, []);
}

/**
 * Pauses or plays the video a mouse's click lands on, as a click on an ordinary page's video does,
 * whose browser controls answer it; the receiver's video has a television's controls instead (D338
 * §5), and a video with no browser controls does nothing at a click (the owner, Chrome, 2026-10-27).
 * A tap opens the controls alone, as an ordinary page's on a phone does.
 */
function pauseOrPlayAtAClick(event: MouseEvent): void {
  const { target } = event;
  if (target instanceof HTMLVideoElement && pressOf(event) === "click") {
    answerPlayerCommand(target, "playPause");
  }
}
