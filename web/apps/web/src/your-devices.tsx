// SPDX-License-Identifier: MPL-2.0

import "./your-devices.css";

import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { removeMyDevice, renameMyDevice, signOutElsewhere, type Device } from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { fieldProblemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useRequest } from "./use-request.ts";
import type { Resource } from "./use-resource.ts";

/**
 * How many of a person's devices their account shows before "All devices (N)": the five most
 * recently used (ui-audit-2026-11-04.md §3, Your account; one account had 115).
 */
export const recentDevicesShown = 5;

/**
 * This person's devices (D218 §4, D221): each named, when added and last used, and whether signed
 * in, with a way to rename it and to remove it, which signs it out
 * at its next request; and a way to sign out everywhere but here. A device signed out signs in again
 * by a code it shows (D288). The recentDevicesShown most recently used show; "All devices (N)" opens
 * the whole list in their place.
 */
export function YourDevices(props: {
  readonly devices: Resource<Device[]>;
  readonly onChanged: () => void;
  /** Whether this session may remove devices and sign the others out (D292 §4). */
  readonly canManage: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, { fields: { "body.name": t("account.deviceName") } }),
  );
  const applyChange = (
    change: (idempotencyKey: string) => Promise<void>,
    afterwards?: () => void,
  ): void => {
    runRequest(async (idempotencyKey) => {
      await change(idempotencyKey);
      props.onChanged();
      afterwards?.();
    });
  };
  return (
    <BusyGroup isBusy={isBusy}>
      <div className="your-devices">
        <DevicesHeading devices={props.devices} />
        <Loaded
          resource={props.devices}
          render={(list) => (
            <RecentDevices
              devices={list}
              onRename={(device, name) => {
                applyChange(() => renameMyDevice(api, device.id, name));
              }}
              onRemove={
                props.canManage
                  ? (device) => {
                      // Removing this device signs it out here too.
                      applyChange(
                        () => removeMyDevice(api, device.id),
                        device.isThisDevice ? session.signOut : undefined,
                      );
                    }
                  : undefined
              }
            />
          )}
        />
        {props.canManage ? (
          <SignOutElsewhere
            onConfirm={() => {
              applyChange((idempotencyKey) => signOutElsewhere(api, idempotencyKey));
            }}
          />
        ) : null}
        {problem === undefined ? null : <p role="alert">{problem}</p>}
      </div>
    </BusyGroup>
  );
}

/**
 * The devices most recently used, the server's order (device.sql's ListDevices, last used first), and
 * where there are more, "All devices (N)": an expanding section, as the Files page's long groups are
 * (files.tsx), which opened shows every device in the recent ones' place.
 */
function RecentDevices(props: Parameters<typeof DeviceList>[0]): JSX.Element {
  const { t } = useTranslation();
  const [isAllShown, setIsAllShown] = useState(false);
  const { devices } = props;
  if (devices.length <= recentDevicesShown) {
    return <DeviceList {...props} />;
  }
  return (
    <>
      {isAllShown ? null : <DeviceList {...props} devices={devices.slice(0, recentDevicesShown)} />}
      <details
        className="your-devices__all"
        open={isAllShown}
        onToggle={(event) => {
          setIsAllShown(event.currentTarget.open);
        }}
      >
        <summary>{t("devices.all", { count: devices.length })}</summary>
        {isAllShown ? <DeviceList {...props} /> : null}
      </details>
    </>
  );
}

/** Signs out every device but this one, asked again first. */
function SignOutElsewhere(props: { readonly onConfirm: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ConfirmButton
      label={t("devices.signOutElsewhere")}
      consequence={t("devices.signOutElsewhereConsequence")}
      confirmLabel={t("devices.signOutElsewhereConfirm")}
      onConfirm={props.onConfirm}
    />
  );
}

/** Which of the three a device is: the one asking, another signed in, or signed out. */
export function deviceState(device: Device): "thisDevice" | "signedIn" | "signedOut" {
  if (device.isThisDevice) {
    return "thisDevice";
  }
  return device.isSignedIn ? "signedIn" : "signedOut";
}

