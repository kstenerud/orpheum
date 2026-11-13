// SPDX-License-Identifier: MPL-2.0

import type { components, RosterMember } from "@orpheum/sdk";
import { useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { timeText } from "./date-time-text.ts";
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
  const playback = playbackRightsText(t, rights);
  if (!rights.canNext) {
    return playback ?? t("watchTogether.rights.none");
  }
  const next = t("watchTogether.rights.next");
  return playback === undefined ? next : `${playback}; ${next}`;
}

/** What rights give over the film playing, in a few words; undefined where they give nothing. */
function playbackRightsText(t: (key: string) => string, rights: Rights): string | undefined {
  if (rights.canPlayPause && rights.canSeek) {
    return t("watchTogether.rights.both");
  }
  if (rights.canPlayPause) {
    return t("watchTogether.rights.playPause");
  }
  return rights.canSeek ? t("watchTogether.rights.seek") : undefined;
}

/**
 * A dialog over the session's controls, its own heading and Close (D281 §5): what is done to one
 * person, or a link made, opened from the list and gone once done, or on a press outside it.
 */
export function ActionDialog(props: {
  readonly heading: string;
  readonly onClose: () => void;
  /** Whether it opens with its first field focused (ModalDialog). */
  readonly shouldFocusFirstField?: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <ModalDialog
      label={props.heading}
      onClose={props.onClose}
      // A press outside closes it: what it holds is a few words and two ticks, nothing worth a guard
      // against a stray press (the owner, 2026-10-29: "losing it isn't a big deal").
      shouldFocusFirstField={props.shouldFocusFirstField === true}
    >
      <div className="watch-together__dialog">
        <div className="watch-together__dialog-head">
          <h2>{props.heading}</h2>
        </div>
        {props.children}
      </div>
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
      <label>
        <input
          type="checkbox"
          checked={rights.canNext}
          onChange={(event) => {
            setRights({ ...rights, canNext: event.target.checked });
          }}
        />{" "}
        {t("watchTogether.mayNext")}
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
  /** Told a link to pass on: shown again (D267 §5), or new, which takes its member's place once used. */
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
 * Gives member a new link for their place: someone moving to another device, not signed in, comes
 * back by one. They stay in until it is used; whoever joins by it then takes their place, with its
 * note and rights, and the devices they came in on are cut off (D311 §5, D312 §3; the owner,
 * 2026-10-19). Returns the link to pass on.
 */
async function relink(
  api: ReturnType<typeof useServices>["api"],
  member: SessionMember & { readonly sessionId: number },
  idempotencyKey: string,
): Promise<Issued> {
  const made = dataOrThrow(
    await api.POST("/together/{id}/members/{member}/links", {
      params: { path: { id: member.sessionId, member: member.id } },
      ...idempotent(idempotencyKey),
    }),
  );
  return { note: member.hostNote ?? member.name, secret: made.secret, link: made.link };
}

/** A new link for a member's place, which takes it over once used (D311 §5, D312 §3). */
function RelinkButton(props: PeopleActions & { readonly member: SessionMember }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { member, sessionId } = props;
  return (
    <button
      type="button"
      onClick={() => {
        props.run(async (idempotencyKey) => {
          props.onIssued(await relink(api, { ...member, sessionId }, idempotencyKey));
          props.onChanged();
        });
      }}
    >
      {t("watchTogether.relink")}
    </button>
  );
}

/**
 * What the host does with one member (D281 §5): how loud their voice plays, but over the film, whose
 * overlay's 🔈 holds each person's volume (D440 §2, amending D281 §5), the link they joined by to
 * show again, which brings them back on the browser that joined (D267 §5), a new link (§4),
 * or removing them.
 */
function PersonDialog(
  props: PeopleActions & {
    readonly member: SessionMember;
    readonly joinedBy: SessionLink | undefined;
    readonly isOverTheFilm: boolean;
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
      {voice === undefined || props.isOverTheFilm ? null : (
        <PersonVoiceVolume {...voice} memberId={member.id} name={member.name} />
      )}
      <p>{t("watchTogether.relinkExplain", { name: member.name })}</p>
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
              api.POST("/together/{id}/members/{member}/eject", {
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

/**
 * Whether the session's roster has member with no device on the session's page: away, elsewhere in
 * the app or their page closed (D372 §9); false while the roster is not known or does not have them.
 */
function isAway(roster: readonly RosterMember[] | undefined, member: SessionMember): boolean {
  return roster?.find((each) => each.id === member.id)?.devices.length === 0;
}

/**
 * The people's rows: each member, said away while they have no device on the session's page, then
 * each link not used yet, opening its dialog when pressed.
 */
function PeopleList(props: {
  readonly members: readonly SessionMember[];
  readonly links: readonly SessionLink[];
  readonly allLinks: readonly SessionLink[];
  readonly roster: readonly RosterMember[] | undefined;
  readonly onOpen: (opened: Opened) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
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
              {isAway(props.roster, member) ? `, ${t("watchTogether.memberAway")}` : null}
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
            {link.expiresAt === undefined
              ? null
              : `, ${t("watchTogether.linkUntil", { time: timeText(link.expiresAt, i18n.language) })}`}
          </button>
        </li>
      ))}
    </ul>
  );
}

/**
 * Each member who joined and each link not used yet, with the host's note of who each is and what
 * they may do, each a row that opens its own dialog (D281 §5); a member the session's roster has
 * with no device on its page is said away (D372 §9).
 */
export function SessionPeople(
  props: Omit<PeopleActions, "sessionId"> & {
    readonly details: SessionDetails;
    /** The session's roster as the host's connection knows it; undefined until it does. */
    readonly roster?: readonly RosterMember[] | undefined;
    /** Whether the film plays under them, its overlay's 🔈 holding each person's volume (D440 §2). */
    readonly isOverTheFilm?: boolean | undefined;
  },
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
      <PeopleList
        members={members}
        links={links}
        allLinks={allLinks}
        roster={props.roster}
        onOpen={setOpened}
      />
      {openMember === undefined ? null : (
        <PersonDialog
          {...actions}
          member={openMember}
          joinedBy={allLinks.find((link) => link.usedByMemberId === openMember.id)}
          isOverTheFilm={props.isOverTheFilm === true}
          onClose={close}
        />
      )}
      {openLink === undefined ? null : <LinkDialog {...actions} link={openLink} onClose={close} />}
    </>
  );
}
