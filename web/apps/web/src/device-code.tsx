// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX, type ReactNode } from "react";
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
import { storageIfAllowed, storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";
import { FormRow } from "./form-row.tsx";
import { fieldProblemText, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { useResource } from "./use-resource.ts";

/** How often a page showing a code asks whether it has been used. */
const codeCheckEveryMs = 3000;

/** This tab's own storage, which keeps nothing where the browser refuses it. */
function tabStorage(): StringStorage {
  return storageThatNeverThrows(storageIfAllowed(() => window.sessionStorage));
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
 * waiting, used, replaced or expired, when onUsed is told so the device list can show it. While it
 * waits, a failure to ask about it is shown, and the asking goes on.
 */
export function useDeviceCode(onUsed: () => void): DeviceCodeState {
  const { t } = useTranslation();
  const { api } = useServices();
  const [code, setCode] = useState<ShownCode | undefined>(() =>
    readShownCode(tabStorage(), Date.now()),
  );
  const [issueProblem, setIssueProblem] = useState<string | undefined>(undefined);
  const secret = code?.secret;
  const [codeCheck] = useResource(
    `device-code/${secret ?? ""}`,
    async () => {
      if (secret === undefined) {
        return false;
      }
      const isWaiting = await isCodeWaiting(api);
      if (!isWaiting) {
        forgetShownCode(tabStorage());
        setCode(undefined);
        onUsed();
      }
      return isWaiting;
    },
    secret === undefined ? undefined : codeCheckEveryMs,
  );
  const checkProblem = codeCheck.state === "failed" ? problemText(t, codeCheck.cause) : undefined;
  const makeCode = (target: CodeTarget, deviceName: string): void => {
    setIssueProblem(undefined);
    issueDeviceCode(api, target).then(
      (issued) => {
        const shown = { ...issued, deviceName };
        setCode(shown);
        keepShownCode(tabStorage(), shown);
      },
      (cause: unknown) => {
        setIssueProblem(
          fieldProblemText(t, cause, { fields: { "body.deviceName": t("account.deviceName") } }),
        );
      },
    );
  };
  return { code, problem: issueProblem ?? checkProblem, makeCode };
}

/** A device code on screen: the code, for which device, and how to use it. */
export function ShownDeviceCode(props: { readonly code: ShownCode }): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  return (
    <div className="device-code">
      <h2>{t("account.codeFor", { device: props.code.deviceName })}</h2>
      <p>{t("account.useCode", { origin: navigation.origin() })}</p>
      <Share kind="code" text={props.code.secret} invitation={props.code} />
    </div>
  );
}

/**
 * A new device's name and the button that makes its code (D218 §3): on the person's own account
 * and on their page for the admin, one form. Anything the form says first, such as a heading, is
 * its children; what went wrong comes after the button.
 */
export function NewDeviceCodeForm(props: {
  readonly onMakeCode: (deviceName: string) => void;
  readonly problem?: string | undefined;
  readonly children?: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  return (
    <FormRow
      onSubmit={() => {
        props.onMakeCode(name);
      }}
    >
      {props.children}
      <label>
        {t("account.deviceName")}{" "}
        <input
          required
          maxLength={100}
          placeholder={t("account.deviceNameExample")}
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </label>
      <button type="submit">{t("account.makeCode")}</button>
      {props.problem === undefined ? null : <p role="alert">{props.problem}</p>}
    </FormRow>
  );
}
