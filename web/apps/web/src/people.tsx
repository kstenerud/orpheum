// SPDX-License-Identifier: MPL-2.0

import "./people.css";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import {
  anotherInvitationLink,
  createAccount,
  invite,
  listAccounts,
  listInvitations,
  registrationUrl,
  withdrawInvitation,
  type Account,
  type Invitation,
  type NewAccount,
} from "./accounts.ts";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { listRoles, roleLabel, type Role } from "./roles.ts";
import { NoLibraryNote, RolesSection } from "./roles-section.tsx";
import { FormRow } from "./form-row.tsx";
import { fieldProblemText, type FormWords } from "./problem.ts";
import { Link } from "./link.tsx";
import { QrCode } from "./qr-code.tsx";
import { Loaded } from "./loaded.tsx";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { BusyGroup } from "./busy-group.tsx";
import { useRequest, type RequestState } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** What the admin last issued, to pass on: a person's link or a device's code. */
export interface IssuedInvitation {
  readonly name: string;
  readonly invitation: components["schemas"]["IssuedSecretBody"];
}

/**
 * The household, for the admin (D218, D246 §2): everyone, with the admin's note of who each is for;
 * an account set up in full, or an invitation for someone to fill in their own, under a role and
 * with that note; the invitations waiting, each with its live links to pass on again (D267 §5, as
 * the owner chose for invitations, 2026-10-28), sent again or withdrawn, and those used, with the
 * person each made (D311 §3, §4; D318).
 */
export function People(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [accounts, reload] = useResource("accounts", () => listAccounts(api));
  const [invitations, reloadInvitations] = useResource("invitations", () => listInvitations(api));
  const [roles, reloadRoles] = useResource("people-roles", () => listRoles(api));
  const roleList = roles.state === "loaded" ? roles.value.roles : undefined;
  const roleNames = roleList?.map((role) => role.name) ?? ["user"];
  const [issued, setIssued] = useState<IssuedInvitation | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, personWords(t)),
  );
  const invitationActions = useInvitationActions(runRequest, setIssued, reloadInvitations);
  return (
    <section className="people" aria-labelledby="people-heading">
      <h1 id="people-heading">{t("people.heading")}</h1>
      <BusyGroup isBusy={isBusy}>
        <InvitePerson
          roleNames={roleNames}
          roles={roleList}
          onInvite={(role, adminNote) => {
            runRequest(async (idempotencyKey) => {
              const { link } = await invite(api, { role, adminNote }, idempotencyKey);
              setIssued({ name: adminNote, invitation: link });
              reloadInvitations();
            });
          }}
        />
        <SetUpAccount
          roleNames={roleNames}
          roles={roleList}
          onCreate={(account) => {
            runRequest(async (idempotencyKey) => {
              await createAccount(api, account, idempotencyKey);
              setIssued(undefined);
              reload();
            });
          }}
        />
      </BusyGroup>
      {issued === undefined ? null : <IssuedInvitationView issued={issued} />}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <Loaded
        resource={invitations}
        render={(list) => <Invitations invitations={list} {...invitationActions} />}
      />
      <Loaded resource={accounts} render={(list) => <PeopleList accounts={list} />} />
      <RolesSection onChange={reloadRoles} />
    </section>
  );
}

/**
 * Sending a waiting invitation another link, which shows it to pass on, and withdrawing one; the
 * list is read again after each.
 */
function useInvitationActions(
  runRequest: RequestState["runRequest"],
  setIssued: (issued: IssuedInvitation) => void,
  reloadInvitations: () => void,
): Pick<InvitationsProps, "onSendAgain" | "onWithdraw"> {
  const { api } = useServices();
  return {
    onSendAgain: (invitation) => {
      runRequest(async (idempotencyKey) => {
        const link = await anotherInvitationLink(api, invitation.id, idempotencyKey);
        setIssued({ name: invitation.adminNote, invitation: link });
        reloadInvitations();
      });
    },
    onWithdraw: (invitationId) => {
      runRequest(async () => {
        await withdrawInvitation(api, invitationId);
        reloadInvitations();
      });
    },
  };
}

