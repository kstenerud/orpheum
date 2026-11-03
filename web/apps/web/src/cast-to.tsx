// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { CastSending, TogetherEndReason } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useCallback, useEffect, useRef, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { listMyDevices, type Device } from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import { CastRemote, type PlaysHereFrom } from "./cast-remote.tsx";
import {
  castAnswerGraceMs,
  castAnswerWithinMs,
  endCast,
  playInCast,
  startCast,
  type Cast,
  type CastFilm,
} from "./cast.ts";
import { dateTimeText } from "./date-time-text.ts";
import { useLive } from "./live.tsx";
import { ApiProblem, problemText } from "./problem.ts";
import { useServices, useToken } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { useResource, type Resource } from "./use-resource.ts";
import { useTogether } from "./use-together.ts";
import { watchingOf } from "./watching.ts";

/**
 * Setting a cast up (D364 §1, §2; D337 §2): the person's other devices open now, each by its name and
 * when it was last used, read again as that set changes; one chosen starts a session with it, playing
 * film, the video this device played as the person came here, if any (film), from where it stood.
 * The device is asked once whether to be used (D364 §2), and this page says it is asking until it
 * answers: its Use makes the session the server's and the live connection says so (useLive's
 * sending), and its Not now or no answer is said in a line. A cast this device sends unanswered that
 * this page is not waiting on, the dialog closed while it asked, shows asking too, to be cancelled
 * (AskingWithCancel). A device with no Orpheum tab in front says so, and a cast to it is refused
 * with what to do (D338 §2). A device receiving a cast is shown unavailable, and a cast to it,
 * raced, is refused with its name (D386 §1).
 */
export function CastTargets(props: { readonly film: () => CastFilm | undefined }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { connectedDeviceIds, inFrontDeviceIds, receivingDeviceIds, sending } = useLive();
  const [devices] = useResource(`cast-to/${connectedDeviceIds.join(",")}`, async () =>
    otherDevicesOpen(await listMyDevices(api), connectedDeviceIds),
  );
  const chosenNameRef = useRef("");
  const [asking, setAsking] = useState<Asking | undefined>(undefined);
  const [answered, setAnswered] = useState<string | undefined>(undefined);
  const [settledCastId, setSettledCastId] = useState<number | undefined>(undefined);
  const settled = (castId: number, said: string): void => {
    setSettledCastId(castId);
    setAsking(undefined);
    setAnswered(said);
  };
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    problemText(t, cause, {
      conflict: t("cast.notInFront", { name: chosenNameRef.current }),
      receivingACast: t("cast.receivingAnother", { name: chosenNameRef.current }),
    }),
  );
  const castTo = (device: Device): void => {
    chosenNameRef.current = device.name;
    setAnswered(undefined);
    runRequest(async () => {
      const cast = await startCast(api, device.id, props.film());
      setAsking({ cast, name: device.name });
    });
  };
  if (asking !== undefined || unansweredOf(sending, settledCastId) !== undefined) {
    return (
      <AskingNow
        asking={asking}
        sending={sending}
        settledCastId={settledCastId}
        onSettled={settled}
      />
    );
  }
  return (
    <section className="cast-to" aria-labelledby="cast-to-heading">
      <h2 id="cast-to-heading">{t("cast.heading")}</h2>
      <BusyGroup isBusy={isBusy}>
        <Targets
          devices={devices}
          states={{ inFrontDeviceIds, receivingDeviceIds }}
          onCast={castTo}
        />
        {problem === undefined ? null : <p role="alert">{problem}</p>}
        {answered === undefined ? null : <output>{answered}</output>}
      </BusyGroup>
    </section>
  );
}

/**
 * The cast this device is asking a receiver about: the one this page started (asking), or else one the
 * live connection says it sends unanswered and this page has not settled (settledCastId); onSettled
 * says which ended, and how it is said.
 */
