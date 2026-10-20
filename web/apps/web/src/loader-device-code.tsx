// SPDX-License-Identifier: MPL-2.0

/**
 * A device that knows no core signs in by a code from the discovery's service (D293 §2): a
 * television at the loader's origin shows the code, a person signed in on their phone enters it under
 * Activate a device, both show the check code made of this page's key, and once the person says they
 * match, this page opens what the core sealed to that key and goes on to the core's client origin as
 * a realm sign-in does, with the hand-off's code. Nothing the service passes on is readable to it.
 */

import {
  deviceCheckCode,
  deviceCodeState,
  newDeviceKey,
  openDeviceOutcome,
  requestDeviceCode,
  type DeviceCode,
  type DeviceKey,
  type DiscoveryClient,
} from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import "./code-sign-in.css";
import { CredentialsForm, CredentialsProblem } from "./credentials-form.tsx";
import { thisDevice } from "./device.ts";
import { handOffFragment } from "./discovery-start.ts";

/** How often the page asks how its code stands: the person is entering it now, so every 2 s. */
const pollMs = 2000;

/** What the code page reaches and goes on by. */
export interface DeviceCodeParts {
  readonly client: DiscoveryClient;
  /** The client origin of the core coreId, or undefined for an id that names none. */
  readonly originOf: (coreId: string) => string | undefined;
  /** Remembers the household the device was activated into, for the loader's list. */
  readonly remember: (core: { coreId: string; fingerprint: string; realm: string }) => void;
  /** Goes on to address, leaving the loader's page. */
  readonly go: (address: string) => void;
  /** Waits ms before the page asks again; a test answers at once. */
  readonly wait?: (ms: number) => Promise<void>;
  /** Makes the key each code is asked for with; a test's is one the core sealed a fixture to. */
  readonly newKey?: () => Promise<DeviceKey>;
}

/** What the page shows: its code, then the check code, or that it signs in; or what went wrong. */
type Shown =
  | { readonly kind: "asking" }
  | { readonly kind: "code"; readonly code: string; readonly isRenewed: boolean }
  | { readonly kind: "check"; readonly checkCode: string }
  | { readonly kind: "signingIn" }
  | { readonly kind: "failed" };

/** Waits ms, by the browser's clock. */
function browserWait(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Runs one device's code to its end while isLive says the page still shows it: asks for a code,
 * shows it, shows the check code once it is entered, asks for a new one when it ends, and goes on to
 * the core once it is activated.
 */
async function runCode(
  parts: DeviceCodeParts,
  show: (shown: Shown) => void,
  isLive: () => boolean,
): Promise<void> {
  const wait = parts.wait ?? browserWait;
  const platform = thisDevice({ deviceId: () => undefined, deviceProof: () => undefined }).platform;
  let isRenewed = false;
  while (isLive()) {
    const key = await (parts.newKey ?? newDeviceKey)();
    const code = await requestDeviceCode(parts.client, key, platform);
    show({ kind: "code", code: code.code, isRenewed });
    const isEnded = await followCode({ parts, key, code, show, isLive, wait });
    if (!isEnded) {
      return;
    }
    isRenewed = true;
  }
}

/** One code the page follows: what it reaches by, its key and code, and how it shows and waits. */
interface Following {
  readonly parts: DeviceCodeParts;
  readonly key: DeviceKey;
  readonly code: DeviceCode;
  readonly show: (shown: Shown) => void;
  readonly isLive: () => boolean;
  readonly wait: (ms: number) => Promise<void>;
}

/** Follows code until it ends, true, or the page went on to the core or stopped showing it, false. */
async function followCode(following: Following): Promise<boolean> {
  const { parts, key, code, show, isLive, wait } = following;
  let isChecking = false;
  while (isLive()) {
    await wait(pollMs);
    const stands = await deviceCodeState(parts.client, code.secret);
    if (stands.state === "ended") {
      return true;
    }
    if (stands.state === "entered" && !isChecking) {
      isChecking = true;
      show({ kind: "check", checkCode: await deviceCheckCode(key.publicKey) });
    }
    if (stands.state === "activated") {
      show({ kind: "signingIn" });
      const outcome = await openDeviceOutcome(key, stands.sealed);
      const origin = parts.originOf(outcome.coreId);
      if (origin === undefined) {
        throw new Error("the activated device's server has no client origin here");
      }
      parts.remember(outcome);
      parts.go(
        `${origin}/${handOffFragment({
          code: outcome.handOffCode,
          bootstrapToken: stands.bootstrapToken,
          fingerprint: outcome.fingerprint,
          realm: outcome.realm,
        })}`,
      );
      return false;
    }
  }
  return false;
}

/**
 * The page a television signs in on by a code: the code, then the check code to compare, and back
 * to the realm sign-in.
 */
export function DeviceCodeSignIn(props: {
  readonly parts: DeviceCodeParts;
  readonly onBack: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [shown, setShown] = useState<Shown>({ kind: "asking" });
  useEffect(() => {
    let isLive = true;
    runCode(
      props.parts,
      (next) => {
        if (isLive) {
          setShown(next);
        }
      },
      () => isLive,
    ).catch(() => {
      if (isLive) {
        setShown({ kind: "failed" });
      }
    });
    return () => {
      isLive = false;
    };
  }, [props.parts]);
  return (
    <CredentialsForm heading={t("loader.code.heading")} onSubmit={props.onBack}>
      <CodeView shown={shown} />
      <button type="submit">{t("loader.code.back")}</button>
    </CredentialsForm>
  );
}

/** The code while no one has entered it, then the check code, or what happened. */
function CodeView(props: { readonly shown: Shown }): JSX.Element | null {
  const { t } = useTranslation();
  const { shown } = props;
  switch (shown.kind) {
    case "asking":
      return null;
    case "failed":
      return <CredentialsProblem isAlert>{t("loader.code.failed")}</CredentialsProblem>;
    case "signingIn":
      return <p>{t("loader.code.signingIn")}</p>;
    case "check":
      return (
        <>
          <p>{t("loader.code.checkExplain")}</p>
          <output className="code-sign-in__code" aria-label={t("loader.code.checkCode")}>
            {shown.checkCode}
          </output>
        </>
      );
    case "code":
      return (
        <>
          {shown.isRenewed ? <p>{t("loader.code.renewed")}</p> : null}
          <p>{t("loader.code.explain")}</p>
          <output className="code-sign-in__code" aria-label={t("loader.code.code")}>
            {shown.code}
          </output>
        </>
      );
  }
}
