// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { Api, CannotKeepUp, CastToReceive } from "@orpheum/sdk";
import { useCallback, useEffect, useMemo, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { endCast, moveCastOn } from "./cast.ts";
import { fillsWindowClass } from "./film-fullscreen.ts";
import type { Command } from "./input-buttons.ts";
import { useCommands } from "./input-layer.tsx";
import { useServices, useToken } from "./services.tsx";
import { useLive } from "./live.tsx";
import type { Navigation } from "./navigation.ts";
import { HostPlaysTogether, SoundOn, useLeadingPlayer } from "./together-host.tsx";
import { useTogether } from "./use-together.ts";

/**
 * How long the "Controlled from" line stays before it fades, and the take-back prompt waits for its
 * second press, in milliseconds: long enough to read a short line across a room. A justified
 * constant.
 */
export const castNoteShownMs = 5000;

/** The commands the receiver's own controls answer while no prompt is up (§6.4): its media keys. */
const ownControls: ReadonlySet<Command> = new Set([
  "playPause",
  "seekBack",
  "seekForward",
  "left",
  "right",
]);

/** The cast this device receives of fileId, while it does, and how it ends here (D337). */
export function useReceiving(fileId: number): Receiving | undefined {
  const { receiving, endReceiving } = useLive();
  return useMemo(
    () => (receiving?.fileId === fileId ? { cast: receiving, onOver: endReceiving } : undefined),
    [receiving, fileId, endReceiving],
  );
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
 * the receiver goes home, the remote told (D337's §9 answers, Q4; plans/up-next.md).
 */
export function moveOnOrGoHome(api: Api, navigation: Navigation, receiving: Receiving): void {
  moveCastOn(api, receiving.cast.castId).catch(() => {
    endCast(api, receiving.cast.castId, "filmEnded").catch(() => undefined);
    receiving.onOver();
    navigation.go({ screen: "home" });
  });
}

/** What the Play page is given while this device receives a cast. */
export interface Receiving {
  readonly cast: CastToReceive;
  /** Told that receiving is over here: control taken back, or the cast ended. */
  readonly onOver: () => void;
}

/**
 * The receiver of a cast (D337 §3–§7; plans/cast-to-device.md §4, §6): its video leads the cast's
 * session of one, from where the sender stood, filling the window with nothing on it but a
 * "Controlled from" line that fades; its own media keys go on working, and every change of theirs
 * reaches the remote as the session's; any other key, a tap, or Back raises Take back control, and a
 * second press takes it. The cast stopped by the remote pauses it; at the film's end it plays the
 * next episode as the sender would (Up next, plans/up-next.md), and where none plays next it goes
 * home.
 */
export function CastReceiver(props: {
  readonly receiving: Receiving;
  readonly fileId: number;
  readonly video: HTMLVideoElement | null;
  readonly startPositionMs: number;
  /** Whether an episode plays after the film, whose Up next moves the cast on to it at its end. */
  readonly hasNext?: boolean;
}): JSX.Element {
  const { api, navigation } = useServices();
  const token = useToken();
  const { cast, onOver } = props.receiving;
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
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  useEffect(() => {
    // A cast replacing this one is received already; any other end leaves an ordinary player.
    if (endReason === undefined || endReason === "replaced") {
      return;
    }
    if (endReason === "stopped") {
      video?.pause();
    }
    onOver();
  }, [endReason, onOver, video]);
  const hasNext = props.hasNext === true;
  useEffect(() => {
    if (hasNext) {
      return undefined; // its Up next moves the cast on, or ends it, as the person's autoplay says
    }
    const atTheEnd = (): void => {
      endCast(api, cast.castId, "filmEnded").catch(() => undefined);
      onOver();
      navigation.go({ screen: "home" });
    };
    video?.addEventListener("ended", atTheEnd);
    return () => {
      video?.removeEventListener("ended", atTheEnd);
    };
  }, [api, cast.castId, navigation, onOver, video, hasNext]);
  const takeBack = useCallback(() => {
    endCast(api, cast.castId, "takenBack").catch(() => undefined);
    onOver();
  }, [api, cast.castId, onOver]);
  return (
    <>
      <SoundOn video={video} connection={connection} />
      <TakeBackPrompt fromName={cast.fromDeviceName} onTakeBack={takeBack} />
    </>
  );
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
 * The "Controlled from" line, fading after castNoteShownMs, and Take back control: raised by any key
 * but the media keys, a tap, or Back (a Fire TV's is a history step, never a key: the page keeps an
 * entry of its own to step back to and keeps it again); one more OK, tap or Back takes control, and
 * castNoteShownMs without one hides it again (D337 §4).
 */
export function TakeBackPrompt(props: {
  readonly fromName: string;
  readonly onTakeBack: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const [isAsking, setIsAsking] = useState(false);
  const isAskingRef = useRef(false);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const { onTakeBack } = props;
  const press = useCallback(() => {
    clearTimeout(hideTimerRef.current);
    if (isAskingRef.current) {
      onTakeBack();
      return;
    }
    isAskingRef.current = true;
    setIsAsking(true);
    hideTimerRef.current = setTimeout(() => {
      isAskingRef.current = false;
      setIsAsking(false);
    }, castNoteShownMs);
  }, [onTakeBack]);
  useEffect(
    () => () => {
      clearTimeout(hideTimerRef.current);
    },
    [],
  );
  useCommands(
    "document",
    (detail, event) => {
      if (isAskingRef.current ? isAMove(detail.command) : ownControls.has(detail.command)) {
        return; // the player's own, or a move about the prompt
      }
      event.preventDefault();
      press();
    },
    { isCapture: true },
  );
  useBackAsAPress(navigation, press);
  useTapAsAPress(press);
  return (
    <div className="cast-receiver">
      <ControlledFrom name={props.fromName} />
      {isAsking ? (
        <div className="cast-receiver__prompt" role="alertdialog" aria-label={t("cast.takeBack")}>
          <p>{t("cast.takeBackExplain", { name: props.fromName })}</p>
          <TakeBackButton onTakeBack={onTakeBack} />
        </div>
      ) : null}
    </div>
  );
}

/** "Controlled from" the sender, which fades and goes after castNoteShownMs (D337 §3). */
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

/** Whether a command only moves about the page: an arrow. */
function isAMove(command: Command): boolean {
  return command === "up" || command === "down" || command === "left" || command === "right";
}

/** The button that takes control back, focused as it shows so OK presses it. */
function TakeBackButton(props: { readonly onTakeBack: () => void }): JSX.Element {
  const { t } = useTranslation();
  const ref = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, []);
  return (
    <button ref={ref} type="button" className="cast-receiver__take-back" onClick={props.onTakeBack}>
      {t("cast.takeBack")}
    </button>
  );
}

/**
 * Hears Back as a press: the page keeps an entry of its own in the history, the receiver's, and a
 * step back from it is a press, the entry kept again for the next.
 */
function useBackAsAPress(
  navigation: ReturnType<typeof useServices>["navigation"],
  press: () => void,
): void {
  const pressRef = useRef(press);
  useEffect(() => {
    pressRef.current = press;
  });
  useEffect(() => {
    navigation.go(navigation.route());
    const onBack = (): void => {
      navigation.go(navigation.route());
      pressRef.current();
    };
    window.addEventListener("popstate", onBack);
    return () => {
      window.removeEventListener("popstate", onBack);
    };
  }, [navigation]);
}

/** Hears a tap or a click anywhere as a press, before what it lands on: nothing else is on screen. */
function useTapAsAPress(press: () => void): void {
  useEffect(() => {
    const onTap = (event: Event): void => {
      if (event.target instanceof Element && event.target.closest(".cast-receiver__take-back")) {
        return; // the button's own click takes control
      }
      event.preventDefault();
      event.stopPropagation();
      press();
    };
    document.addEventListener("click", onTap, true);
    return () => {
      document.removeEventListener("click", onTap, true);
    };
  }, [press]);
}
