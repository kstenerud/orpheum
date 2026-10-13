// SPDX-License-Identifier: MPL-2.0

import "./code-sign-in.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  forgetPairing,
  keepPairing,
  pairingStateOf,
  readPairing,
  requestPairing,
  takePairedSession,
  type DevicePairing,
  type PairingState,
} from "./pairing.ts";
import { tabStorage } from "./browser-storage.ts";
import { CredentialsForm, CredentialsProblem } from "./credentials-form.tsx";
import { backingOffPoller, type Poller } from "./polling.ts";
import { problemText } from "./problem.ts";
import { useServices, type Services } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/**
 * How often a device showing its code asks how its pairing stands: every 2 s, with no backing off,
 * since the person is entering it on another device now and waits for this one (D288).
 */
const pairingPolling = backingOffPoller({ firstMs: 2000, longestMs: 2000, failedMs: 3000 });

/** What the device shows: its code and how its pairing stands, or that it is signing in. */
type Shown =
  | {
      readonly kind: "pairing";
      readonly pairing: DevicePairing;
      readonly state: PairingState;
      /** Whether this code replaced one that ended: refused, expired or lost to a restart. */
      readonly isRenewed: boolean;
    }
  | { readonly kind: "signingIn" };

/**
 * Looks at this device's pairing: asks for a code when it has none; signs in once the person
 * confirmed it, and asks for a new code once it ended (D288).
 */
async function lookAtPairing(
  services: Pick<Services, "api" | "session" | "navigation">,
): Promise<Shown> {
  const { api, session, navigation } = services;
  const storage = tabStorage();
  const ask = async (isRenewed: boolean): Promise<Shown> => {
    const pairing = await requestPairing(api, {
      secret: undefined,
      previousDeviceId: session.deviceId(),
      previousDeviceProof: session.deviceProof(),
    });
    keepPairing(storage, pairing);
    return { kind: "pairing", pairing, state: { state: "waiting" }, isRenewed };
  };
  const kept = readPairing(storage, Date.now());
  if (kept === undefined) {
    return await ask(false);
  }
  const state = await pairingStateOf(api, kept.secret);
  if (state.state === "activated") {
    const { token, deviceSecret, user } = await takePairedSession(api, kept.secret);
    forgetPairing(storage);
    // A television signed in by code starts at home, not wherever the address last was.
    navigation.go({ screen: "films" });
    session.signIn(token, user.device?.id, deviceSecret);
    return { kind: "signingIn" };
  }
  if (state.state === "ended") {
    forgetPairing(storage);
    return await ask(true);
  }
  return { kind: "pairing", pairing: kept, state, isRenewed: false };
}

/**
 * Signs this device in by a code it shows (D288): a person signed in on another device enters it
 * under Activate a device, both devices then show a check code, and once the person confirms they
 * match, this device is signed in as them. A reload in this tab keeps the same code.
 */
export function CodeSignIn(props: {
  readonly onBack: () => void;
  /** How it looks at its pairing again: every 2 s, unless a test runs it by hand. */
  readonly poller?: Poller;
}): JSX.Element {
  const { t } = useTranslation();
  const services = useServices();
  const [shown] = useResource(
    "pairing",
    () => lookAtPairing(services),
    props.poller ?? pairingPolling,
  );
  return (
    <CredentialsForm heading={t("codeSignIn.heading")} onSubmit={props.onBack}>
      {shown.state === "loaded" ? <PairingView shown={shown.value} /> : null}
      {shown.state === "failed" ? (
        <CredentialsProblem isAlert>{problemText(t, shown.cause)}</CredentialsProblem>
      ) : null}
      <button type="submit">{t("codeSignIn.back")}</button>
    </CredentialsForm>
  );
}

/** The device's code while no one has entered it, then the check code to compare. */
function PairingView(props: { readonly shown: Shown }): JSX.Element | null {
  const { t } = useTranslation();
  const { shown } = props;
  if (shown.kind === "signingIn") {
    return <p>{t("codeSignIn.signingIn")}</p>;
  }
  if (shown.state.state === "checking") {
    return (
      <>
        <p>{t("codeSignIn.checkExplain")}</p>
        <output className="code-sign-in__code" aria-label={t("codeSignIn.checkCode")}>
          {shown.state.checkCode}
        </output>
      </>
    );
  }
  return (
    <>
      {shown.isRenewed ? <p>{t("codeSignIn.renewed")}</p> : null}
      <p>{t("codeSignIn.explain")}</p>
      <output className="code-sign-in__code" aria-label={t("codeSignIn.code")}>
        {shown.pairing.code}
      </output>
    </>
  );
}
