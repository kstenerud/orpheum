// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { Api, CannotKeepUp, CastToReceive } from "@orpheum/sdk";
import { useCallback, useEffect, useMemo, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { endCast, moveCastOn } from "./cast.ts";
import { fillsWindowClass } from "./film-fullscreen.ts";
import { onCommand } from "./input-commands.ts";
import { useServices, useToken } from "./services.tsx";
import { useLive } from "./live.tsx";
import type { Navigation } from "./navigation.ts";
import { HostPlaysTogether, SoundOn, useLeadingPlayer } from "./together-host.tsx";
import { useTogether } from "./use-together.ts";

/**
 * How long the "Controlled from" line stays before it fades, in milliseconds: long enough to read a
 * short line across a room. A justified constant.
 */
export const castNoteShownMs = 5000;

/** The cast this device receives of fileId, while it does, and how it ends here (D337). */
export function useReceiving(fileId: number): Receiving | undefined {
  const { receiving, endReceiving, leaveReceiving } = useLive();
  return useMemo(
    () =>
      receiving?.fileId === fileId
        ? { cast: receiving, onOver: endReceiving, onLeave: leaveReceiving }
        : undefined,
    [receiving, fileId, endReceiving, leaveReceiving],
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
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  useEffect(() => {
    // A cast replacing this one is received already; any other end leaves an ordinary player.
    if (endReason === undefined || endReason === "replaced") {
      return;
    }
    if (endReason === "stopped" || endReason === "filmEnded") {
      onLeave();
    } else {
      onOver();
    }
  }, [endReason, onOver, onLeave]);
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
      <SoundOn video={video} connection={connection} />
      <ControlledMode fromName={cast.fromDeviceName} onTakeBack={takeBack} />
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
 * key are the receiver's own controls'. A "Controlled from" line, which fades after castNoteShownMs,
 * says who controls it and how to leave.
 */
export function ControlledMode(props: {
  readonly fromName: string;
  readonly onTakeBack: () => void;
}): JSX.Element {
  const { navigation } = useServices();
  useLeavingPresses(navigation, props.onTakeBack);
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
 * hears it, or as a step back from the receiver's own history entry, kept as it starts; and OK or a
 * tap twice within doublePressWithinMs, the second press taken from the page.
 */
function useLeavingPresses(navigation: Navigation, onTakeBack: () => void): void {
  const takeBackRef = useRef(onTakeBack);
  useEffect(() => {
    takeBackRef.current = onTakeBack;
  });
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
  }, [navigation]);
}
