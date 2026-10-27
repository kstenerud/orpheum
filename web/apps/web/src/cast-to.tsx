// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { CastSending, TogetherEndReason } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useEffect, useRef, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { listMyDevices, type Device } from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
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
import { problemText } from "./problem.ts";
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
 * sending), and its Not now or no answer is said in a line. A device with no Orpheum tab in front
 * says so, and a cast to it is refused with what to do (D338 §2).
 */
export function CastTargets(props: { readonly film: () => CastFilm | undefined }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { connectedDeviceIds, inFrontDeviceIds, sending } = useLive();
  const [devices] = useResource(`cast-to/${connectedDeviceIds.join(",")}`, async () =>
    otherDevicesOpen(await listMyDevices(api), connectedDeviceIds),
  );
  const chosenNameRef = useRef("");
  const [asking, setAsking] = useState<Asking | undefined>(undefined);
  const [answered, setAnswered] = useState<string | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    problemText(t, cause, { conflict: t("cast.notInFront", { name: chosenNameRef.current }) }),
  );
  const castTo = (device: Device): void => {
    chosenNameRef.current = device.name;
    setAnswered(undefined);
    runRequest(async () => {
      const cast = await startCast(api, device.id, props.film());
      setAsking({ cast, name: device.name });
    });
  };
  if (asking !== undefined) {
    return (
      <AwaitingAnswer
        key={asking.cast.castId}
        asking={asking}
        sending={sending}
        onNotTaken={(said) => {
          setAsking(undefined);
          setAnswered(said);
        }}
      />
    );
  }
  return (
    <section className="cast-to" aria-labelledby="cast-to-heading">
      <h2 id="cast-to-heading">{t("cast.heading")}</h2>
      <BusyGroup isBusy={isBusy}>
        <Targets devices={devices} inFrontDeviceIds={inFrontDeviceIds} onCast={castTo} />
        {problem === undefined ? null : <p role="alert">{problem}</p>}
        {answered === undefined ? null : <output>{answered}</output>}
      </BusyGroup>
    </section>
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
 * gone, is given up on after castAnswerWithinMs and castAnswerGraceMs.
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
  const isTaken = props.sending?.castId === cast.castId && props.sending.isAccepted;
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  const toldRef = useRef(props.onNotTaken);
  const isSettledRef = useRef(false);
  useEffect(() => {
    toldRef.current = props.onNotTaken;
  });
  useEffect(() => {
    if (isSettledRef.current) {
      return;
    }
    if (isTaken) {
      isSettledRef.current = true; // the casting page shows the session from now on
    } else if (endReason !== undefined) {
      isSettledRef.current = true;
      toldRef.current(notTakenText(t, endReason, name));
    }
  }, [isTaken, endReason, name, t]);
  const unansweredText = t("cast.answered.unanswered", { name });
  const unansweredRef = useRef(unansweredText);
  useEffect(() => {
    unansweredRef.current = unansweredText;
  });
  useEffect(() => {
    const timer = setTimeout(() => {
      if (isSettledRef.current) {
        return;
      }
      isSettledRef.current = true;
      endCast(api, cast.castId, "unanswered").catch(() => undefined);
      toldRef.current(unansweredRef.current);
    }, castAnswerWithinMs + castAnswerGraceMs);
    return () => {
      clearTimeout(timer);
    };
  }, [api, cast.castId]);
  return <output>{t("cast.asking", { name })}</output>;
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

/** The devices to cast to as they are read: Loaded's words while loading or failed. */
function Targets(props: {
  readonly devices: Resource<Device[]>;
  readonly inFrontDeviceIds: readonly number[];
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
    <TargetList
      devices={devices.value}
      inFrontDeviceIds={props.inFrontDeviceIds}
      onCast={props.onCast}
    />
  );
}

/**
 * The devices to cast to: each a button with its name and when it was last used, or, with no Orpheum
 * tab in front, that one must be brought to the front there (D338 §2).
 */
function TargetList(props: {
  readonly devices: readonly Device[];
  readonly inFrontDeviceIds: readonly number[];
  readonly onCast: (device: Device) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  return (
    <ul className="cast-to__devices">
      {props.devices.map((device) => (
        <li key={device.id}>
          <button
            type="button"
            onClick={() => {
              props.onCast(device);
            }}
          >
            {device.name}
          </button>{" "}
          <span>
            {props.inFrontDeviceIds.includes(device.id)
              ? t("cast.connectedNow", { used: dateTimeText(device.lastUsedAt, i18n.language) })
              : t("cast.noTabInFront")}
          </span>
        </li>
      ))}
    </ul>
  );
}

/** The person's devices open now but this one, as their list orders them, most recently used first. */
export function otherDevicesOpen(
  devices: readonly Device[],
  connectedDeviceIds: readonly number[],
): Device[] {
  return devices.filter(
    (device) => !device.isThisDevice && device.isSignedIn && connectedDeviceIds.includes(device.id),
  );
}

/**
 * How long a Play page waits to hear from the server whether this device casts before it plays here,
 * in milliseconds: the live connection says so as it opens, a round trip after a page is loaded; a
 * server that has not said by then plays the film here rather than hold it up. A justified constant.
 */
export const sendingKnownWithinMs = 3000;

/**
 * A film played on a device that casts goes on to its receiver (D364 §3), from atMs or, undefined,
 * where the person left it, the device staying its remote on the casting page; one played here on
 * purpose (shouldPlayHere, the remote's "Play on this device instead", §4), or once the server says the
 * device casts nothing, or not within sendingKnownWithinMs, plays here, as children. It decides once:
 * a cast started or ended after leaves the film where it is.
 */
export function PlayOrSendOn(props: {
  readonly fileId: number;
  readonly atMs?: number | undefined;
  readonly shouldPlayHere: boolean;
  readonly children: ReactNode;
}): ReactNode {
  const { t } = useTranslation();
  const where = useWhereItPlays(props.shouldPlayHere);
  useSendsOn(where, props.fileId, props.atMs);
  if (where.kind === "here") {
    return props.children;
  }
  return where.kind === "sending" ? <p>{t("cast.sendingOn")}</p> : null;
}

/** Where a Play page's film plays: here, sent on to the cast castId's receiver, or not known yet. */
type PlaysWhere =
  | { readonly kind: "here" }
  | { readonly kind: "sending"; readonly castId: number; readonly onFailed: () => void }
  | { readonly kind: "unknown" };

/**
 * Decides once where a Play page's film plays (PlayOrSendOn): here when shouldPlayHere, when the
 * device sends no cast its receiver said Use to, or when the server has not said within
 * sendingKnownWithinMs; else sent on to that cast's receiver, here again should sending fail.
 */
function useWhereItPlays(shouldPlayHere: boolean): PlaysWhere {
  const { sending, isSendingKnown } = useLive();
  const [decided, setDecided] = useState<{ readonly castId: number } | "here" | undefined>(
    shouldPlayHere ? "here" : undefined,
  );
  const [isTimedOut, setIsTimedOut] = useState(false);
  if (decided === undefined && (isSendingKnown || isTimedOut)) {
    // Decided as it renders, once: a cast started or ended later leaves the film where it is.
    setDecided(sentTo(isSendingKnown ? sending : undefined));
  }
  const isWaiting = decided === undefined && !isSendingKnown;
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
  if (decided === undefined) {
    return { kind: "unknown" };
  }
  return decided === "here"
    ? { kind: "here" }
    : {
        kind: "sending",
        castId: decided.castId,
        onFailed: () => {
          setDecided("here");
        },
      };
}

/** Where a film played now goes: on to sending's receiver, once it said Use, else here. */
function sentTo(sending: CastSending | undefined): { readonly castId: number } | "here" {
  return sending?.isAccepted === true ? { castId: sending.castId } : "here";
}

/**
 * Sends the film on to the cast's receiver, from atMs, once where says it is sent: the casting page,
 * the remote, then shows in the Play page's place; a send that fails plays it here.
 */
function useSendsOn(where: PlaysWhere, fileId: number, atMs: number | undefined): void {
  const { api, navigation } = useServices();
  const castId = where.kind === "sending" ? where.castId : undefined;
  const sentRef = useRef<number | undefined>(undefined);
  const failedRef = useRef(where.kind === "sending" ? where.onFailed : undefined);
  useEffect(() => {
    failedRef.current = where.kind === "sending" ? where.onFailed : undefined;
  });
  useEffect(() => {
    if (castId === undefined || sentRef.current === castId) {
      return;
    }
    sentRef.current = castId;
    playInCast(api, castId, { fileId, positionMs: atMs }).then(
      () => {
        watchingOf(api).sentAway(fileId);
        navigation.replace({ screen: "casting" });
      },
      () => {
        failedRef.current?.();
      },
    );
  }, [api, navigation, castId, fileId, atMs]);
}
