// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { registrationUrl, withdrawInvitation, type Invitation } from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { personWords } from "./person-words.ts";
import { fieldProblemText } from "./problem.ts";
import { QrCode } from "./qr-code.tsx";
import { roleLabel } from "./roles.ts";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { useRequest } from "./use-request.ts";

/**
 * The invitations still waiting (D399, revising D218 and D267 §5): a row each, with the admin's note
 * of who it is for, its role and when it was issued. A row opens its dialog; nothing else on the
 * page sends or withdraws one. One used is not listed, its account is among the accounts (D403).
 * Nothing when there are none.
 */
export function InvitationsSection(props: {
  readonly invitations: readonly Invitation[];
  readonly onOpen: (invitation: Invitation) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.invitations.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="people-invitations">
      <h2 id="people-invitations">{t("people.invitations")}</h2>
      <ul className="people__list">
        {props.invitations.map((invitation) => (
          <li key={invitation.id}>
            <button
              type="button"
              className="people__row"
              onClick={() => {
                props.onOpen(invitation);
              }}
            >
              <span className="people__name">{invitation.adminNote}</span>
              <span>{roleLabel(t, invitation.role)}</span>
              <span>
                {t("people.issuedOn", { at: dateTimeText(invitation.issuedAt, i18n.language) })}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * One waiting invitation's dialog: who it is for, its role and when it was issued, its link as a QR
 * code a phone's camera opens (D218 §1) and as text with Copy, and Withdraw. link is the one just made,
 * where the dialog opens at the invitation's creation; otherwise the live links it has, which an
 * invitation sent another link before D399 may have more than one of.
 */
export function InvitationDialog(props: {
  readonly invitation: Invitation;
  readonly link: components["schemas"]["IssuedSecretBody"] | undefined;
  readonly onClose: () => void;
  readonly onWithdrawn: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { invitation } = props;
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, personWords(t)),
  );
  const links = linksToShow(invitation, props.link);
  return (
    <ModalDialog
      label={t("people.invitationFor", { note: invitation.adminNote })}
      onClose={props.onClose}
    >
      <h2>{t("people.invitationFor", { note: invitation.adminNote })}</h2>
      <dl className="people__details">
        <dt>{t("people.role")}</dt>
        <dd>{roleLabel(t, invitation.role)}</dd>
        <dt>{t("people.issuedLabel")}</dt>
        <dd>{dateTimeText(invitation.issuedAt, i18n.language)}</dd>
      </dl>
      <WaitingInvitation
        invitation={invitation}
        links={links}
        isBusy={isBusy}
        onWithdraw={() => {
          runRequest(async () => {
            await withdrawInvitation(api, invitation.id);
            props.onWithdrawn();
          });
        }}
      />
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </ModalDialog>
  );
}

/** The links a waiting invitation's dialog shows: the one just made, else those still live. */
function linksToShow(
  invitation: Invitation,
  made: components["schemas"]["IssuedSecretBody"] | undefined,
): readonly components["schemas"]["IssuedSecretBody"][] {
  if (made !== undefined) {
    return [made];
  }
  return invitation.liveLinks > 0 ? (invitation.links ?? []) : [];
}

/** What a waiting invitation shows: each live link, and Withdraw. */
function WaitingInvitation(props: {
  readonly invitation: Invitation;
  readonly links: readonly components["schemas"]["IssuedSecretBody"][];
  readonly isBusy: boolean;
  readonly onWithdraw: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const { invitation, links } = props;
  return (
    <>
      {links.length === 0 ? <p>{t("people.invitationEnded")}</p> : null}
      {links.map((link) => {
        const text = registrationUrl(navigation.origin(), link);
        return (
          <div key={link.secret}>
            <QrCode text={text} label={t("people.qrLabel", { name: invitation.adminNote })} />
            <Share text={text} invitation={link} />
          </div>
        );
      })}
      <BusyGroup isBusy={props.isBusy}>
        <ConfirmButton
          label={t("people.withdraw")}
          consequence={t("people.withdrawConsequence")}
          confirmLabel={t("people.withdrawConfirm")}
          onConfirm={props.onWithdraw}
        />
      </BusyGroup>
    </>
  );
}
