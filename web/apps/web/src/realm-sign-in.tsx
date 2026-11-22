// SPDX-License-Identifier: MPL-2.0

import type { TFunction } from "i18next";
import { useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { installGuideOnGitHub } from "./agent-help.ts";
import { Checkbox } from "./checkbox.tsx";
import { CredentialsForm } from "./credentials-form.tsx";
import { missingText, TextField, useFormFields, type FieldProblem } from "./field.tsx";
import { PasswordField } from "./password-field.tsx";
import { SubmitButton } from "./submit-button.tsx";
import { StatusNote } from "./status-note.tsx";

/** What a person typed to sign in by realm, and whether this device keeps the realm (D297 §5). */
/**
 * What a realm sign-in says went wrong: a part wrong, said as one as the discovery does; the server
 * found but not reached from this network, and when this network has no IPv6 for a server with no
 * IPv4 way in, or this browser's secure connection to it never completed, said so (D398 §3); or a
 * server this device reached before not reached now.
 */
export type RealmProblem =
  | "wrong"
  | "unreached"
  | "unreachedNoIpv6"
  | "unreachedSecure"
  | "turnedAway"
  | "offline"
  | "again"
  | "keyRefused";

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
  /** Where the discovery's households keep their admin accounts, linked below the form; absent, none. */
  readonly accountOrigin?: string | undefined;
  /** What comes below the form, as what happened to copy (CredentialsForm's after). */
  readonly after?: ReactNode;
}

/**
 * The sign-in at a paid discovery's page for a person with no link: realm, name and password
 * together, and "remember my household", ticked when a realm was kept (D297 §2,
 * §5). Any part wrong says so as one, as the discovery does, naming no field (RealmProblem): only
 * what is missing is said at its field.
 */
export function RealmSignIn(props: RealmSignInProps): JSX.Element {
  const { t } = useTranslation();
  const [realm, setRealm] = useState(props.rememberedRealm ?? "");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [isRemembered, setIsRemembered] = useState(props.rememberedRealm !== undefined);
  const fields = useFormFields(undefined, []);
  return (
    <CredentialsForm
      heading={t("discovery.realm.heading")}
      onSubmit={() => {
        if (fields.check(missingOf(t, { realm, name, password }))) {
          props.onSubmit({ realm, name, password, isRemembered });
        }
      }}
      after={props.after}
    >
      <TextField
        {...fields.fieldOf("realm")}
        label={t("discovery.realm.realm")}
        hint={t("discovery.realm.realmHint")}
        name="realm"
        autoComplete="organization"
        isVerbatim
        value={realm}
        onChange={fields.editing("realm", setRealm)}
      />
      <TextField
        {...fields.fieldOf("name")}
        label={t("discovery.realm.name")}
        name="name"
        autoComplete="username"
        isVerbatim
        value={name}
        onChange={fields.editing("name", setName)}
      />
      <PasswordField
        {...fields.fieldOf("password")}
        label={t("discovery.realm.password")}
        value={password}
        onChange={fields.editing("password", setPassword)}
        autoComplete="current-password"
      />
      <Checkbox
        label={t("discovery.realm.remember")}
        isChecked={isRemembered}
        onChange={setIsRemembered}
      />
      {props.problem === undefined ? null : (
        <StatusNote told={{ kind: "problem", text: t(`discovery.realm.${props.problem}`) }} />
      )}
      <SubmitButton label={t("discovery.realm.submit")} isBusy={props.isSending} />
      <OtherWays onUseCode={props.onUseCode} accountOrigin={props.accountOrigin} />
    </CredentialsForm>
  );
}

/** What a sign-in sent now would lack: each field left empty, to be said at that field. */
function missingOf(
  t: TFunction,
  typed: { readonly realm: string; readonly name: string; readonly password: string },
): readonly FieldProblem[] {
  return [
    ["realm", missingText(t, t("discovery.realm.realm"), typed.realm)],
    ["name", missingText(t, t("discovery.realm.name"), typed.name)],
    ["password", missingText(t, t("discovery.realm.password"), typed.password)],
  ];
}

/**
 * The other ways in than a realm: at home, the server's own address, which needs no account at all;
 * the link sent, and a code where one is offered; and, for a person who came to set up a server
 * rather than to sign in, the install guide (a household's first install, 2026-11-19: the page
 * showed no way to it, and a phone and a TV at home were pointed at it first); and, for a household's
 * admin come for a key, the way to their admin account at the discovery (the owner, 2026-11-20).
 */
function OtherWays(props: {
  readonly onUseCode: (() => void) | undefined;
  readonly accountOrigin: string | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <p>{t("discovery.realm.atHome")}</p>
      <p>{t("discovery.realm.orLink")}</p>
      {props.onUseCode === undefined ? null : (
        <button type="button" onClick={props.onUseCode}>
          {t("discovery.realm.useCode")}
        </button>
      )}
      <p>
        <a href={installGuideOnGitHub} rel="noreferrer noopener" target="_blank">
          {t("discovery.realm.install")}
        </a>
      </p>
      {props.accountOrigin === undefined ? null : (
        <p>
          <a href={props.accountOrigin} rel="noreferrer noopener" target="_blank">
            {t("discovery.realm.adminAccount")}
          </a>
        </p>
      )}
    </>
  );
}
