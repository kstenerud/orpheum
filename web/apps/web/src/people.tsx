// SPDX-License-Identifier: MPL-2.0

import "./people.css";

import { useState, type JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import {
  createAccount,
  listAccounts,
  registrationUrl,
  type Account,
  type IssuedSecret,
} from "./accounts.ts";
import { FormRow } from "./form-row.tsx";
import { problemText } from "./problem.ts";
import { Link } from "./link.tsx";
import { QrCode } from "./qr-code.tsx";
import { Loaded } from "./title.tsx";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { useResource } from "./use-resource.ts";

/** What the admin last issued, to pass on: a person's link or a device's code. */
export type IssuedInvitation =
  | { readonly kind: "link"; readonly name: string; readonly invitation: IssuedSecret }
  | { readonly kind: "code"; readonly name: string; readonly invitation: IssuedSecret };

/**
 * The household, for the admin (D218): everyone, whether they have registered, a way to add a
 * person, and for each a new registration link or a code for a device.
 */
export function People(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [accounts, reload] = useResource("accounts", () => listAccounts(api));
  const [issued, setIssued] = useState<IssuedInvitation | undefined>(undefined);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const showProblem = (cause: unknown): void => {
    setProblem(problemText(t, cause, { conflict: t("people.nameTaken") }));
  };
  return (
    <section className="people" aria-labelledby="people-heading">
      <h1 id="people-heading">{t("people.heading")}</h1>
      <AddPerson
        onAdded={(name, invitation) => {
          setIssued({ kind: "link", name, invitation });
          reload();
        }}
        onFailed={showProblem}
      />
      {issued === undefined ? null : <IssuedInvitationView issued={issued} />}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <Loaded resource={accounts} render={(list) => <PeopleList accounts={list} />} />
    </section>
  );
}

/** What was just issued, to pass on: a link also as a QR code a phone's camera opens (D218 §1). */
export function IssuedInvitationView(props: { readonly issued: IssuedInvitation }): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const { issued } = props;
  const text =
    issued.kind === "link"
      ? registrationUrl(navigation.origin(), issued.invitation.secret)
      : issued.invitation.secret;
  return (
    <div className="people__issued">
      <h2>{t(`people.issued.${issued.kind}`, { name: issued.name })}</h2>
      {issued.kind === "link" ? (
        <QrCode text={text} label={t("people.qrLabel", { name: issued.name })} />
      ) : null}
      <Share kind={issued.kind} text={text} invitation={issued.invitation} />
    </div>
  );
}

/** Adds a person: their name and role, and their registration link to pass on. */
function AddPerson(props: {
  readonly onAdded: (name: string, invitation: IssuedSecret) => void;
  readonly onFailed: (cause: unknown) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [name, setName] = useState("");
  const [role, setRole] = useState<"admin" | "user">("user");
  const submit = (): void => {
    createAccount(api, name, role).then(({ registrationInvitation: registration }) => {
      props.onAdded(name, registration);
      setName("");
    }, props.onFailed);
  };
  return (
    <FormRow onSubmit={submit}>
      <h2>{t("people.add")}</h2>
      <label>
        {t("people.name")}{" "}
        <input
          required
          maxLength={100}
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </label>
      <label>
        {t("people.role")}{" "}
        <select
          value={role}
          onChange={(event) => {
            setRole(event.target.value === "admin" ? "admin" : "user");
          }}
        >
          <option value="user">{t("people.roleUser")}</option>
          <option value="admin">{t("people.roleAdmin")}</option>
        </select>
      </label>
      <button type="submit">{t("people.create")}</button>
    </FormRow>
  );
}

/** Whether a person has set their password yet, in words. */
export function registrationText(t: TFunction, account: { readonly hasPassword: boolean }): string {
  return t(account.hasPassword ? "people.registered" : "people.notRegistered");
}

/** Everyone, each leading to their page (the owner, 2026-09-27). */
export function PeopleList(props: { readonly accounts: readonly Account[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ul className="people__list">
      {props.accounts.map((account) => (
        <li key={account.id} className="people__person">
          <Link to={{ screen: "person", userId: account.id }} className="people__name">
            {account.name}
          </Link>{" "}
          <span className="people__status">{registrationText(t, account)}</span>
        </li>
      ))}
    </ul>
  );
}
