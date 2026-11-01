// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  CredentialsField,
  CredentialsForm,
  CredentialsProblem,
  PasswordField,
} from "./credentials-form.tsx";

/** What a person typed to sign in by realm, and whether this device keeps the realm (D297 §5). */
/**
 * What a realm sign-in says went wrong: a part wrong, said as one as the discovery does; the server
 * found but not reached from this network, and when this network has no IPv6 for a server with no
 * IPv4 way in, said so (D398 §3); or a server this device reached before not reached now.
 */
export type RealmProblem =
  "wrong" | "unreached" | "unreachedNoIpv6" | "turnedAway" | "offline" | "again" | "keyRefused";

export interface RealmCredentials {
  readonly realm: string;
  readonly name: string;
  readonly password: string;
  readonly isRemembered: boolean;
}

/** What the realm sign-in shows, and is told. */
interface RealmSignInProps {
  readonly rememberedRealm: string | undefined;
  readonly isSending: boolean;
  readonly problem: RealmProblem | undefined;
  readonly onSubmit: (typed: RealmCredentials) => void;
  /** Offers a code instead, for a device no one types a password on: a television (D293 §2). */
  readonly onUseCode?: (() => void) | undefined;
}

/**
 * The sign-in at a paid discovery's page for a person with no link: realm, name and password
 * together, and "this is my own device: remember my realm", ticked when a realm was kept (D297 §2,
 * §5). Any part wrong says so as one, as the discovery does.
 */
export function RealmSignIn(props: RealmSignInProps): JSX.Element {
  const { t } = useTranslation();
  const [realm, setRealm] = useState(props.rememberedRealm ?? "");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [isRemembered, setIsRemembered] = useState(props.rememberedRealm !== undefined);
  return (
    <CredentialsForm
      heading={t("discovery.realm.heading")}
      onSubmit={() => {
        props.onSubmit({ realm, name, password, isRemembered });
      }}
    >
      <PlainField
        label={t("discovery.realm.realm")}
        name="realm"
        autoComplete="organization"
        value={realm}
        onChange={setRealm}
        describedBy="realm-hint"
      />
      <p id="realm-hint">{t("discovery.realm.realmHint")}</p>
      <PlainField
        label={t("discovery.realm.name")}
        name="name"
        autoComplete="username"
        value={name}
        onChange={setName}
      />
      <PasswordField
        label={t("discovery.realm.password")}
        value={password}
        onChange={setPassword}
        autoComplete="current-password"
      />
      <label className="credentials-form__check">
        <input
          type="checkbox"
          checked={isRemembered}
          onChange={(event) => {
            setIsRemembered(event.target.checked);
          }}
        />
        {t("discovery.realm.remember")}
      </label>
      {props.problem === undefined ? null : (
        <CredentialsProblem isAlert>{t(`discovery.realm.${props.problem}`)}</CredentialsProblem>
      )}
      <button type="submit" disabled={props.isSending}>
        {props.isSending ? t("discovery.realm.signingIn") : t("discovery.realm.submit")}
      </button>
      <OtherWays onUseCode={props.onUseCode} />
    </CredentialsForm>
  );
}

/** The other ways in than a realm: the link sent, and a code where one is offered. */
function OtherWays(props: { readonly onUseCode: (() => void) | undefined }): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <p>{t("discovery.realm.orLink")}</p>
      {props.onUseCode === undefined ? null : (
        <button type="button" onClick={props.onUseCode}>
          {t("discovery.realm.useCode")}
        </button>
      )}
    </>
  );
}

/** A required text field typed as it is, never capitalised or corrected: a realm, a name. */
function PlainField(props: {
  readonly label: string;
  readonly name: string;
  readonly autoComplete: string;
  readonly value: string;
  readonly onChange: (value: string) => void;
  readonly describedBy?: string;
}): JSX.Element {
  return (
    <CredentialsField label={props.label}>
      <input
        name={props.name}
        autoComplete={props.autoComplete}
        autoCapitalize="none"
        spellCheck={false}
        required
        value={props.value}
        aria-describedby={props.describedBy}
        onChange={(event) => {
          props.onChange(event.target.value);
        }}
      />
    </CredentialsField>
  );
}
