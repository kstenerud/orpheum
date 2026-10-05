// SPDX-License-Identifier: MPL-2.0

import "./people.css";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { createAccount, listAccounts, registrationUrl, type Account } from "./accounts.ts";
import { listRoles, roleLabel } from "./roles.ts";
import { RolesSection } from "./roles-section.tsx";
import { FormRow } from "./form-row.tsx";
import { fieldProblemText, type FormWords } from "./problem.ts";
import { Link } from "./link.tsx";
import { QrCode } from "./qr-code.tsx";
import { Loaded } from "./title.tsx";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { BusyGroup } from "./busy-group.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** What the admin last issued, to pass on: a person's link or a device's code. */
export type IssuedInvitation =
  | {
      readonly kind: "link";
      readonly name: string;
      readonly invitation: components["schemas"]["IssuedSecretBody"];
    }
  | {
      readonly kind: "code";
      readonly name: string;
      readonly invitation: components["schemas"]["IssuedSecretBody"];
    };

/**
 * The household, for the admin (D218): everyone, whether they have registered, a way to add a
 * person, and for each a new registration invitation or a code for a device.
 */
export function People(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [accounts, reload] = useResource("accounts", () => listAccounts(api));
  const [roles, reloadRoles] = useResource("people-roles", () => listRoles(api));
  const roleNames =
    roles.state === "loaded" ? roles.value.roles.map((role) => role.name) : ["user"];
  const [issued, setIssued] = useState<IssuedInvitation | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, personWords(t)),
  );
  return (
    <section className="people" aria-labelledby="people-heading">
      <h1 id="people-heading">{t("people.heading")}</h1>
      <BusyGroup isBusy={isBusy}>
        <AddPerson
          roleNames={roleNames}
          onAdd={(name, role, isOutsider) => {
            runRequest(async (idempotencyKey) => {
              const { registrationInvitation } = await createAccount(
                api,
                { name, role, isOutsider },
                idempotencyKey,
              );
              setIssued({ kind: "link", name, invitation: registrationInvitation });
              reload();
            });
          }}
        />
      </BusyGroup>
      {issued === undefined ? null : <IssuedInvitationView issued={issued} />}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <Loaded resource={accounts} render={(list) => <PeopleList accounts={list} />} />
      <RolesSection onChange={reloadRoles} />
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

/**
 * Adds a person: their name, their role, which is their whole access, and whether they are from
 * outside the house, an invitee (D243 §4); the page shows their registration link to pass on.
 */
function AddPerson(props: {
  readonly roleNames: readonly string[];
  readonly onAdd: (name: string, role: string, isOutsider: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [role, setRole] = useState("user");
  const [isOutsider, setIsOutsider] = useState(false);
  const submit = (): void => {
    props.onAdd(name, role, isOutsider);
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
            setRole(event.target.value);
          }}
        >
          {props.roleNames.map((roleName) => (
            <option key={roleName} value={roleName}>
              {roleLabel(t, roleName)}
            </option>
          ))}
        </select>
      </label>
      <label>
        <input
          type="checkbox"
          checked={isOutsider}
          onChange={() => {
            setIsOutsider(!isOutsider);
          }}
        />{" "}
        {t("people.outsider")}
      </label>
      <button type="submit">{t("people.create")}</button>
    </FormRow>
  );
}

/** Whether a person has set their password yet, in words. */
/** How a refusal of a person's name or role is told, on the People page and a person's own. */
export function personWords(t: TFunction): FormWords {
  return {
    fields: { "body.name": t("people.name"), "body.role": t("people.role") },
    overrides: { conflict: t("people.nameTaken") },
  };
}

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