/** The devices, each with its facts and what can be done to it. */
export function DeviceList(props: {
  readonly devices: readonly Device[];
  /** Absent where the names are not the viewer's to change: an admin's view of someone's. */
  readonly onRename?: ((device: Device, name: string) => void) | undefined;
  /** Absent where removing is not the viewer's to do: a session that may not change the account. */
  readonly onRemove?: ((device: Device) => void) | undefined;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  return (
    <ul className="your-devices__list">
      {props.devices.map((device) => (
        <li key={device.id} className="your-devices__device">
          {props.onRename === undefined ? (
            <span className="your-devices__name">{device.name}</span>
          ) : (
            <DeviceName device={device} onRename={props.onRename} />
          )}
          <span>{t(`devices.${deviceState(device)}`)}</span>
          {device.isByCode ? (
            <span className="your-devices__by-code">{t("devices.byCode")}</span>
          ) : null}
          <span>
            {t("devices.addedAndUsed", {
              added: dateTimeText(device.addedAt, i18n.language),
              used: dateTimeText(device.lastUsedAt, i18n.language),
            })}
          </span>
          {/* Removing signs the device out for good: red, and asked again (the owner, 2026-10-12). */}
          {props.onRemove === undefined ? null : (
            <ConfirmButton
              label={t("devices.remove")}
              labelForReader={t("devices.removeLabel", { name: device.name })}
              consequence={t("devices.removeConsequence", { name: device.name })}
              confirmLabel={t("devices.removeConfirm")}
              onConfirm={() => {
                props.onRemove?.(device);
              }}
            />
          )}
        </li>
      ))}
    </ul>
  );
}

/**
 * A device's name, which can be changed in place: Rename opens its field, focused, and the name
 * typed is saved on Enter or on leaving the field, with no Save (ui-audit-2026-11-04.md Q5:
 * every setting saves at once). Enter gives focus back to Rename; leaving it for another control
 * leaves focus there. An empty or unchanged name saves nothing.
 */
export function DeviceName(props: {
  readonly device: Device;
  readonly onRename: (device: Device, name: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const renameRef = useRef<HTMLButtonElement>(null);
  // Whether focus goes back to Rename as the field closes: after Enter, not after leaving it.
  const isRefocusingRef = useRef(false);
  useEffect(() => {
    if (!isOpen && isRefocusingRef.current) {
      isRefocusingRef.current = false;
      renameRef.current?.focus();
    }
  }, [isOpen]);
  if (isOpen) {
    return (
      <NameField
        name={props.device.name}
        onClose={(name, isByEnter) => {
          isRefocusingRef.current = isByEnter;
          if (name !== "" && name !== props.device.name) {
            props.onRename(props.device, name);
          }
          setIsOpen(false);
        }}
      />
    );
  }
  return (
    <span className="your-devices__name">
      {props.device.name}{" "}
      <button
        ref={renameRef}
        type="button"
        onClick={() => {
          setIsOpen(true);
        }}
      >
        {t("devices.rename")}
      </button>
    </span>
  );
}

/**
 * A device's name being typed, focused as it opens, closed by Enter or by leaving it: onClose has
 * what was typed and which closed it, once, though Enter's closing may blur it after.
 */
function NameField(props: {
  readonly name: string;
  readonly onClose: (name: string, isByEnter: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(props.name);
  const fieldRef = useRef<HTMLInputElement>(null);
  const isClosedRef = useRef(false);
  useEffect(() => {
    fieldRef.current?.focus();
  }, []);
  const close = (isByEnter: boolean): void => {
    if (!isClosedRef.current) {
      isClosedRef.current = true;
      props.onClose(draft, isByEnter);
    }
  };
  return (
    <form
      className="your-devices__name"
      onSubmit={(event) => {
        // Not submitWith: its action, made in render, may not touch isClosedRef (react-hooks/refs).
        event.preventDefault();
        close(true);
      }}
    >
      <input
        ref={fieldRef}
        required
        maxLength={100}
        aria-label={t("account.deviceName")}
        value={draft}
        onChange={(event) => {
          setDraft(event.target.value);
        }}
        onBlur={() => {
          close(false);
        }}
      />
    </form>
  );
}

/**
 * The section's heading and, while a device a code activated is among the person's others, a word
 * that such a device watches and hosts as them, and to remove one they don't know (the device code
 * audit's M4).
 */
function DevicesHeading(props: { readonly devices: Resource<Device[]> }): JSX.Element {
  const { t } = useTranslation();
  const { devices } = props;
  const hasByCode =
    devices.state === "loaded" &&
    devices.value.some((device) => device.isByCode && !device.isThisDevice);
  return (
    <>
      <h2>{t("devices.heading")}</h2>
      {hasByCode ? (
        <p className="your-devices__by-code-notice" role="note">
          {t("devices.byCodeNotice")}
        </p>
      ) : null}
    </>
  );
}
