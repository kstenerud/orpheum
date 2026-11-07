// SPDX-License-Identifier: MPL-2.0

import "./lab-words.ts";

import { failureText } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { activateDevice, answerActivation } from "./accounts.ts";
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
  if (typeof message !== "object" || message === null) {
    return undefined;
  }
  const code: unknown = Reflect.get(message, "code");
  return Reflect.get(message, "type") === "pairing" && typeof code === "string" && code !== ""
    ? code
    : undefined;
}

/** Whether the receiver's message says it is connected as a device, ready to be cast to. */
function isTargetReady(message: unknown): boolean {
  return (
    typeof message === "object" &&
    message !== null &&
    Reflect.get(message, "type") === "device" &&
    Reflect.get(message, "isReady") === true
  );
}

/** A code entered here, and the check code the server gave for it. */
interface Entered {
  readonly code: string;
  readonly checkCode: string;
}

/**
 * The receiver's messages of its pairing, heard while casting to it: its code entered under this
 * page's session, the check code kept to compare; and whether it said it is ready to be cast to.
 */
function useTargetMessages(
  cast: CastSession | undefined,
  log: (line: string) => void,
): {
  readonly entered: Entered | undefined;
  readonly setEntered: (entered: Entered | undefined) => void;
  readonly readyName: string | undefined;
  readonly setReadyName: (name: string | undefined) => void;
} {
  const { api } = useServices();
  const [entered, setEntered] = useState<Entered | undefined>();
  const [readyName, setReadyName] = useState<string | undefined>();
  useEffect(() => {
    if (cast === undefined) {
      return;
    }
    const deviceName = cast.receiverName === "" ? "Chromecast" : cast.receiverName;
    cast.onMessage((message) => {
      if (isTargetReady(message)) {
        setReadyName(deviceName);
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
  return { entered, setEntered, readyName, setReadyName };
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
  const { entered, setEntered, readyName, setReadyName } = useTargetMessages(cast, log);
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
            setReadyName(undefined);
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
      {readyName === undefined ? undefined : (
        <output className="lab__big">{t("lab.cast.targetReady", { name: readyName })}</output>
      )}
    </>
  );
}
