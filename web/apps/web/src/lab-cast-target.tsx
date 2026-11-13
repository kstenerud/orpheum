// SPDX-License-Identifier: MPL-2.0

import "./lab-words.ts";

import { failureText } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { activateDevice, answerActivation } from "./accounts.ts";
import { at } from "./cast-sdk.ts";
import { newIdempotencyKey } from "./idempotent.ts";
import type { CastSession } from "./lab-cast-sdk.ts";
import { useServices } from "./services.tsx";

/**
 * The lab's step that makes our Cast receiver a cast target (inventory B-10; the owner, 2026-11-07):
 * asked, the receiver pairs as a device of this page's person (D288), its code told back over the
 * lab's namespace and entered here under the page's session, the check codes compared by the person,
 * after which the Chromecast is listed in the Cast control as any device connected now (D337) and
 * plays a cast handed to it in the cast's session (D434; web/apps/cast-receiver/src/device.ts).
 */

/** The receiver's pairing code, where its message tells one. */
export function pairingCodeOf(message: unknown): string | undefined {
  const code = at(message, "code");
  return at(message, "type") === "pairing" && typeof code === "string" && code !== ""
    ? code
    : undefined;
}

/** Whether the receiver's message says it is connected as a device, ready to be cast to. */
function isTargetReady(message: unknown): boolean {
  return at(message, "type") === "device" && at(message, "isReady") === true;
}

/**
 * The receiver's refusal, where its message says the core did not take it as a cast target: the
 * core's words, or what the receiver could say of why (the 11-24 batch's item 3).
 */
export function refusalOf(message: unknown): string | undefined {
  const refusal = at(message, "refusal");
  return at(message, "type") === "device" &&
    at(message, "isReady") === false &&
    typeof refusal === "string" &&
    refusal !== ""
    ? refusal
    : undefined;
}

/** How a press of Make it a cast target stands: not asked, waiting on the core, or answered. */
type TargetOutcome =
  | { readonly state: "none" }
  | { readonly state: "waiting" }
  | { readonly state: "ready"; readonly name: string }
  | { readonly state: "refused"; readonly name: string; readonly reason: string };

/** A code entered here, and the check code the server gave for it. */
interface Entered {
  readonly code: string;
  readonly checkCode: string;
}

/**
 * The receiver's messages of its pairing, heard while casting to it: its code entered under this
 * page's session, the check code kept to compare; and how the core answered it: ready to be cast to
 * once the core has taken its sign-in, or refused, which is logged with the core's words.
 */
function useTargetMessages(
  cast: CastSession | undefined,
  log: (line: string) => void,
): {
  readonly entered: Entered | undefined;
  readonly setEntered: (entered: Entered | undefined) => void;
  readonly outcome: TargetOutcome;
  readonly setOutcome: (outcome: TargetOutcome) => void;
} {
  const { api } = useServices();
  const [entered, setEntered] = useState<Entered | undefined>();
  const [outcome, setOutcome] = useState<TargetOutcome>({ state: "none" });
  useEffect(() => {
    if (cast === undefined) {
      return;
    }
    const deviceName = cast.receiverName === "" ? "Chromecast" : cast.receiverName;
    cast.onMessage((message) => {
      if (isTargetReady(message)) {
        setOutcome({ state: "ready", name: deviceName });
        return;
      }
      const reason = refusalOf(message);
      if (reason !== undefined) {
        setEntered(undefined);
        setOutcome({ state: "refused", name: deviceName, reason });
        log(`the core did not take ${deviceName} as a cast target: ${reason}`);
        return;
      }
      const code = pairingCodeOf(message);
      if (code === undefined) {
        return;
      }
      activateDevice(api, { code, deviceName }, newIdempotencyKey()).then(
        (activation) => {
          setEntered({ code, checkCode: activation.checkCode });
          log(`entered the receiver's code; check code ${activation.checkCode}`);
        },
        (cause: unknown) => {
          log(`could not enter the receiver's code: ${failureText(cause)}`);
        },
      );
    });
  }, [cast, api, log]);
  return { entered, setEntered, outcome, setOutcome };
}

/** The check code to compare with the TV's, and the person's answer. */
function CheckCode(props: {
  readonly checkCode: string;
  readonly onAnswer: (isMatch: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="lab__actions">
      <output className="lab__big">
        {t("lab.cast.checkCode", { checkCode: props.checkCode })}
      </output>
      <button
        type="button"
        className="lab__button"
        onClick={() => {
          props.onAnswer(true);
        }}
      >
        {t("lab.cast.codesMatch")}
      </button>
      <button
        type="button"
        className="lab__button"
        onClick={() => {
          props.onAnswer(false);
        }}
      >
        {t("lab.cast.codesDiffer")}
      </button>
    </div>
  );
}

/** What the core has said of the receiver as a cast target: waiting for it, ready, or refused. */
function TargetOutcomeView(props: { readonly outcome: TargetOutcome }): JSX.Element | undefined {
  const { t } = useTranslation();
  const { outcome } = props;
  switch (outcome.state) {
    case "none":
      return undefined;
    case "waiting":
      return <output className="lab__lead">{t("lab.cast.targetWaiting")}</output>;
    case "ready":
      return (
        <output className="lab__big">{t("lab.cast.targetReady", { name: outcome.name })}</output>
      );
    case "refused":
      return (
        <output className="lab__big" role="alert">
          {t("lab.cast.targetRefused", { name: outcome.name, reason: outcome.reason })}
        </output>
      );
  }
}

/** Making the receiver cast to a cast target: the press, the codes to compare, and the outcome. */
export function CastTargetPart(props: {
  readonly cast: CastSession | undefined;
  /** What asks the receiver to become one: castTargetRequest's. */
  readonly request: Readonly<Record<string, unknown>>;
  readonly log: (line: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { cast, log } = props;
  const { entered, setEntered, outcome, setOutcome } = useTargetMessages(cast, log);
  const answer = (isMatch: boolean): void => {
    if (entered === undefined) {
      return;
    }
    setEntered(undefined);
    answerActivation(api, { code: entered.code, isMatch }, newIdempotencyKey()).then(
      () => {
        log(isMatch ? "confirmed the check code" : "said the check codes differ");
      },
      (cause: unknown) => {
        log(`could not answer the check: ${failureText(cause)}`);
      },
    );
  };
  return (
    <>
      <h2>{t("lab.cast.target")}</h2>
      <p className="lab__lead">{t("lab.cast.targetExplain")}</p>
      <div className="lab__actions">
        <button
          type="button"
          className="lab__button"
          disabled={cast === undefined}
          onClick={() => {
            setOutcome({ state: "waiting" });
            cast?.send(props.request).catch((cause: unknown) => {
              log(failureText(cause));
            });
          }}
        >
          {t("lab.cast.makeTarget")}
        </button>
      </div>
      {entered === undefined ? undefined : (
        <CheckCode checkCode={entered.checkCode} onAnswer={answer} />
      )}
      <TargetOutcomeView outcome={outcome} />
    </>
  );
}
