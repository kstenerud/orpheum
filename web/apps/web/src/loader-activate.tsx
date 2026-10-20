// SPDX-License-Identifier: MPL-2.0

/**
 * The loader's /activate, which a TV's QR code opens on a phone (D321): the code and the TV's key's
 * hash in its query. With one realm the browser goes straight to that household's Activate a device,
 * the code filled in; with several it asks which; with none it offers the link to copy to a phone
 * that is signed in, or a sign-in here that carries on to Activate a device.
 */

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { copyToClipboard } from "./clipboard.ts";
import { CredentialsForm } from "./credentials-form.tsx";
import type { Household } from "./loader-households.ts";
import { routeHref, type Scanned } from "./router.ts";

/** The path the loader's QR code link has. */
export const activatePath = "/activate";

/** The scanned code the loader's address carries, or undefined when it is no /activate link. */
export function scannedAt(location: {
  readonly pathname: string;
  readonly search: string;
}): Scanned | undefined {
  if (location.pathname.replace(/\/$/u, "") !== activatePath) {
    return undefined;
  }
  const query = new URLSearchParams(location.search);
  const code = query.get("code") ?? "";
  const keyHash = query.get("key") ?? "";
  return code === "" || keyHash === "" ? undefined : { code, keyHash };
}

/** Where a household's client activates the scanned code: its account, with the code (D321). */
export function activateHere(scanned: Scanned): string {
  return routeHref({ screen: "account", scanned });
}

/** The realm a household is known by, as the choice names it: its realm, else its name or id. */
function realmOf(household: Household): string {
  if (household.realm !== "") {
    return household.realm;
  }
  return household.name === "" ? household.coreId : household.name;
}

/**
 * The page /activate shows when the browser cannot go straight on: several realms, to choose which
 * the TV joins; or none, the link to copy to a phone that is signed in, or a sign-in here.
 */
export function ActivateWhere(props: {
  readonly scanned: Scanned;
  readonly listed: readonly Household[];
  readonly originOf: (coreId: string) => string | undefined;
  /** The address of this page, the link to copy. */
  readonly link: string;
  readonly onSignInHere: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [copied, setCopied] = useState<"copied" | "refused" | undefined>(undefined);
  if (props.listed.length > 0) {
    return (
      <CredentialsForm heading={t("loader.activate.whichRealm")} onSubmit={props.onSignInHere}>
        <ul className="loader-households">
          {props.listed.map((household) => {
            const origin = props.originOf(household.coreId);
            return origin === undefined ? null : (
              <li key={household.coreId}>
                <a href={`${origin}${activateHere(props.scanned)}`}>{realmOf(household)}</a>
              </li>
            );
          })}
        </ul>
        <button type="submit">{t("loader.activate.signInHere")}</button>
      </CredentialsForm>
    );
  }
  return (
    <CredentialsForm heading={t("loader.activate.heading")} onSubmit={props.onSignInHere}>
      <p>{t("loader.activate.notSignedIn")}</p>
      <p>
        <code className="loader-activate__link">{props.link}</code>
      </p>
      <button
        type="button"
        onClick={() => {
          copyToClipboard(props.link).then(
            () => {
              setCopied("copied");
            },
            () => {
              setCopied("refused");
            },
          );
        }}
      >
        {t("loader.activate.copy")}
      </button>
      {copied === undefined ? null : <output>{t(`loader.activate.${copied}`)}</output>}
      <button type="submit">{t("loader.activate.signInHere")}</button>
    </CredentialsForm>
  );
}
