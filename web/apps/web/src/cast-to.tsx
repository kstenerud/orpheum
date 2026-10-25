// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import { useEffect, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { listMyDevices, type Device } from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import { isNotedSending, listMyCasts, noteSending, startCast } from "./cast.ts";
import { dateTimeText } from "./date-time-text.ts";
import { useLive } from "./live.tsx";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { tabStorage } from "./browser-storage.ts";
import { useMembership } from "./together-membership.ts";
import { useRequest } from "./use-request.ts";
import { useResource, type Resource } from "./use-resource.ts";

/**
 * Cast to another device (D337 §2; plans/cast-to-device.md §1): opens the list of the person's other
 * devices open now, each by its name and when it was last used; one chosen plays the film from
 * positionMs() (undefined: where the person left it) and this device becomes its remote. Not
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

/** The person's other devices open now, read again as that set changes, each a button to cast to. */
function CastTargets(props: {
  readonly fileId: number;
  readonly positionMs: () => number | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const { connectedDeviceIds } = useLive();
  const [devices] = useResource(`cast-to/${connectedDeviceIds.join(",")}`, async () =>
    otherDevicesOpen(await listMyDevices(api), connectedDeviceIds),
  );
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const castTo = (device: Device): void => {
    runRequest(async () => {
      await startCast(api, {
        toDeviceId: device.id,
        fileId: props.fileId,
        ...positionOf(props.positionMs()),
      });
      noteSending(tabStorage(), true);
      navigation.go({ screen: "casting" });
    });
  };
  return (
    <BusyGroup isBusy={isBusy}>
      <Targets devices={devices} onCast={castTo} />
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </BusyGroup>
  );
}

/**
 * The devices to cast to as they are read: Loaded's words while loading or failed, kept apart from
 * title.tsx, whose page offers casting too.
 */
function Targets(props: {
  readonly devices: Resource<Device[]>;
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
    <TargetList devices={devices.value} onCast={props.onCast} />
  );
}

/** The devices to cast to: each a button with its name and when it was last used. */
function TargetList(props: {
  readonly devices: readonly Device[];
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
            {t("cast.connectedNow", { used: dateTimeText(device.lastUsedAt, i18n.language) })}
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
