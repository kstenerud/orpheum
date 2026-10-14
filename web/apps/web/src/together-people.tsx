// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { FormRow } from "./form-row.tsx";
import { idempotent } from "./idempotent.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow, throwIfRefused } from "./problem.ts";
import { QrCode } from "./qr-code.tsx";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { PersonVoiceVolume, useHostVoice } from "./together-voice.tsx";

/** A session as its host sees it: its links and members, with the host's notes. */
export type SessionDetails = components["schemas"]["TogetherDetailsOutputBody"];

/** A member as the session's details list them. */
type SessionMember = NonNullable<SessionDetails["members"]>[number];

/** A session link as the host's view lists it. */
type SessionLink = NonNullable<SessionDetails["links"]>[number];

/** What a guest may do to playback, as a link gives it (D271 §4). */
export type Rights = SessionDetails["nextRights"];

/** A link to pass on, and the note of whom it is for. */
export interface Issued {
  readonly note: string;
  readonly secret: string;
  /** The link in the discovery's form, which the server names while a discovery has claimed it. */
  readonly link?: string | undefined;
}

/** An answer the server may have refused, as throwIfRefused reads it. */
type Answer = Parameters<typeof throwIfRefused>[0];

/**
 * The link a person joins by: the session's page, the secret after # so no request carries it (D243
 * §3). While a discovery has claimed the server, the server names it in the discovery's form, which
 * reaches it from anywhere (the discovery protocol's §3.1), and that one is passed on.
 */
export function togetherUrl(origin: string, issued: Pick<Issued, "secret" | "link">): string {
  return issued.link ?? `${origin}/together#${issued.secret}`;
}

/** What rights give, in a few words for the host's list of people. */
function rightsText(t: (key: string) => string, rights: Rights): string {
  if (rights.canPlayPause && rights.canSeek) {
    return t("watchTogether.rights.both");
  }
  if (rights.canPlayPause) {
    return t("watchTogether.rights.playPause");
  }
  return t(rights.canSeek ? "watchTogether.rights.seek" : "watchTogether.rights.none");
}

/**
 * A dialog over the session's controls, its own heading and Close (D281 §5): what is done to one
 * person, or a link made, opened from the list and gone once done.
 */
export function ActionDialog(props: {
  readonly heading: string;
  readonly onClose: () => void;
  /** Whether it opens with its first field focused (ModalDialog). */
  readonly shouldFocusFirstField?: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ModalDialog
      className="watch-together__dialog"
      label={props.heading}
      onClose={props.onClose}
      shouldFocusFirstField={props.shouldFocusFirstField === true}
    >
      <div className="watch-together__dialog-head">
        <h2>{props.heading}</h2>
        <button type="button" onClick={props.onClose}>
          {t("status.close")}
        </button>
      </div>
      {props.children}
    </ModalDialog>
  );
}

/**
 * Making one person's link: the host's note of who it is for, which only the host sees, and what
 * they may do to playback, starting with what the last link gave (D271 §4).
 */
export function MakeLink(props: {
  readonly nextRights: Rights;
  readonly onMake: (hostNote: string, rights: Rights) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [hostNote, setHostNote] = useState("");
  const [rights, setRights] = useState(props.nextRights);
  return (
    <FormRow
      onSubmit={() => {
        props.onMake(hostNote, rights);
      }}
    >
      <label>
        {t("watchTogether.note")}{" "}
        <input
          required
          maxLength={200}
          placeholder={t("watchTogether.noteExample")}
          value={hostNote}
          onChange={(event) => {
            setHostNote(event.target.value);
          }}
        />
      </label>
      <label>
        <input
          type="checkbox"
          checked={rights.canPlayPause}
          onChange={(event) => {
            setRights({ ...rights, canPlayPause: event.target.checked });
          }}
        />{" "}
        {t("watchTogether.mayPlayPause")}
      </label>
      <label>
        <input
          type="checkbox"
          checked={rights.canSeek}
          onChange={(event) => {
            setRights({ ...rights, canSeek: event.target.checked });
          }}
        />{" "}
        {t("watchTogether.maySeek")}
      </label>
      <button type="submit">{t("watchTogether.makeLink")}</button>
    </FormRow>
  );
}

/** The link just made or shown again, to pass on: as text and as a QR code a phone's camera opens. */
export function IssuedLink(props: Issued): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const url = togetherUrl(navigation.origin(), props);
  return (
    <div className="watch-together__issued">
      <QrCode text={url} label={t("watchTogether.qrLabel", { note: props.note })} />
      <Share text={url} invitation={undefined} />
    </div>
  );
}

