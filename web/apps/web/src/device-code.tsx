// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  forgetShownCode,
  isCodeWaiting,
  issueDeviceCode,
  keepShownCode,
  readShownCode,
  type CodeTarget,
  type ShownCode,
} from "./accounts.ts";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";

/** How often a page showing a code asks whether it has been used. */
const codeCheckEveryMs = 3000;

/** This tab's own storage, or undefined where the browser refuses it. */
function tabStorage(): Storage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}

/** The device code on screen, and how to make one: what useDeviceCode returns. */
export interface DeviceCodeState {
  readonly code: ShownCode | undefined;
  readonly problem: string | undefined;
  readonly makeCode: (target: CodeTarget, deviceName: string) => void;
}

/**
 * The person's device code (D218 §3, D221): made for a new device or a listed one, kept across a
 * reload in this tab's storage, and taken off the screen once the server says it is no longer
 * waiting, used, replaced or expired, when onUsed is told so the device list can show it.
 */
export function useDeviceCode(onUsed: () => void): DeviceCodeState {
  const { t } = useTranslation();
  const { api } = useServices();
  const [code, setCode] = useState<ShownCode | undefined>(() => {
    const storage = tabStorage();
    return storage === undefined ? undefined : readShownCode(storage, Date.now());
  });
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const secret = code?.secret;
  useEffect(() => {
    if (secret === undefined) {
      return undefined;
    }
    const pollTimer = setInterval(() => {
      isCodeWaiting(api).then(
        (isWaiting) => {
          if (!isWaiting) {
            const storage = tabStorage();
            if (storage !== undefined) {
              forgetShownCode(storage);
            }
            setCode(undefined);
            onUsed();
          }
        },
        () => undefined, // asked again at the next tick
      );
    }, codeCheckEveryMs);
    return () => {
      clearInterval(pollTimer);
    };
  }, [api, secret, onUsed]);
  const makeCode = (target: CodeTarget, deviceName: string): void => {
    setProblem(undefined);
    issueDeviceCode(api, target).then(
      (issued) => {
        const shown = { ...issued, deviceName };
        setCode(shown);
        const storage = tabStorage();
        if (storage !== undefined) {
          keepShownCode(storage, shown);
        }
      },
      (cause: unknown) => {
        setProblem(problemText(t, cause));
      },
    );
  };
  return { code, problem, makeCode };
}

/** A device code on screen: the code, for which device, and how to use it. */
export function ShownDeviceCode(props: { readonly code: ShownCode }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="device-code">
      <h2>{t("account.codeFor", { device: props.code.deviceName })}</h2>
      <p>{t("account.useCode", { origin: window.location.origin })}</p>
      <Share kind="code" text={props.code.secret} invitation={props.code} />
    </div>
  );
}