function AskingNow(props: {
  readonly asking: Asking | undefined;
  readonly sending: CastSending | undefined;
  readonly settledCastId: number | undefined;
  readonly onSettled: (castId: number, said: string) => void;
}): JSX.Element | null {
  const { asking, onSettled } = props;
  if (asking !== undefined) {
    return (
      <AwaitingAnswer
        key={asking.cast.castId}
        asking={asking}
        sending={props.sending}
        onNotTaken={(said) => {
          onSettled(asking.cast.castId, said);
        }}
      />
    );
  }
  const unanswered = unansweredOf(props.sending, props.settledCastId);
  return unanswered === undefined ? null : (
    <AskingWithCancel
      key={unanswered.castId}
      castId={unanswered.castId}
      name={unanswered.receiverName}
      onCancelled={(said) => {
        onSettled(unanswered.castId, said);
      }}
    />
  );
}

/** A cast started, its receiver asked whether to be used (D364 §2), and the name it is asked on. */
interface Asking {
  readonly cast: Cast;
  readonly name: string;
}

/**
 * The sender waiting for its receiver's answer (D364 §2, D358 §1–§4): its Use is said by the live
 * connection (sending, accepted), after which the casting page shows the session's remote; Not now,
 * or no answer, ends the cast, said in a line (onNotTaken). A receiver that never answers, its page
 * gone, is given up on after castAnswerWithinMs and castAnswerGraceMs. Cancel casting request ends
 * it while it is asked, and a cast the live connection said this device sends, then no longer, is
 * over at once: the server lost it (a restart), and no answer will come (the owner, 2026-11-03: the
 * phone stayed asking).
 */
