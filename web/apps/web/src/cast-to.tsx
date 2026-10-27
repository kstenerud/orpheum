// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { TogetherEndReason } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useEffect, useRef, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { listMyDevices, type Device } from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import {
  castAnswerGraceMs,
  castAnswerWithinMs,
  endCast,
  isNotedSending,
  listMyCasts,
  noteSending,
  startCast,
  type Cast,
} from "./cast.ts";
import { dateTimeText } from "./date-time-text.ts";
import { useLive } from "./live.tsx";
import { problemText } from "./problem.ts";
import { useServices, useToken } from "./services.tsx";
import { tabStorage } from "./browser-storage.ts";
import { useMembership } from "./together-membership.ts";
import { useRequest } from "./use-request.ts";
import { useResource, type Resource } from "./use-resource.ts";
import { useTogether } from "./use-together.ts";

/**
 * Cast to another device (D337 §2; plans/cast-to-device.md §1): opens the list of the person's other
 * devices open now, each by its name and when it was last used; one chosen is asked whether to play
 * the film from positionMs() (undefined: where the person left it), and once it does this device
 * becomes its remote (D358). Not
 * offered while this tab hosts a Watch Together session, whose casting waits (§7, §8 step 8).
 */
export function CastToDevice(props: {
  readonly fileId: number;
  readonly positionMs: () => number | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  if (isHosting) {
    return null;
  }
  if (!isOpen) {
    return (
      <button
        type="button"
        className="cast-to__open"
        onClick={() => {
          setIsOpen(true);
        }}
      >
        {t("cast.open")}
      </button>
    );
  }
  return (
    <section className="cast-to" aria-label={t("cast.heading")}>
      <h2>{t("cast.heading")}</h2>
      <CastTargets fileId={props.fileId} positionMs={props.positionMs} />
      <button
        type="button"
        onClick={() => {
          setIsOpen(false);
        }}
      >
        {t("cast.close")}
      </button>
    </section>
  );
}

/**
 * The person's other devices open now, read again as that set changes, each a button to cast to; a
 * device with no Orpheum tab in front says so, and a cast to it is refused with what to do (D338 §2).
 */
function CastTargets(props: {
  readonly fileId: number;
  readonly positionMs: () => number | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const { connectedDeviceIds, inFrontDeviceIds } = useLive();
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
      const cast = await startCast(api, {
        toDeviceId: device.id,
        fileId: props.fileId,
        ...positionOf(props.positionMs()),
      });
      setAsking({ cast, name: device.name });
    });
  };
  if (asking !== undefined) {
    return (
      <AwaitingAnswer
        key={asking.cast.castId}
        asking={asking}
        fileId={props.fileId}
        onPlayed={() => {
          noteSending(tabStorage(), true);
          navigation.go({ screen: "casting" });
        }}
        onNotPlayed={(said) => {
          setAsking(undefined);
          setAnswered(said);
        }}
      />
    );
  }
  return (
    <BusyGroup isBusy={isBusy}>
      <Targets devices={devices} inFrontDeviceIds={inFrontDeviceIds} onCast={castTo} />
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      {answered === undefined ? null : <output>{answered}</output>}
    </BusyGroup>
  );
}

/** A cast started, its receiver asked whether to play it (D358), and the name it is asked on. */
interface Asking {
  readonly cast: Cast;
  readonly name: string;
}

/**
 * The sender waiting for its receiver's answer (D358 §1–§4), still playing its own film: the
 * receiver's Play puts the film on in the cast's session, and only then does the sender hand its
 * playback off, becoming the remote (onPlayed); Not now, or no answer, ends the cast, said in a line
 * (onNotPlayed), the film playing on here. A receiver that never answers, its page gone, is given up
 * on after castAnswerWithinMs and castAnswerGraceMs.
 */
function AwaitingAnswer(props: {
  readonly asking: Asking;
  readonly fileId: number;
  readonly onPlayed: () => void;
  readonly onNotPlayed: (said: string) => void;
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
  const isPlayed = snapshot?.group?.fileId === props.fileId;
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  const toldRef = useRef(props);
  const isSettledRef = useRef(false);
  useEffect(() => {
    toldRef.current = props;
  });
  useEffect(() => {
    if (isSettledRef.current) {
      return;
    }
    if (isPlayed) {
      isSettledRef.current = true;
      toldRef.current.onPlayed();
    } else if (endReason !== undefined) {
      isSettledRef.current = true;
      toldRef.current.onNotPlayed(notPlayedText(t, endReason, name));
    }
  }, [isPlayed, endReason, name, t]);
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
      toldRef.current.onNotPlayed(unansweredRef.current);
    }, castAnswerWithinMs + castAnswerGraceMs);
    return () => {
      clearTimeout(timer);
    };
  }, [api, cast.castId]);
  return <output>{t("cast.asking", { name })}</output>;
}

/** What a sender says of a cast that ended before its receiver played it. */
function notPlayedText(t: TFunction, reason: TogetherEndReason, name: string): string {
  if (reason === "declined") {
    return t("cast.answered.declined", { name });
  }
  return reason === "unanswered"
    ? t("cast.answered.unanswered", { name })
    : t("cast.answered.ended", { name });
}

/**
 * The devices to cast to as they are read: Loaded's words while loading or failed, kept apart from
 * title.tsx, whose page offers casting too.
 */
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

/** A position to cast from, if any: the cast's positionMs. */
function positionOf(positionMs: number | undefined): { readonly positionMs?: number } {
  return positionMs === undefined ? {} : { positionMs };
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
 * A film played on a device that casts goes on to its receiver (plans/cast-to-device.md §9 Q3), the
 * device staying its remote; once the server says it casts no more, or never while the tab has not
 * cast, the film plays here, as children.
 */
export function PlayOrSendOn(props: {
  readonly fileId: number;
  readonly children: ReactNode;
}): ReactNode {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const [isChecking, setIsChecking] = useState(() => isNotedSending(tabStorage()));
  const { fileId } = props;
  useEffect(() => {
    if (!isChecking) {
      return undefined;
    }
    let isCurrent = true;
    const playHere = (): void => {
      noteSending(tabStorage(), false);
      if (isCurrent) {
        setIsChecking(false);
      }
    };
    listMyCasts(api)
      .then(async (casts) => {
        const sent = casts.find((each) => each.isSender);
        if (sent === undefined) {
          playHere();
          return;
        }
        await startCast(api, { toDeviceId: sent.receiverDeviceId, fileId });
        if (isCurrent) {
          navigation.replace({ screen: "casting" });
        }
      })
      .catch(playHere);
    return () => {
      isCurrent = false;
    };
  }, [api, navigation, fileId, isChecking]);
  return isChecking ? <p>{t("cast.sendingOn")}</p> : props.children;
}
