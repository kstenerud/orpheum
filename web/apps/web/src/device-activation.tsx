// SPDX-License-Identifier: MPL-2.0

import "./device-activation.css";

import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { activateDevice, answerActivation, type Activation } from "./accounts.ts";
import { arrivalMark } from "./focus-navigation.ts";
import { FormRow } from "./form-row.tsx";
import { useFieldMarks, type FieldMarks } from "./field-marks.ts";
import { WarningIcon } from "./icons.tsx";
import { failedFields, fieldProblemText, type FormWords } from "./problem.ts";
import type { Scanned } from "./router.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

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
  /**
   * A device's QR code the phone scanned (D321): entered at once, with its key's hash, and answered
   * by one tap, there being no check code; onScannedAnswered is told once it was.
   */
  readonly scanned?: Scanned;
  readonly onScannedAnswered?: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [step, setStep] = useState<Step>({ kind: "entering" });
  const { isBusy, problem, failure, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, activateWords(t)),
  );
  const marks = useFieldMarks(failure, failedFields(failure, activateWords(t)));
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
  useScannedEntry(props.scanned, runRequest, setStep);
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
      if (step.activation.checkCode === "") {
        props.onScannedAnswered?.();
      }
    });
  };
  return (
    <div className="device-activation">
      <h2>{t("activate.heading")}</h2>
      {step.kind === "checking" ? (
        <Checking activation={step.activation} isBusy={isBusy} onAnswer={answer} />
      ) : (
        <>
          {step.kind === "done" ? <Done step={step} /> : null}
          <EnterCode isBusy={isBusy} onEnter={enter} marks={marks} />
        </>
      )}
      {problem === undefined ? null : (
        <p role="alert" id={marks.problemId}>
          {problem}
        </p>
      )}
    </div>
  );
}

/**
 * How a refused activation is told: its fields, and a code no device shows (notFound: wrong, or
 * expired) about the code's.
 */
function activateWords(t: TFunction): FormWords {
  return {
    fields: { "body.code": t("activate.code"), "body.deviceName": t("activate.deviceName") },
    overrides: { notFound: t("activate.noSuchCode"), rateLimited: t("activate.tooManyCodes") },
    codeFields: { notFound: ["body.code"] },
  };
}

/** What came of the last code entered. */
function Done(props: { readonly step: Extract<Step, { kind: "done" }> }): JSX.Element {
  const { t } = useTranslation();
  const { step } = props;
  return (
    <output>
      {step.isActivated
        ? t("activate.activated", { device: step.deviceName })
        : t("activate.notActivated")}
    </output>
  );
}

/**
 * The code a device shows and the name to list it by, and the button that enters them, the three in
 * one row under what to do (ui-audit-2026-11-04.md §3, Your account), so Right goes from the code to
 * the name to Activate.
 */
function EnterCode(props: {
  readonly isBusy: boolean;
  readonly onEnter: (code: string, deviceName: string) => void;
  /** The marks on the fields the failure shown is about. */
  readonly marks: FieldMarks;
}): JSX.Element {
  const { t } = useTranslation();
  const { navigation, codeAddress } = useServices();
  const { marks } = props;
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  return (
    <>
      <p>{t("activate.explain", { origin: codeAddress ?? navigation.origin() })}</p>
      <p className="device-activation__entry-warning" role="note">
        {t("activate.entryWarning")}
      </p>
      <FormRow
        onSubmit={() => {
          props.onEnter(code, name);
        }}
      >
        <label>
          {t("activate.code")}{" "}
          <input
            className="device-activation__code-field"
            {...arrivalMark}
            required
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            maxLength={12}
            placeholder={t("activate.codeExample")}
            value={code}
            {...marks.markOf("body.code")}
            onChange={(event) => {
              marks.editing("body.code", setCode)(event.target.value);
            }}
          />
        </label>
        <label>
          {t("activate.deviceName")}{" "}
          <input
            maxLength={100}
            placeholder={t("activate.deviceNameExample")}
            value={name}
            {...marks.markOf("body.deviceName")}
            onChange={(event) => {
              marks.editing("body.deviceName", setName)(event.target.value);
            }}
          />
        </label>
        <button type="submit" disabled={props.isBusy}>
          {t("activate.submit")}
        </button>
      </FormRow>
    </>
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
      <AskedBy activation={props.activation} />
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

/**
 * Enters a scanned device's code once, as the page opens, with its key's hash (D321), going on to the
 * question it asks.
 */
function useScannedEntry(
  scanned: Scanned | undefined,
  runRequest: (request: (idempotencyKey: string) => Promise<void>) => void,
  setStep: (step: Step) => void,
): void {
  const { api } = useServices();
  const isEnteredRef = useRef(false);
  useEffect(() => {
    if (scanned === undefined || isEnteredRef.current) {
      return;
    }
    isEnteredRef.current = true;
    runRequest(async (idempotencyKey) => {
      const activation = await activateDevice(
        api,
        { code: scanned.code, deviceName: "", keyHash: scanned.keyHash },
        idempotencyKey,
      );
      setStep({ kind: "checking", code: scanned.code, activation });
    });
  }, [api, runRequest, scanned, setStep]);
}

/** What a code entered asks: one question for a scanned one, with no check code, else the check. */
function Checking(props: {
  readonly activation: Activation;
  readonly isBusy: boolean;
  readonly onAnswer: (isMatch: boolean) => void;
}): JSX.Element {
  return props.activation.checkCode === "" ? (
    <ScannedConfirm {...props} />
  ) : (
    <CheckCode {...props} />
  );
}

/**
 * A scanned device's one question, with no check code to compare (D321): what it is and how long ago
 * it asked, the warning that only a device in front of one is to be activated, and Activate.
 */
function ScannedConfirm(props: {
  readonly activation: Activation;
  readonly isBusy: boolean;
  readonly onAnswer: (isActivating: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="device-activation__check">
      <p>{t("activate.scannedQuestion", { device: props.activation.deviceName })}</p>
      <AskedBy activation={props.activation} />
      <div className="device-activation__warning" role="note">
        <WarningIcon />
        <p>{t("activate.scannedWarning")}</p>
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
          {t("activate.scannedActivate")}
        </button>
        <button
          type="button"
          disabled={props.isBusy}
          onClick={() => {
            props.onAnswer(false);
          }}
        >
          {t("activate.scannedCancel")}
        </button>
      </div>
    </div>
  );
}

/** How long ago seconds is, as the person reads it. */
function ageText(t: TFunction, seconds: number): string {
  if (seconds < 10) {
    return t("activate.justNow");
  }
  if (seconds < 60) {
    return t("activate.secondsAgo", { count: seconds });
  }
  return t("activate.minutesAgo", { count: Math.floor(seconds / 60) });
}

/** How long ago a code asked for long since counts as a while: two minutes. */
const longAgoSeconds = 120;

/**
 * What asked for the code, and how long ago: a code asked for a while before is more likely one
 * someone sent the person than one in front of them (the device code audit's M3).
 */
function AskedBy(props: { readonly activation: Activation }): JSX.Element {
  const { t } = useTranslation();
  const seconds = props.activation.askedSecondsAgo;
  const age = ageText(t, seconds);
  return (
    <>
      <p>{t("activate.asked", { platform: props.activation.platform, age })}</p>
      {seconds >= longAgoSeconds ? (
        <p className="device-activation__long-ago" role="alert">
          {t("activate.askedLongAgo")}
        </p>
      ) : null}
    </>
  );
}
