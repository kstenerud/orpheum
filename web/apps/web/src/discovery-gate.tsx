// SPDX-License-Identifier: MPL-2.0

import { NotFound, reportDeadEnd, type DiscoveryClient, type RtcTransport } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CredentialsForm } from "./credentials-form.tsx";
import {
  connectThrough,
  targetOfLink,
  targetOfSession,
  type HeldSession,
  type KeptCore,
  type Start,
  type Target,
} from "./discovery-start.ts";

/** What the gate needs: the discovery, the remembered core's session, and who takes over. */
export interface GateParts {
  readonly client: DiscoveryClient;
  /** The session this device holds with core, when it holds one. */
  readonly sessionOf: (core: KeptCore) => HeldSession | undefined;
  /** Told the core reached and the transport to it; the app takes over from there. */
  readonly onConnected: (core: KeptCore, transport: RtcTransport) => void;
}

type Phase = "waiting" | "connecting" | "notFound" | "deadEnd";

/** The phase a start opens in: a link waits for its click; a remembered core is reached at once. */
function firstPhase(start: Start, parts: GateParts): Phase {
  if (start.kind === "link") {
    return "waiting";
  }
  return start.kind === "remembered" && parts.sessionOf(start.core) !== undefined
    ? "connecting"
    : "notFound";
}

/** Reaches a core: finds it, connects, and says how it went. */
async function reach(
  parts: GateParts,
  core: KeptCore,
  find: () => Promise<Target>,
): Promise<"notFound" | "deadEnd" | undefined> {
  let target: Target | undefined;
  try {
    target = await find();
    parts.onConnected(core, await connectThrough(parts.client, target));
    return undefined;
  } catch (problem: unknown) {
    if (problem instanceof NotFound) {
      return "notFound";
    }
    if (target !== undefined) {
      await reportDeadEnd(parts.client, target.held);
    }
    return "deadEnd";
  }
}

/**
 * The page a discovery's origin opens on, until the core is reached (webrtc-transport.md W4c): a link
 * is spent by a click, never by loading the page (the protocol's §3.1); a remembered core is reached at
 * once; with neither, the page says how to get in. When nothing reaches the core, it says so for
 * whoever reads it and tells the discovery (§3.6).
 */
export function DiscoveryGate(props: {
  readonly start: Start;
  readonly parts: GateParts;
}): JSX.Element {
  const { start, parts } = props;
  const [phase, setPhase] = useState<Phase>(() => firstPhase(start, parts));
  const reachingRef = useRef(false);
  const reachThen = (core: KeptCore, find: () => Promise<Target>): void => {
    if (reachingRef.current) {
      return;
    }
    reachingRef.current = true;
    void reach(parts, core, find).then((ended) => {
      reachingRef.current = false;
      if (ended !== undefined) {
        setPhase(ended);
      }
    });
  };
  useEffect(() => {
    const held = start.kind === "remembered" ? parts.sessionOf(start.core) : undefined;
    if (held !== undefined) {
      reachThen(held.core, () => targetOfSession(parts.client, held));
    }
    // Once, as the page opens: a remembered core is reached at once.
    // eslint-disable-next-line react-hooks/exhaustive-deps, @eslint-react/exhaustive-deps -- the start and the parts never change
  }, []);
  if (start.kind === "link" && phase === "waiting") {
    const { link } = start;
    return (
      <LinkPage
        onConnect={() => {
          setPhase("connecting");
          reachThen({ coreId: link.coreId, fingerprint: link.fingerprint }, () =>
            targetOfLink(parts.client, link),
          );
        }}
      />
    );
  }
  return <GateMessage phase={start.kind === "none" ? "none" : phase} />;
}

/** A link's page: what it is, and the click that spends it. */
function LinkPage(props: { readonly onConnect: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsForm heading={t("discovery.link.heading")} onSubmit={props.onConnect}>
      <p>{t("discovery.link.text")}</p>
      <button type="submit">{t("discovery.link.connect")}</button>
    </CredentialsForm>
  );
}

/** A page of the gate that only says how it stands. */
function GateMessage(props: { readonly phase: Phase | "none" }): JSX.Element {
  const { t } = useTranslation();
  if (props.phase === "connecting" || props.phase === "waiting") {
    return (
      <CredentialsForm heading={t("discovery.connecting")} onSubmit={() => undefined}>
        {null}
      </CredentialsForm>
    );
  }
  return (
    <CredentialsForm heading={t(`discovery.${props.phase}.heading`)} onSubmit={() => undefined}>
      <output>{t(`discovery.${props.phase}.text`)}</output>
    </CredentialsForm>
  );
}