function AwaitingAnswer(props: {
  readonly asking: Asking;
  readonly sending: CastSending | undefined;
  readonly onNotTaken: (said: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const token = useToken();
  const { cast, name } = props.asking;
  const { snapshot } = useTogether({
    credential: cast.membershipCredential,
    sessionToken: token,
    role: "panel",
  });
  const isSent = props.sending?.castId === cast.castId;
  const isTaken = isSent && props.sending.isAccepted;
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  const settle = useSettlesOnce(props.onNotTaken);
  const isLost = useIsLost(isSent);
  useEffect(() => {
    if (isTaken) {
      settle(undefined); // the casting page shows the session from now on
    } else if (endReason !== undefined) {
      settle(notTakenText(t, endReason, name));
    } else if (isLost) {
      settle(t("cast.over.lost"));
    }
  }, [isTaken, endReason, isLost, name, t, settle]);
  const unansweredText = t("cast.answered.unanswered", { name });
  const unansweredRef = useRef(unansweredText);
  useEffect(() => {
    unansweredRef.current = unansweredText;
  });
  useEffect(() => {
    const timer = setTimeout(() => {
      if (settle(unansweredRef.current)) {
        endCast(api, cast.castId, "unanswered").catch(() => undefined);
      }
    }, castAnswerWithinMs + castAnswerGraceMs);
    return () => {
      clearTimeout(timer);
    };
  }, [api, cast.castId, settle]);
  return (
    <AskingWithCancel
      castId={cast.castId}
      name={name}
      onCancelled={(said) => {
        settle(said);
      }}
    />
  );
}

/**
 * The cast this device sends whose receiver has not said Use, unless this page has settled it
 * (settledCastId): the live connection keeps saying it until the server ends it.
 */
function unansweredOf(
  sending: CastSending | undefined,
  settledCastId: number | undefined,
): CastSending | undefined {
  return sending !== undefined && !sending.isAccepted && sending.castId !== settledCastId
    ? sending
    : undefined;
}

/**
 * "Asking ‹name›…" and Cancel casting request, which ends the cast, tried once more where the first
 * try fails (principles §28), and says a failure of both; a cast already gone is cancelled as well.
 * Shown for the cast this page started (AwaitingAnswer) and for one it did not start or no longer
 * waits on, which the live connection says this device sends unanswered: Cast's dialog closed while
 * it asked and opened again, or the page loaded again (the owner, 2026-11-03: the phone stayed
 * asking, its dialog closed, with no way to cancel).
 */
function AskingWithCancel(props: {
  readonly castId: number;
  readonly name: string;
  readonly onCancelled: (said: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { castId, name, onCancelled } = props;
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const cancel = (): void => {
    runRequest(async () => {
      const ending = (): Promise<unknown> => endCast(api, castId, "stopped").catch(unlessGone);
      await ending().catch(ending);
      onCancelled(t("cast.answered.stopped", { name }));
    });
  };
  return (
    <div className="cast-to">
      <output>{t("cast.asking", { name })}</output>{" "}
      <button type="button" disabled={isBusy} onClick={cancel}>
        {t("cast.cancelRequest")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </div>
  );
}

/** Takes a refusal that the cast is gone as its end, there being nothing left to end; rethrows others. */
function unlessGone(cause: unknown): undefined {
  if (cause instanceof ApiProblem && cause.code === "not_found") {
    return undefined;
  }
  throw cause;
}

/**
 * Settles a cast's asking once: what it is given then says why it was not taken (onNotTaken), or
 * undefined for taken, which says nothing; true the first time, false after.
 */
function useSettlesOnce(onNotTaken: (said: string) => void): (said: string | undefined) => boolean {
  const toldRef = useRef(onNotTaken);
  const isSettledRef = useRef(false);
  useEffect(() => {
    toldRef.current = onNotTaken;
  });
  return useCallback((said: string | undefined) => {
    if (isSettledRef.current) {
      return false;
    }
    isSettledRef.current = true;
    if (said !== undefined) {
      toldRef.current(said);
    }
    return true;
  }, []);
}

/** Whether a cast the live connection said this device sends (isSent) no longer is. */
function useIsLost(isSent: boolean): boolean {
  const [wasSent, setWasSent] = useState(false);
  if (isSent && !wasSent) {
    setWasSent(true);
  }
  return wasSent && !isSent;
}

/** What a sender says of a cast that ended before its receiver said Use. */
function notTakenText(t: TFunction, reason: TogetherEndReason, name: string): string {
  if (reason === "declined") {
    return t("cast.answered.declined", { name });
  }
  return reason === "unanswered"
    ? t("cast.answered.unanswered", { name })
    : t("cast.answered.ended", { name });
}

/** Which of the person's devices have an Orpheum tab in front, and which receive a cast now. */
interface TargetStates {
  readonly inFrontDeviceIds: readonly number[];
  readonly receivingDeviceIds: readonly number[];
}

/** The devices to cast to as they are read: Loaded's words while loading or failed. */
function Targets(props: {
  readonly devices: Resource<Device[]>;
  readonly states: TargetStates;
  readonly onCast: (device: Device) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { devices } = props;
  if (devices.state === "loading") {
    return <p>{t("common.loading")}</p>;
  }
  if (devices.state === "failed") {
    return <p role="alert">{problemText(t, devices.cause)}</p>;
  }
  return devices.value.length === 0 ? (
    <p>{t("cast.none")}</p>
  ) : (
    <TargetList devices={devices.value} states={props.states} onCast={props.onCast} />
  );
}

/**
 * The devices to cast to: each a button with its name and when it was last used, or, with no Orpheum
 * tab in front, that one must be brought to the front there (D338 §2); one receiving a cast is
 * unavailable, its button disabled, until that cast ends (D386 §1).
 */
function TargetList(props: {
  readonly devices: readonly Device[];
  readonly states: TargetStates;
  readonly onCast: (device: Device) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { inFrontDeviceIds, receivingDeviceIds } = props.states;
  const stateOf = (device: Device): string => {
    if (receivingDeviceIds.includes(device.id)) {
      return t("cast.showingAnother");
    }
    return inFrontDeviceIds.includes(device.id)
      ? t("cast.connectedNow", { used: dateTimeText(device.lastUsedAt, i18n.language) })
      : t("cast.noTabInFront");
  };
  return (
    <ul className="cast-to__devices">
      {props.devices.map((device) => (
        <li key={device.id}>
          <button
            type="button"
            disabled={receivingDeviceIds.includes(device.id)}
            onClick={() => {
              props.onCast(device);
            }}
          >
            {device.name}
          </button>{" "}
          <span>{stateOf(device)}</span>
        </li>
      ))}
    </ul>
  );
}

/**
 * The person's devices open now but this one, as their list orders them, most recently used first,
 * and none in the Watch Together session they are in, whose cast would take it from the session (D384
 * §3: the owner's S23, casting to the Mac that hosted the session, ended it, 2026-10-30).
 */
export function otherDevicesOpen(
  devices: readonly Device[],
  connectedDeviceIds: readonly number[],
): Device[] {
  return devices.filter(
    (device) =>
      !device.isThisDevice &&
      device.isSignedIn &&
      device.isInYourWatchTogether !== true &&
      connectedDeviceIds.includes(device.id),
  );
}

/**
 * How long a Play page waits to hear from the server whether this device casts before it plays here,
 * in milliseconds: the live connection says so as it opens, a round trip after a page is loaded; a
 * server that has not said by then plays the film here rather than hold it up. A justified constant.
 */
export const sendingKnownWithinMs = 3000;

/**
 * Where the Play page's film plays here from: atMs, or where the person left it where undefined;
 * paused there where isPaused, as a cast's receiver had it as its sender took the film over (D385 §2).
 */
export interface PlayedHereFrom {
  readonly atMs: number | undefined;
  readonly isPaused?: boolean;
  /** Which time the page plays it here, so playing it again here starts its playback afresh. */
  readonly round: number;
}

/**
 * Where a Play page's film plays, the page keeping its place throughout (D364 §3, D384 §2): on a
 * device that casts, it goes on to the receiver, from atMs or, undefined, where the person left it,
 * and the page's video area becomes the cast's remote (CastRemote); a cast started from Cast's dialog
 * over the page, moving its film, does the same. One played here on purpose (shouldPlayHere), once the
 * server says the device casts nothing, or not within sendingKnownWithinMs, plays here (playHere), as
 * it does again once the cast is stopped, from where the receiver stood. Nothing plays while a cast
 * the tab received before the page loaded again is in doubt (Live's isReceivingInDoubt, D383 §2).
 */
export function PlayOrSendOn(props: {
  readonly fileId: number;
  readonly atMs?: number | undefined;
  /** Whether the film played here waits paused at atMs (PlayedHereFrom). */
  readonly isPaused?: boolean | undefined;
  readonly shouldPlayHere: boolean;
  readonly playHere: (from: PlayedHereFrom) => ReactNode;
}): ReactNode {
  const { t } = useTranslation();
  const { where, playHereFrom, sentTo } = useWhereItPlays(props);
  useSendsOn(where, props, { onSent: sentTo, onFailed: playHereFrom });
  const { isReceivingInDoubt } = useLive();
  if (isReceivingInDoubt) {
    return null; // a cast this tab received before it loaded again, which may be over (D383 §2)
  }
  switch (where.kind) {
    case "here":
      return props.playHere(where.from);
    case "sending":
      return <p>{t("cast.sendingOn")}</p>;
    case "remote":
      return <CastRemote castId={where.castId} fileId={props.fileId} onPlayHere={playHereFrom} />;
    case "unknown":
      return null;
  }
}

/**
 * Where a Play page's film plays: here; being sent on to the cast castId's receiver; sent, the page
 * its remote; or not known yet.
 */
type PlaysWhere =
  | { readonly kind: "here"; readonly from: PlayedHereFrom; readonly leftCastId?: number }
  | { readonly kind: "sending"; readonly castId: number }
  | { readonly kind: "remote"; readonly castId: number }
  | { readonly kind: "unknown" };

/**
 * Where a Play page's film plays (PlayOrSendOn), first decided once the server says what the device
 * sends, or after sendingKnownWithinMs: here when shouldPlayHere or the device sends no cast its
 * receiver said Use to; the remote at once where that cast plays this film already; else sent on.
 * Playing here, a cast that takes this film on (started from Cast's dialog) makes the page its
 * remote; playHereFrom plays it here again, as the cast ends.
 */
function useWhereItPlays(props: {
  readonly atMs?: number | undefined;
  readonly isPaused?: boolean | undefined;
  readonly fileId: number;
  readonly shouldPlayHere: boolean;
}): {
  readonly where: PlaysWhere;
  readonly playHereFrom: PlaysHereFrom;
  /** The film went on to the cast castId's receiver: the page is its remote. */
  readonly sentTo: (castId: number) => void;
} {
  const { sending, isSendingKnown } = useLive();
  const [where, setWhere] = useState<PlaysWhere>(() =>
    props.shouldPlayHere ? hereFrom(props.atMs, 0, props.isPaused) : { kind: "unknown" },
  );
  const isTimedOut = useTimesOut(where.kind === "unknown" && !isSendingKnown);
  const taken = sending?.isAccepted === true ? sending : undefined;
  if (where.kind === "unknown" && (isSendingKnown || isTimedOut)) {
    setWhere(firstWhere(isSendingKnown ? taken : undefined, props));
  } else if (isTakenOn(where, taken, props.fileId)) {
    setWhere({ kind: "remote", castId: taken.castId }); // a cast that took this film on
  }
  const playHereFrom = useCallback((atMs: number | undefined, isPaused?: boolean) => {
    setWhere((now) => ({
      ...hereFrom(atMs, now.kind === "here" ? now.from.round + 1 : 1, isPaused),
      ...(now.kind === "remote" || now.kind === "sending" ? { leftCastId: now.castId } : {}),
    }));
  }, []);
  const sentTo = useCallback((castId: number) => {
    setWhere({ kind: "remote", castId });
  }, []);
  return { where, playHereFrom, sentTo };
}

/**
 * Whether taken, a cast its receiver said Use to, took on the page's film fileId while it played here:
 * one started from Cast's dialog over the page, not one the page played here from.
 */
function isTakenOn(
  where: PlaysWhere,
  taken: CastSending | undefined,
  fileId: number,
): taken is CastSending {
  return where.kind === "here" && taken?.fileId === fileId && taken.castId !== where.leftCastId;
}

/** Playing here from atMs, the round-th time, paused there where isPaused. */
function hereFrom(atMs: number | undefined, round: number, isPaused?: boolean): PlaysWhere {
  return { kind: "here", from: { atMs, round, ...(isPaused === true ? { isPaused } : {}) } };
}

/** Where the page's film plays first, given the cast taken, if any, its receiver having said Use. */
function firstWhere(
  taken: CastSending | undefined,
  page: {
    readonly atMs?: number | undefined;
    readonly isPaused?: boolean | undefined;
    readonly fileId: number;
  },
): PlaysWhere {
  if (taken === undefined) {
    return hereFrom(page.atMs, 0, page.isPaused);
  }
  return taken.fileId === page.fileId
    ? { kind: "remote", castId: taken.castId }
    : { kind: "sending", castId: taken.castId };
}

/** Whether sendingKnownWithinMs has gone by while isWaiting. */
function useTimesOut(isWaiting: boolean): boolean {
  const [isTimedOut, setIsTimedOut] = useState(false);
  useEffect(() => {
    if (!isWaiting) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setIsTimedOut(true);
    }, sendingKnownWithinMs);
    return () => {
      clearTimeout(timer);
    };
  }, [isWaiting]);
  return isTimedOut;
}

/**
 * Sends the film on to the cast's receiver, from atMs, once where says it is sent, telling onSent, the
 * page then its remote; a send that fails plays it here (onFailed). The film's warm-up, preload and
 * playback here are let go of while the page is a remote (watching.ts's sentAway).
 */
function useSendsOn(
  where: PlaysWhere,
  film: { readonly fileId: number; readonly atMs?: number | undefined },
  ways: {
    readonly onSent: (castId: number) => void;
    readonly onFailed: (atMs: number | undefined) => void;
  },
): void {
  const { api } = useServices();
  const { fileId, atMs } = film;
  const castId = where.kind === "sending" ? where.castId : undefined;
  const sentRef = useRef<number | undefined>(undefined);
  const waysRef = useRef(ways);
  useEffect(() => {
    waysRef.current = ways;
  });
  useEffect(() => {
    if (castId === undefined || sentRef.current === castId) {
      return;
    }
    sentRef.current = castId;
    playInCast(api, castId, { fileId, positionMs: atMs }).then(
      () => {
        waysRef.current.onSent(castId);
      },
      () => {
        waysRef.current.onFailed(atMs);
      },
    );
  }, [api, castId, fileId, atMs]);
  const isRemote = where.kind === "remote";
  useEffect(() => {
    if (isRemote) {
      watchingOf(api).sentAway(fileId);
    }
  }, [api, isRemote, fileId]);
}
