// SPDX-License-Identifier: MPL-2.0

import type { TFunction } from "i18next";
import type { JSX, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import type { Sending } from "./lab-send.ts";
import { Link } from "./link.tsx";

/**
 * A probe's page: the way back to the lab's menu and its heading, on one line where they fit, and
 * its body; compact, its parts closer together, for a page that must fit a TV's screen unscrolled.
 */
export function LabFrame(props: {
  readonly id: string;
  readonly heading: string;
  readonly isCompact?: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <section
      className={props.isCompact === true ? "lab lab--compact" : "lab"}
      aria-labelledby={`${props.id}-heading`}
    >
      <div className="lab__top">
        <p>
          <Link to={{ screen: "lab" }} className="lab__back">
            {t("lab.backToMenu")}
          </Link>
        </p>
        <h1 id={`${props.id}-heading`}>{props.heading}</h1>
      </div>
      {props.children}
    </section>
  );
}

/** The Send to server button, and where the sending stands: its name in the log once sent. */
export function LabSendButton(props: {
  readonly sending: Sending;
  readonly onSend: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const said = sendingText(t, props.sending);
  return (
    <div className="lab__send">
      <button
        type="button"
        className="lab__button"
        disabled={props.sending.state === "sending"}
        onClick={props.onSend}
      >
        {t("lab.send")}
      </button>
      {said === undefined ? null : (
        <output className="lab__big" aria-live="polite">
          {said}
        </output>
      )}
    </div>
  );
}

/** What a sending's state says, or undefined before it is sent. */
function sendingText(t: TFunction, sending: Sending): string | undefined {
  switch (sending.state) {
    case "unsent":
      return undefined;
    case "sending":
      return t("lab.sending");
    case "sent":
      return t("lab.sent", { id: sending.id });
    case "failed":
      return t("lab.sendFailed");
  }
}

/** A yes or no the lab shows, in words. */
export function yesNo(t: TFunction, isYes: boolean | undefined): string {
  if (isYes === undefined) {
    return t("lab.unknown");
  }
  return isYes ? t("lab.yes") : t("lab.no");
}
