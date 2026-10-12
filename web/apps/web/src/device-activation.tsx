// SPDX-License-Identifier: MPL-2.0

import "./device-activation.css";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { activateDevice, answerActivation, type Activation } from "./accounts.ts";
import { FormRow } from "./form-row.tsx";
import { fieldProblemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { WarningIcon } from "./warning-sign.tsx";

/** Where an activation stands on this device: being entered, being checked, or done. */
type Step =
  | { readonly kind: "entering" }
  | { readonly kind: "checking"; readonly code: string; readonly activation: Activation }
  | { readonly kind: "done"; readonly isActivated: boolean; readonly deviceName: string };

/**
 * Activates a device by the code it shows (D288): the code and the name to list it by, entered here;
 * then the check code, which the person compares with their own eyes to the one the device shows,
 * under a plain warning never to take someone's word for it; only their "Yes, they match" signs the
 * device in. For oneself on one's account, or, with userId, by an admin for that person.
 */
export function ActivateDevice(props: {
  readonly userId?: number;
  /** Told once a device is activated, so the list can show it. */
  readonly onActivated: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [step, setStep] = useState<Step>({ kind: "entering" });
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: { "body.code": t("activate.code"), "body.deviceName": t("activate.deviceName") },
      overrides: { notFound: t("activate.noSuchCode"), rateLimited: t("activate.tooManyCodes") },
    }),
  );
  const enter = (code: string, deviceName: string): void => {
    runRequest(async (idempotencyKey) => {
      const activation = await activateDevice(
        api,
        { code, deviceName, ...(props.userId === undefined ? {} : { userId: props.userId }) },
        idempotencyKey,
      );
      setStep({ kind: "checking", code, activation });
    });
  };
  const answer = (isMatch: boolean): void => {
    if (step.kind !== "checking") {
      return;
    }
    runRequest(async (idempotencyKey) => {
      await answerActivation(api, { code: step.code, isMatch }, idempotencyKey);
      setStep({ kind: "done", isActivated: isMatch, deviceName: step.activation.deviceName });
      if (isMatch) {
        props.onActivated();
      }
    });
  };
  return (
    <div className="device-activation">
      <h2>{t("activate.heading")}</h2>
      {step.kind === "checking" ? (
        <CheckCode activation={step.activation} isBusy={isBusy} onAnswer={answer} />
      ) : (
        <>
          {step.kind === "done" ? (
            <output>
              {step.isActivated
                ? t("activate.activated", { device: step.deviceName })
                : t("activate.notActivated")}
            </output>
          ) : null}
          <EnterCode isBusy={isBusy} onEnter={enter} />
        </>
      )}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </div>
  );
}

/** The code a device shows and the name to list it by, and the button that enters them. */
function EnterCode(props: {
  readonly isBusy: boolean;
  readonly onEnter: (code: string, deviceName: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  return (
    <FormRow
      onSubmit={() => {
        props.onEnter(code, name);
      }}
    >
      <p>{t("activate.explain", { origin: navigation.origin() })}</p>
      <label>
        {t("activate.code")}{" "}
        <input
          className="device-activation__code-field"
          required
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          maxLength={12}
          placeholder={t("activate.codeExample")}
          value={code}
          onChange={(event) => {
            setCode(event.target.value);
          }}
        />
      </label>
      <label>
        {t("activate.deviceName")}{" "}
        <input
          maxLength={100}
          placeholder={t("activate.deviceNameExample")}
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </label>
      <button type="submit" disabled={props.isBusy}>
        {t("activate.submit")}
      </button>
    </FormRow>
  );
}

/**
 * The check code, and the warning to compare it with one's own eyes, never on someone's word (the
 * owner, 2026-10-12: "a big, easy to understand warning that they need to verify visually, not take
 * some stranger's word for it").
 */
function CheckCode(props: {
  readonly activation: Activation;
  readonly isBusy: boolean;
  readonly onAnswer: (isMatch: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="device-activation__check">
      <p>{t("activate.checkExplain", { device: props.activation.deviceName })}</p>
      <output className="device-activation__check-code" aria-label={t("activate.checkCode")}>
        {props.activation.checkCode}
      </output>
      <div className="device-activation__warning" role="note">
        <WarningIcon />
        <div>
          <p className="device-activation__warning-heading">{t("activate.warningHeading")}</p>
          <p>{t("activate.warning")}</p>
        </div>
      </div>
      <div className="device-activation__answers">
        <button
          type="button"
          className="device-activation__match"
          disabled={props.isBusy}
          onClick={() => {
            props.onAnswer(true);
          }}
        >
          {t("activate.match")}
        </button>
        <button
          type="button"
          disabled={props.isBusy}
          onClick={() => {
            props.onAnswer(false);
          }}
        >
          {t("activate.noMatch")}
        </button>
      </div>
    </div>
  );
}
