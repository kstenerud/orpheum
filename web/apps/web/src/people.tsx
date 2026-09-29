// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  createAccount,
  listAccounts,
  registrationUrl,
  type Account,
  type Invitation,
} from "./accounts.ts";
import { problemText } from "./problem.ts";
import { Link } from "./link.tsx";
import { QrCode } from "./qr-code.tsx";
import { Loaded } from "./title.tsx";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { useResource } from "./use-resource.ts";

/** What the admin last made, to pass on: a person's link or a device's code. */
export type Made =
  | { readonly what: "link"; readonly name: string; readonly invitation: Invitation }
  | { readonly what: "code"; readonly name: string; readonly invitation: Invitation };

/**
 * The household, for the admin (D218): everyone, whether they have registered, a way to add a
 * person, and for each a new registration link or a code for a device.
 */
export function People(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [accounts, reload] = useResource("accounts", () => listAccounts(api));
  const [made, setMade] = useState<Made | undefined>(undefined);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const failed = (cause: unknown): void => {
    setProblem(problemText(t, cause, { conflict: t("people.nameTaken") }));
  };
  return (
    <section className="people" aria-labelledby="people-heading">
      <h1 id="people-heading">{t("people.heading")}</h1>
      <AddPerson
        onMade={(name, invitation) => {
          setMade({ what: "link", name, invitation });
          reload();
        }}
        onFailed={failed}
      />
      {made === undefined ? null : <MadeView made={made} />}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <Loaded resource={accounts} render={(list) => <PeopleList accounts={list} />} />
    </section>
  );
}

/** What was just made, to pass on: a link also as a QR code a phone's camera opens (D218 §1). */
export function MadeView(props: { readonly made: Made }): JSX.Element {
  const { t } = useTranslation();
  const { made } = props;
  const text =
    made.what === "link"
      ? registrationUrl(window.location.origin, made.invitation.secret)
      : made.invitation.secret;
  return (
    <div className="people__made">
      <h2>{t(`people.made.${made.what}`, { name: made.name })}</h2>
      {made.what === "link" ? (
        <QrCode text={text} label={t("people.qrLabel", { name: made.name })} />
      ) : null}
      <Share what={made.what} text={text} invitation={made.invitation} />
    </div>
  );
}

/** Adds a person: their name and role, and their registration link to pass on. */
function AddPerson(props: {
  readonly onMade: (name: string, invitation: Invitation) => void;
  readonly onFailed: (cause: unknown) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [name, setName] = useState("");
  const [role, setRole] = useState<"admin" | "user">("user");
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    createAccount(api, name, role).then(({ registrationInvitation: registration }) => {
      props.onMade(name, registration);
      setName("");
    }, props.onFailed);
  };
  return (
    <form className="people__add" onSubmit={submit}>
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
    </form>
  );
}

/** Everyone, each leading to their page (the owner, 2026-09-27). */
export function PeopleList(props: { readonly accounts: readonly Account[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ul className="people__list">
      {props.accounts.map((account) => (
        <li key={account.id} className="people__person">
          <Link to={{ screen: "person", id: account.id }} className="people__name">
            {account.name}
          </Link>{" "}
          <span className="people__status">
            {t(account.hasPassword ? "people.registered" : "people.waiting")}
          </span>
        </li>
      ))}
    </ul>
  );
}
