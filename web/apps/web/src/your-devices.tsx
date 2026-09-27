// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { removeMyDevice, renameMyDevice, signOutElsewhere, type Device } from "./accounts.ts";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import type { Resource } from "./use-resource.ts";

/**
 * This person's devices (D218 §4, D221): each named, when added and last used, and whether signed
 * in, with a way to rename it, to remove it, which signs it out at its next request, and, when it
 * is signed out, to make a code that signs it back in; and a way to sign out everywhere but here.
 */
export function YourDevices(props: {
  readonly devices: Resource<Device[]>;
  readonly reload: () => void;
  readonly onCodeFor: (device: Device) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const then = (change: Promise<void>, afterwards?: () => void): void => {
    change.then(
      () => {
        setProblem(undefined);
        props.reload();
        afterwards?.();
      },
      (cause: unknown) => {
        setProblem(problemText(t, cause));
      },
    );
  };
  return (
    <div className="account__devices">
      <h2>{t("devices.heading")}</h2>
      <Loaded
        resource={props.devices}
        render={(list) => (
          <DeviceList
            devices={list}
            onRename={(device, name) => {
              then(renameMyDevice(api, device.id, name));
            }}
            onRemove={(device) => {
              // Removing this device signs it out here too.
              then(
                removeMyDevice(api, device.id),
                device.isThisDevice ? session.signOut : undefined,
              );
            }}
            onCodeFor={props.onCodeFor}
          />
        )}
      />
      <button
        type="button"
        onClick={() => {
          then(signOutElsewhere(api));
        }}
      >
        {t("devices.signOutElsewhere")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </div>
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
  readonly onRename: (device: Device, name: string) => void;
  readonly onRemove: (device: Device) => void;
  readonly onCodeFor: (device: Device) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const when = new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" });
  return (
    <ul className="account__device-list">
      {props.devices.map((device) => (
        <li key={device.id} className="account__device">
          <DeviceName device={device} onRename={props.onRename} />
          <span>{t(`devices.${deviceState(device)}`)}</span>
          <span>
            {t("devices.addedAndUsed", {
              added: when.format(new Date(device.addedAt)),
              used: when.format(new Date(device.lastUsedAt)),
            })}
          </span>
          {device.isSignedIn ? null : (
            <button
              type="button"
              onClick={() => {
                props.onCodeFor(device);
              }}
            >
              {t("devices.codeToSignBackIn")}
            </button>
          )}
          <button
            type="button"
            onClick={() => {
              props.onRemove(device);
            }}
            aria-label={t("devices.removeLabel", { name: device.name })}
          >
            {t("devices.remove")}
          </button>
        </li>
      ))}
    </ul>
  );
}

/** A device's name, which can be changed in place. */
function DeviceName(props: {
  readonly device: Device;
  readonly onRename: (device: Device, name: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<string | undefined>(undefined);
  if (draft === undefined) {
    return (
      <span className="account__device-name">
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
      className="account__device-name"
      onSubmit={(event) => {
        event.preventDefault();
        props.onRename(props.device, draft);
        setDraft(undefined);
      }}
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
