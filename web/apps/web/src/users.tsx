// SPDX-License-Identifier: MPL-2.0

import "./people-words.ts";

import "./users.css";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  listAccounts,
  listInvitations,
  registrationUrl,
  type Account,
  type Invitation,
} from "./accounts.ts";
import { GridList } from "./grid-list.tsx";
import { InvitationDialog, InvitationsSection } from "./invitations-section.tsx";
import { Link } from "./link.tsx";
import { Loaded } from "./loaded.tsx";
import { CreateAccountDialog, InviteDialog, type RoleChoices } from "./people-dialogs.tsx";
import { QrCode } from "./qr-code.tsx";
import { listRoles, roleLabel } from "./roles.ts";
import { RoleDialog } from "./role-dialog.tsx";
import { RolesSection } from "./roles-section.tsx";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { useResource } from "./use-resource.ts";

/** What the admin last issued, to pass on: a person's link or a device's code. */
export interface IssuedInvitation {
  readonly name: string;
  readonly invitation: components["schemas"]["IssuedSecretBody"];
  /** A registered person's reset password link (D466), headed as one. */
  readonly isReset?: boolean;
}

/**
 * The dialog the page has open, if any: one of the three that make something, a role's (a new one
 * with no roleId, else that role) or an invitation's.
 */
type DialogState =
  | { readonly kind: "account" | "inviting" }
  | { readonly kind: "role"; readonly roleId?: number }
  | {
      readonly kind: "invitation";
      readonly invitation: Invitation;
      readonly link: components["schemas"]["IssuedSecretBody"] | undefined;
    };

/** The three things the buttons make. */
const makers = [
  { kind: "role", label: "people.createRole" },
  { kind: "account", label: "people.createAccount" },
  { kind: "inviting", label: "people.inviteSomeone" },
] as const;

/** The buttons above the sections, each opening the dialog that makes one thing. */
function Makers(props: { readonly onOpen: (open: DialogState) => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="people__actions">
      {makers.map((maker) => (
        <button
          key={maker.kind}
          type="button"
          onClick={() => {
            props.onOpen({ kind: maker.kind });
          }}
        >
          {t(maker.label)}
        </button>
      ))}
    </div>
  );
}

/**
 * The household, for the admin (D399, revising D218 and D246 §2): three buttons, Create Role,
 * Create Account and Invite Someone, each opening a dialog with its controls; then three sections:
 * the invitations, each opening its dialog; the accounts, each leading to the person's page; and
 * the roles, each leading to the role's page. A new invitation opens its own dialog at once, with
 * the link to pass on; a new account or role opens nothing.
 */
export function Users(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [accounts, reloadAccounts] = useResource("accounts", () => listAccounts(api));
  const [invitations, reloadInvitations] = useResource("invitations", () => listInvitations(api));
  const [roles, reloadRoles] = useResource("people-roles", () => listRoles(api));
  const roleList = roles.state === "loaded" ? roles.value.roles : undefined;
  const choices = { roleNames: roleList?.map((role) => role.name) ?? ["user"], roles: roleList };
  const [open, setOpen] = useState<DialogState | undefined>(undefined);
  const close = (): void => {
    setOpen(undefined);
  };
  return (
    <section className="people" aria-labelledby="people-heading">
      <h1 id="people-heading">{t("people.heading")}</h1>
      <Makers onOpen={setOpen} />
      <Loaded
        resource={invitations}
        render={(list) => (
          <InvitationsSection
            invitations={list}
            onOpen={(invitation) => {
              setOpen({ kind: "invitation", invitation, link: undefined });
            }}
          />
        )}
      />
      <section aria-labelledby="people-accounts">
        <h2 id="people-accounts">{t("people.accounts")}</h2>
        <Loaded resource={accounts} render={(list) => <UsersList accounts={list} />} />
      </section>
      <Loaded
        resource={roles}
        render={(listed) => (
          <RolesSection
            roles={listed.roles}
            onOpen={(role) => {
              setOpen({ kind: "role", roleId: role.id });
            }}
          />
        )}
      />
      <OpenDialog
        open={open}
        choices={choices}
        onClose={close}
        onOpen={setOpen}
        onChanged={{ reloadAccounts, reloadInvitations, reloadRoles }}
      />
    </section>
  );
}

/** The dialog the page has open, if any, and what the page reloads once it has changed something. */
function OpenDialog(props: {
  readonly open: DialogState | undefined;
  readonly choices: RoleChoices;
  readonly onClose: () => void;
  readonly onOpen: (open: DialogState) => void;
  readonly onChanged: {
    readonly reloadAccounts: () => void;
    readonly reloadInvitations: () => void;
    readonly reloadRoles: () => void;
  };
}): JSX.Element {
  return (
    <>
      {props.open?.kind === "role" ? (
        <RoleDialog
          roleId={props.open.roleId}
          onClose={() => {
            props.onClose();
            props.onChanged.reloadRoles();
          }}
        />
      ) : null}
      {props.open?.kind === "account" ? (
        <CreateAccountDialog
          choices={props.choices}
          onClose={props.onClose}
          onCreated={() => {
            props.onClose();
            props.onChanged.reloadAccounts();
            props.onChanged.reloadRoles();
          }}
        />
      ) : null}
      {props.open?.kind === "inviting" ? (
        <InviteDialog
          choices={props.choices}
          onClose={props.onClose}
          onInvited={(invitation, link) => {
            props.onOpen({ kind: "invitation", invitation, link });
            props.onChanged.reloadInvitations();
          }}
        />
      ) : null}
      {props.open?.kind === "invitation" ? (
        <InvitationDialog
          invitation={props.open.invitation}
          link={props.open.link}
          onClose={props.onClose}
          onWithdrawn={() => {
            props.onClose();
            props.onChanged.reloadInvitations();
          }}
        />
      ) : null}
    </>
  );
}

/**
 * What was just issued, to pass on: a link also as a QR code a phone's camera opens (D218 §1). A
 * reset link is shown here once, when made, and never again (D466 §4).
 */
export function IssuedInvitationView(props: { readonly issued: IssuedInvitation }): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const { issued } = props;
  const text = registrationUrl(navigation.origin(), issued.invitation);
  return (
    <div className="people__issued">
      <h2>
        {t(issued.isReset === true ? "people.issuedReset" : "people.issued", { name: issued.name })}
      </h2>
      <QrCode text={text} label={t("people.qrLabel", { name: issued.name })} />
      <Share text={text} invitation={issued.invitation} />
    </div>
  );
}

/** Everyone, each leading to their page (the owner, 2026-09-27): name, the admin's note and role. */
export function UsersList(props: { readonly accounts: readonly Account[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <GridList
      rows={props.accounts}
      rowKey={(account) => String(account.id)}
      columns={[
        { key: "name", text: "truncate", cell: (account) => account.name },
        { key: "note", text: "wrap", cell: (account) => account.adminNote },
        { key: "role", text: "fit", cell: (account) => roleLabel(t, account.role) },
      ]}
      target={(account, drawn) => <Link to={{ screen: "user", userId: account.id }} {...drawn} />}
      isArrival
    />
  );
}
