// SPDX-License-Identifier: MPL-2.0

import "./your-devices.css";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { removeMyDevice, renameMyDevice, signOutElsewhere, type Device } from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { submitWith } from "./form-row.tsx";
import { fieldProblemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useRequest } from "./use-request.ts";
import type { Resource } from "./use-resource.ts";

/**
 * This person's devices (D218 §4, D221): each named, when added and last used, and whether signed
 * in, with a way to rename it and to remove it, which signs it out
 * at its next request; and a way to sign out everywhere but here. A device signed out signs in again
 * by a code it shows (D288).
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
            <DeviceList
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

/** Which of the three a device is: the one asking, another signed in, or signed out. */
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

/** A device's name, which can be changed in place. */
export function DeviceName(props: {
  readonly device: Device;
  readonly onRename: (device: Device, name: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<string | undefined>(undefined);
  if (draft === undefined) {
    return (
      <span className="your-devices__name">
        {props.device.name}{" "}
        <button
          type="button"
          onClick={() => {
            setDraft(props.device.name);
          }}
        >
          {t("devices.rename")}
        </button>
      </span>
    );
  }
  return (
    <form
      className="your-devices__name"
      onSubmit={submitWith(() => {
        props.onRename(props.device, draft);
        setDraft(undefined);
      })}
    >
      <input
        required
        maxLength={100}
        aria-label={t("account.deviceName")}
        value={draft}
        onChange={(event) => {
          setDraft(event.target.value);
        }}
      />
      <button type="submit">{t("devices.save")}</button>
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
