// SPDX-License-Identifier: MPL-2.0

import "./your-devices.css";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  removeMyDevice,
  renameMyDevice,
  setMyDeviceAudioOutput,
  signOutElsewhere,
  type AudioOutput,
  type Device,
} from "./accounts.ts";
import { camelKey } from "./catalogue-key.ts";
import { dateTimeText } from "./date-time-text.ts";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { audioOutputs } from "./sound.tsx";
import { Loaded } from "./title.tsx";
import type { Resource } from "./use-resource.ts";

/**
 * This person's devices (D218 §4, D221): each named, when added and last used, and whether signed
 * in, with a way to rename it, to say how it makes sound (D222), to remove it, which signs it out at its next request, and, when it
 * is signed out, to make a code that signs it back in; and a way to sign out everywhere but here.
 */
export function YourDevices(props: {
  readonly devices: Resource<Device[]>;
  readonly onChanged: () => void;
  readonly onMakeCode: (device: Device) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const applyChange = (request: Promise<void>, afterwards?: () => void): void => {
    request.then(
      () => {
        setProblem(undefined);
        props.onChanged();
        afterwards?.();
      },
      (cause: unknown) => {
        setProblem(problemText(t, cause));
      },
    );
  };
  return (
    <div className="your-devices">
      <h2>{t("devices.heading")}</h2>
      <Loaded
        resource={props.devices}
        render={(list) => (
          <DeviceList
            devices={list}
            onRename={(device, name) => {
              applyChange(renameMyDevice(api, device.id, name));
            }}
            onSoundChange={(device, output) => {
              applyChange(setMyDeviceAudioOutput(api, device.id, output));
            }}
            onRemove={(device) => {
              // Removing this device signs it out here too.
              applyChange(
                removeMyDevice(api, device.id),
                device.isThisDevice ? session.signOut : undefined,
              );
            }}
            onMakeCode={props.onMakeCode}
          />
        )}
      />
      <button
        type="button"
        onClick={() => {
          applyChange(signOutElsewhere(api));
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
  /** Absent where the names are not the viewer's to change: an admin's view of someone's. */
  readonly onRename?: ((device: Device, name: string) => void) | undefined;
  /** Absent where how the devices make sound is not the viewer's to change. */
  readonly onSoundChange?: ((device: Device, output: AudioOutput) => void) | undefined;
  readonly onRemove: (device: Device) => void;
  readonly onMakeCode: (device: Device) => void;
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
          <DeviceSound device={device} onSoundChange={props.onSoundChange} />
          <span>
            {t("devices.addedAndUsed", {
              added: dateTimeText(device.addedAt, i18n.language),
              used: dateTimeText(device.lastUsedAt, i18n.language),
            })}
          </span>
          {device.isSignedIn ? null : (
            <button
              type="button"
              onClick={() => {
                props.onMakeCode(device);
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

/** How a device makes sound: chosen here when it is the viewer's to change, else only said. */
function DeviceSound(props: {
  readonly device: Device;
  readonly onSoundChange: ((device: Device, output: AudioOutput) => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { device, onSoundChange } = props;
  if (onSoundChange === undefined) {
    return device.audioOutput === undefined ? null : (
      <span>{t("sound.current", { output: t(`sound.${camelKey(device.audioOutput)}`) })}</span>
    );
  }
  return (
    <label>
      {t("sound.label")}{" "}
      <select
        value={device.audioOutput ?? ""}
        onChange={(event) => {
          const chosen = audioOutputs.find((output) => output === event.target.value);
          if (chosen !== undefined) {
            onSoundChange(device, chosen);
          }
        }}
      >
        {device.audioOutput === undefined ? (
          <option value="" disabled>
            {t("sound.notAsked")}
          </option>
        ) : null}
        {audioOutputs.map((output) => (
          <option key={output} value={output}>
            {t(`sound.${camelKey(output)}`)}
          </option>
        ))}
      </select>
    </label>
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