/** What the people's list and its dialogs act with: the session, and how a change is sent and told. */
export interface PeopleActions {
  readonly sessionId: number;
  readonly run: (request: (idempotencyKey: string) => Promise<void>) => void;
  readonly onChanged: () => void;
  /** Told a link to pass on: shown again (D267 §5), or new, which cut off its member (§4). */
  readonly onIssued: (issued: Issued) => void;
}

/** Sends request, then has the session read again. */
function sendThenReload(
  actions: PeopleActions,
  request: (idempotencyKey: string) => Promise<Answer>,
): void {
  actions.run(async (idempotencyKey) => {
    throwIfRefused(await request(idempotencyKey));
    actions.onChanged();
  });
}

/**
 * Gives member a new link: someone moving to another device, not signed in, comes back by one. They
 * are removed, cutting off every device they came in on, and the link admits them again under the
 * same note, with the rights the old one gave (D267 §4, D271 §4). Returns the link to pass on.
 */
async function relink(
  api: ReturnType<typeof useServices>["api"],
  member: SessionMember & { readonly sessionId: number; readonly rights: Rights | undefined },
  idempotencyKey: string,
): Promise<Issued> {
  const { sessionId: id, rights } = member;
  const hostNote = member.hostNote ?? member.name;
  throwIfRefused(
    await api.POST("/together/{id}/eject/{member}", {
      params: { path: { id, member: member.id } },
      ...idempotent(`${idempotencyKey}-eject`),
    }),
  );
  const made = dataOrThrow(
    await api.POST("/together/{id}/links", {
      params: { path: { id } },
      body: rights === undefined ? { hostNote } : { hostNote, rights },
      ...idempotent(`${idempotencyKey}-link`),
    }),
  );
  return { note: hostNote, secret: made.secret };
}

/** A new link for a member, cutting off the devices they came in on (D267 §4). */
function RelinkButton(
  props: PeopleActions & {
    readonly member: SessionMember;
    readonly joinedBy: SessionLink | undefined;
  },
): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { member, joinedBy, sessionId } = props;
  return (
    <button
      type="button"
      onClick={() => {
        props.run(async (idempotencyKey) => {
          props.onIssued(
            await relink(api, { ...member, sessionId, rights: joinedBy?.rights }, idempotencyKey),
          );
          props.onChanged();
        });
      }}
    >
      {t("watchTogether.relink")}
    </button>
  );
}

/**
 * What the host does with one member (D281 §5): how loud their voice plays, the link they joined
 * by to show again, which brings them back on the browser that joined (D267 §5), a new link (§4),
 * or removing them.
 */
function PersonDialog(
  props: PeopleActions & {
    readonly member: SessionMember;
    readonly joinedBy: SessionLink | undefined;
    readonly onClose: () => void;
  },
): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const voice = useHostVoice();
  const { member, joinedBy, sessionId } = props;
  return (
    <ActionDialog heading={member.name} onClose={props.onClose}>
      <p>
        {member.hostNote ?? ""}
        {joinedBy === undefined ? null : `, ${rightsText(t, joinedBy.rights)}`}
      </p>
      {voice === undefined ? null : (
        <PersonVoiceVolume {...voice} memberId={member.id} name={member.name} />
      )}
      <div className="watch-together__actions">
        {joinedBy === undefined ? null : (
          <button
            type="button"
            onClick={() => {
              props.onIssued({ note: joinedBy.hostNote, secret: joinedBy.secret });
            }}
          >
            {t("watchTogether.showLink")}
          </button>
        )}
        <RelinkButton {...props} />
        <ConfirmButton
          label={t("watchTogether.eject")}
          consequence={t("watchTogether.ejectConsequence", { name: member.name })}
          confirmLabel={t("watchTogether.ejectConfirm")}
          onConfirm={() => {
            props.onClose();
            sendThenReload(props, (idempotencyKey) =>
              api.POST("/together/{id}/eject/{member}", {
                params: { path: { id: sessionId, member: member.id } },
                ...idempotent(idempotencyKey),
              }),
            );
          }}
        />
      </div>
    </ActionDialog>
  );
}