/** What was just issued, to pass on: a link also as a QR code a phone's camera opens (D218 §1). */
export function IssuedInvitationView(props: { readonly issued: IssuedInvitation }): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const { issued } = props;
  const text = registrationUrl(navigation.origin(), issued.invitation);
  return (
    <div className="people__issued">
      <h2>{t("people.issued", { name: issued.name })}</h2>
      <QrCode text={text} label={t("people.qrLabel", { name: issued.name })} />
      <Share text={text} invitation={issued.invitation} />
    </div>
  );
}

/** A role chooser over the roles there are, saying when the one chosen reaches no library. */
function RoleSelect(props: {
  readonly roleNames: readonly string[];
  readonly roles: readonly Role[] | undefined;
  readonly role: string;
  readonly onChange: (role: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <label>
        {t("people.role")}{" "}
        <select
          value={props.role}
          onChange={(event) => {
            props.onChange(event.target.value);
          }}
        >
          {props.roleNames.map((roleName) => (
            <option key={roleName} value={roleName}>
              {roleLabel(t, roleName)}
            </option>
          ))}
        </select>
      </label>
      <NoLibraryNote roles={props.roles} roleName={props.role} />
    </>
  );
}

/**
 * Invites someone to make their own account: the role it will have, and the admin's note of who
 * it is for, which the admin must write and only the admin sees (D246 §2); the page then shows the
 * link to pass on.
 */
function InvitePerson(props: {
  readonly roleNames: readonly string[];
  readonly roles: readonly Role[] | undefined;
  readonly onInvite: (role: string, adminNote: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [role, setRole] = useState("user");
  const [adminNote, setAdminNote] = useState("");
  return (
    <FormRow
      onSubmit={() => {
        props.onInvite(role, adminNote);
      }}
    >
      <h2>{t("people.invite")}</h2>
      <p>{t("people.inviteExplain")}</p>
      <label>
        {t("people.adminNote")}{" "}
        <input
          required
          maxLength={200}
          placeholder={t("people.adminNoteExample")}
          value={adminNote}
          onChange={(event) => {
            setAdminNote(event.target.value);
          }}
        />
      </label>
      <RoleSelect roleNames={props.roleNames} roles={props.roles} role={role} onChange={setRole} />
      <button type="submit">{t("people.makeLink")}</button>
    </FormRow>
  );
}

/** Sets an account up in full: a name, a password, a role, and the admin's note (D246 §2). */
function SetUpAccount(props: {
  readonly roleNames: readonly string[];
  readonly roles: readonly Role[] | undefined;
  readonly onCreate: (account: NewAccount) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("user");
  const [adminNote, setAdminNote] = useState("");
  return (
    <FormRow
      onSubmit={() => {
        props.onCreate({ name, password, role, adminNote });
      }}
    >
      <h2>{t("people.setUp")}</h2>
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
        {t("people.password")}{" "}
        <input
          required
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(event) => {
            setPassword(event.target.value);
          }}
        />
      </label>
      <RoleSelect roleNames={props.roleNames} roles={props.roles} role={role} onChange={setRole} />
      <label>
        {t("people.adminNote")}{" "}
        <input
          maxLength={200}
          value={adminNote}
          onChange={(event) => {
            setAdminNote(event.target.value);
          }}
        />
      </label>
      <button type="submit">{t("people.create")}</button>
    </FormRow>
  );
}

/** The invitations, and what the admin does with one still waiting. */
interface InvitationsProps {
  readonly invitations: readonly Invitation[];
  readonly onSendAgain: (invitation: Invitation) => void;
  readonly onWithdraw: (invitationId: number) => void;
}

/**
 * The invitations: those waiting, each with who it is for, its role and until when its links work,
 * sent again by another link, which leaves the others live, or withdrawn; and those used, each with
 * the person it made (D311 §3, §4; D318).
 */
function Invitations(props: InvitationsProps): JSX.Element {
  const waiting = props.invitations.filter((invitation) => invitation.made === undefined);
  const used = props.invitations.filter((invitation) => invitation.made !== undefined);
  return (
    <>
      <WaitingInvitations
        invitations={waiting}
        onSendAgain={props.onSendAgain}
        onWithdraw={props.onWithdraw}
      />
      <UsedInvitations invitations={used} />
    </>
  );
}

/** The invitations waiting, each sent again or withdrawn; nothing when there are none. */
function WaitingInvitations(props: {
  readonly invitations: readonly Invitation[];
  readonly onSendAgain: (invitation: Invitation) => void;
  readonly onWithdraw: (invitationId: number) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.invitations.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="people-invitations">
      <h2 id="people-invitations">{t("people.invitations")}</h2>
      <ul>
        {props.invitations.map((invitation) => (
          <li key={invitation.id}>
            {invitation.liveLinks > 0
              ? t("people.invitation", {
                  note: invitation.adminNote,
                  role: roleLabel(t, invitation.role),
                  until: dateTimeText(invitation.expiresAt, i18n.language),
                })
              : t("people.invitationEnded", {
                  note: invitation.adminNote,
                  role: roleLabel(t, invitation.role),
                })}{" "}
            <button
              type="button"
              onClick={() => {
                props.onSendAgain(invitation);
              }}
            >
              {t("people.sendAgain")}
            </button>{" "}
            <ConfirmButton
              label={t("people.withdraw")}
              consequence={t("people.withdrawConsequence")}
              confirmLabel={t("people.withdrawConfirm")}
              onConfirm={() => {
                props.onWithdraw(invitation.id);
              }}
            />
            <InvitationLinks links={invitation.links ?? []} />
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * A waiting invitation's live links, each shown again to pass on, with Copy, until it is used,
 * withdrawn or ends (D267 §5, as the owner chose for invitations, 2026-10-28); nothing when it has
 * none.
 */
function InvitationLinks(props: {
  readonly links: readonly components["schemas"]["IssuedSecretBody"][];
}): JSX.Element | null {
  const { navigation } = useServices();
  if (props.links.length === 0) {
    return null;
  }
  return (
    <ul className="people__links">
      {props.links.map((link) => (
        <li key={link.secret}>
          <Share text={registrationUrl(navigation.origin(), link)} invitation={link} />
        </li>
      ))}
    </ul>
  );
}

/** The invitations used, each with the person it made and when; nothing when there are none. */
function UsedInvitations(props: {
  readonly invitations: readonly Invitation[];
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.invitations.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="people-used-invitations">
      <h2 id="people-used-invitations">{t("people.usedInvitations")}</h2>
      <ul>
        {props.invitations.map((invitation) =>
          invitation.made === undefined ? null : (
            <li key={invitation.id}>
              {t("people.usedInvitation", {
                note: invitation.adminNote,
                at: dateTimeText(invitation.made.at, i18n.language),
              })}{" "}
              <Link to={{ screen: "person", userId: invitation.made.userId }}>
                {invitation.made.name}
              </Link>
            </li>
          ),
        )}
      </ul>
    </section>
  );
}

/** Whether a person has set their password yet, in words. */
/** How a refusal of a person's name or role is told, on the People page and a person's own. */
export function personWords(t: TFunction): FormWords {
  return {
    fields: {
      "body.name": t("people.name"),
      "body.role": t("people.role"),
      "body.adminNote": t("people.adminNote"),
      "body.password": t("people.password"),
    },
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
          {account.adminNote === undefined ? null : (
            <span className="people__note">{t("people.noted", { note: account.adminNote })}</span>
          )}{" "}
          <span className="people__status">{registrationText(t, account)}</span>
        </li>
      ))}
    </ul>
  );
}
