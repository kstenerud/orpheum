// SPDX-License-Identifier: MPL-2.0

import type { DeadEndCause } from "@orpheum/sdk";
import { useEffect, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { CredentialsForm } from "./credentials-form.tsx";
import { OpenInBrowser } from "./open-in-browser.tsx";

/**
 * The pages the discovery's gate says how it stands by, kept apart from how it reaches its core
 * (discovery-gate.tsx) so each fits in one read: turned away, its server not connected, a browser
 * that cannot play, and a dead end by the cause the page knows (D398 §3).
 */

/** Where the gate stands: what it shows. */
export type Phase =
  | "waiting"
  | "connecting"
  | "notFound"
  | "deadEnd"
  | "noWorker"
  | "turnedAway"
  | "offline"
  | "keyRefused"
  | "realm"
  | "realmWrong"
  | "realmUnreached"
  | "realmTurnedAway"
  | "realmOffline"
  | "realmAgain"
  | "realmKeyRefused";

/**
 * The page a core that turned this device away for its network's share leaves: what to do, and a way
 * to try again once pages are closed. It is no network failure, and says none (the owner's Fire TV,
 * 2026-10-25, told its network could not reach the server).
 */
function TurnedAway(props: { readonly onRetry: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsForm heading={t("discovery.turnedAway.heading")} onSubmit={props.onRetry}>
      <output>{t("discovery.turnedAway.text")}</output>
      <button type="submit">{t("discovery.turnedAway.again")}</button>
    </CredentialsForm>
  );
}

/**
 * How long the page waits before it tries a server its discovery cannot reach again, by itself: a
 * restart takes some ten seconds. A justified constant.
 */
export const offlineRetryAfterMs = 5000;

/**
 * The page a server its discovery cannot reach leaves: restarting, switched off, or without
 * internet, never this device's network; it tries again by itself every few seconds, and on the
 * button (the owner's Fire TV, 2026-10-25: a restart read as "can't be reached from this network").
 */
function Offline(props: { readonly onRetry: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { onRetry } = props;
  useEffect(() => {
    const timer = setTimeout(onRetry, offlineRetryAfterMs);
    return () => {
      clearTimeout(timer);
    };
  }, [onRetry]);
  return (
    <CredentialsForm heading={t("discovery.offline.heading")} onSubmit={onRetry}>
      <output>{t("discovery.offline.text")}</output>
      <button type="submit">{t("discovery.offline.again")}</button>
    </CredentialsForm>
  );
}

/**
 * A page of the gate that says how it stands, and, for a device turned away, tries again; after
 * comes below its form (CredentialsForm's after). A dead end says the cause the page knows (D398 §3):
 * a secure connection that never completed was no network's failure, and its page says so and to try
 * another browser (the owner's Firefox 158, 2026-11-07, where Chromium on the same laptop worked).
 */
export function GateMessage(props: {
  readonly phase: Phase | "none";
  readonly cause: DeadEndCause;
  readonly onRetry: () => void;
  readonly after: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  if (props.phase === "turnedAway") {
    return <TurnedAway onRetry={props.onRetry} />;
  }
  if (props.phase === "offline") {
    return <Offline onRetry={props.onRetry} />;
  }
  if (props.phase === "noWorker") {
    return <OpenInBrowser after={props.after} />;
  }
  if (props.phase === "connecting" || props.phase === "waiting") {
    return (
      <CredentialsForm heading={t("discovery.connecting")} onSubmit={() => undefined}>
        {null}
      </CredentialsForm>
    );
  }
  if (props.phase === "deadEnd" && props.cause !== "unknown") {
    const words = deadEndWords[props.cause];
    return (
      <CredentialsForm heading={t(words.heading)} onSubmit={() => undefined} after={props.after}>
        <output>{t(words.text)}</output>
      </CredentialsForm>
    );
  }
  return (
    <CredentialsForm
      heading={t(`discovery.${props.phase}.heading`)}
      onSubmit={() => undefined}
      after={props.after}
    >
      <output>{t(`discovery.${props.phase}.text`)}</output>
    </CredentialsForm>
  );
}

/** A dead end's heading and words, by the cause the page knows (D398 §3). */
const deadEndWords = {
  noIpv6: { heading: "discovery.deadEnd.heading", text: "discovery.deadEnd.noIpv6" },
  secureUnfinished: {
    heading: "discovery.deadEnd.secure.heading",
    text: "discovery.deadEnd.secure.text",
  },
} as const;