/** What the host does with a link not used yet: show it again, or withdraw it. */
function LinkDialog(
  props: PeopleActions & { readonly link: SessionLink; readonly onClose: () => void },
): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { link } = props;
  return (
    <ActionDialog
      heading={t("watchTogether.waitingLink", { note: link.hostNote })}
      onClose={props.onClose}
    >
      <p>{rightsText(t, link.rights)}</p>
      <div className="watch-together__actions">
        <button
          type="button"
          onClick={() => {
            props.onIssued({ note: link.hostNote, secret: link.secret, link: link.link });
          }}
        >
          {t("watchTogether.showLink")}
        </button>
        <ConfirmButton
          label={t("watchTogether.withdraw")}
          consequence={t("watchTogether.withdrawConsequence")}
          confirmLabel={t("watchTogether.withdrawConfirm")}
          onConfirm={() => {
            props.onClose();
            sendThenReload(props, () =>
              api.DELETE("/together/{id}/links/{link}", {
                params: { path: { id: props.sessionId, link: link.id } },
              }),
            );
          }}
        />
      </div>
    </ActionDialog>
  );
}

/** Which row of the people's list has its dialog open. */
type Opened = { readonly memberId: number } | { readonly linkId: number } | undefined;

/** The people's rows: each member, then each link not used yet, opening its dialog when pressed. */
function PeopleList(props: {
  readonly members: readonly SessionMember[];
  readonly links: readonly SessionLink[];
  readonly allLinks: readonly SessionLink[];
  readonly onOpen: (opened: Opened) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ul className="watch-together__people">
      {props.members.map((member) => {
        const joinedBy = props.allLinks.find((link) => link.usedByMemberId === member.id);
        return (
          <li key={`member-${String(member.id)}`}>
            <button
              type="button"
              onClick={() => {
                props.onOpen({ memberId: member.id });
              }}
            >
              {t("watchTogether.member", { name: member.name, note: member.hostNote ?? "" })}
              {joinedBy === undefined ? null : `, ${rightsText(t, joinedBy.rights)}`}
            </button>
          </li>
        );
      })}
      {props.links.map((link) => (
        <li key={`link-${String(link.id)}`}>
          <button
            type="button"
            onClick={() => {
              props.onOpen({ linkId: link.id });
            }}
          >
            {t("watchTogether.waitingLink", { note: link.hostNote })}, {rightsText(t, link.rights)}
          </button>
        </li>
      ))}
    </ul>
  );
}

/**
 * Each member who joined and each link not used yet, with the host's note of who each is and what
 * they may do, each a row that opens its own dialog (D281 §5).
 */
export function SessionPeople(
  props: Omit<PeopleActions, "sessionId"> & { readonly details: SessionDetails },
): JSX.Element {
  const { t } = useTranslation();
  const [opened, setOpened] = useState<Opened>(undefined);
  const close = (): void => {
    setOpened(undefined);
  };
  // A link to pass on takes the dialog's place.
  const actions = {
    ...props,
    sessionId: props.details.sessionId,
    onIssued: (issued: Issued) => {
      close();
      props.onIssued(issued);
    },
  };
  const allLinks = props.details.links ?? [];
  const links = allLinks.filter((link) => link.usedByMemberId === undefined);
  const members = (props.details.members ?? []).filter((member) => !member.isHost);
  const openMember = members.find(
    (member) => opened !== undefined && "memberId" in opened && opened.memberId === member.id,
  );
  const openLink = links.find(
    (link) => opened !== undefined && "linkId" in opened && opened.linkId === link.id,
  );
  return (
    <>
      {links.length === 0 && members.length === 0 ? <p>{t("watchTogether.nobody")}</p> : null}
      <PeopleList members={members} links={links} allLinks={allLinks} onOpen={setOpened} />
      {openMember === undefined ? null : (
        <PersonDialog
          {...actions}
          member={openMember}
          joinedBy={allLinks.find((link) => link.usedByMemberId === openMember.id)}
          onClose={close}
        />
      )}
      {openLink === undefined ? null : <LinkDialog {...actions} link={openLink} onClose={close} />}
    </>
  );
}
