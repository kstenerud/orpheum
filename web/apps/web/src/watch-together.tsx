// SPDX-License-Identifier: MPL-2.0

import "./watch-together.css";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { BusyGroup } from "./busy-group.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { FormRow } from "./form-row.tsx";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { ApiProblem, dataOrThrow, fieldProblemText, throwIfRefused } from "./problem.ts";
import { QrCode } from "./qr-code.tsx";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { Loaded } from "./title.tsx";
import { useMembership } from "./together-membership.ts";
import { forgetMembership, keepMembership, type Membership } from "./together-storage.ts";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** An answer the server may have refused, as throwIfRefused reads it. */
type Answer = Parameters<typeof throwIfRefused>[0];

/** A session as its host sees it: its links and members, with the host's notes. */
type SessionDetails = components["schemas"]["TogetherDetailsOutputBody"];

/** The link a person joins by: the session's page, the secret after # so no request carries it (D243 §3). */
export function togetherUrl(origin: string, secret: string): string {
  return `${origin}/together#${secret}`;
}

/**
 * The host's Watch Together screen (D246 §3, §5): start a session, or have back the one this tab
 * started; make a link for each person with a note of who it is for, which only the host sees;
 * withdraw a link, remove a member, end the session. While it is on, what the host plays plays
 * for everyone in it.
 */
export function WatchTogether(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section className="watch-together" aria-labelledby="watch-together-heading">
      <h1 id="watch-together-heading">{t("watchTogether.heading")}</h1>
      <WatchTogetherControls />
    </section>
  );
}

/**
 * What the host does with their session, on its own screen or in the status tray's pop-up over
 * whatever screen they are on (D280), so managing it never means leaving the film, which stops it
 * for everyone (D246 §5): start one, or have back the one this tab started, and run it.
 */
export function WatchTogetherControls(): JSX.Element {
  // Started here, or had back from the server on another of the host's devices (D267 §2).
  const membership = useMembership(tabStorage, "host");
  const start = (started: Membership): void => {
    keepMembership(tabStorage(), "host", started);
  };
  return (
    <>
      {membership === undefined ? (
        <StartSession onStarted={start} />
      ) : (
        <HostedSession
          key={membership.sessionId}
          membership={membership}
          onStarted={start}
          onEnded={() => {
            forgetMembership(tabStorage(), "host");
          }}
        />
      )}
    </>
  );
}

/** How a refusal on this screen is told: a role that may not host says so. */
function hostingWords(t: (key: string) => string): Parameters<typeof fieldProblemText>[2] {
  return {
    fields: { "body.hostNote": t("watchTogether.note") },
    overrides: { forbidden: t("watchTogether.mayNotHost") },
  };
}

/** Starting a session with nothing on, or having back the one the host has (D246 §5). */
function StartSession(props: {
  readonly onStarted: (membership: Membership) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, canFollowAGroup } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, hostingWords(t)),
  );
  if (!canFollowAGroup()) {
    // The host's player is a member's too (D250 §2), and must keep in step (D250 §7).
    return <p role="alert">{t("watchTogether.cannotHost")}</p>;
  }
  return (
    <>
      <p>{t("watchTogether.explain")}</p>
      <button
        type="button"
        className="watch-together__start"
        disabled={isBusy}
        onClick={() => {
          runRequest(async (idempotencyKey) => {
            const opened = dataOrThrow(await api.POST("/together", idempotent(idempotencyKey)));
            props.onStarted({
              sessionId: opened.sessionId,
              credential: opened.membershipCredential,
            });
          });
        }}
      >
        {t("watchTogether.start")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}

/** Whether a failure to read the session means it is over: ended, or no longer the host's. */
function isSessionOver(cause: unknown): boolean {
  return cause instanceof ApiProblem && (cause.code === "not_found" || cause.code === "forbidden");
}

/** The session this tab hosts, read again as people join; over, it offers a new one. */
function HostedSession(props: {
  readonly membership: Membership;
  readonly onStarted: (membership: Membership) => void;
  readonly onEnded: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { sessionId } = props.membership;
  const [details, reload] = useResource(
    `together/${String(sessionId)}`,
    async () =>
      dataOrThrow(await api.GET("/together/{id}", { params: { path: { id: sessionId } } })),
    useBackgroundChangesPoller(),
  );
  if (details.state === "failed" && isSessionOver(details.cause)) {
    return (
      <>
        <output>{t("watchTogether.over")}</output>
        <StartSession onStarted={props.onStarted} />
      </>
    );
  }
  return (
    <Loaded
      resource={details}
      render={(value) => <SessionView details={value} onChanged={reload} onEnded={props.onEnded} />}
    />
  );
}

/** A session's links and members, with what the host does to them. */
function SessionView(props: {
  readonly details: SessionDetails;
  readonly onChanged: () => void;
  readonly onEnded: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [issued, setIssued] = useState<{ note: string; secret: string } | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, hostingWords(t)),
  );
  const id = props.details.sessionId;
  return (
    <>
      <p>{t("watchTogether.onExplain")}</p>
      <BusyGroup isBusy={isBusy}>
        <MakeLink
          nextRights={props.details.nextRights}
          onMake={(hostNote, rights) => {
            runRequest(async (idempotencyKey) => {
              const made = dataOrThrow(
                await api.POST("/together/{id}/links", {
                  params: { path: { id } },
                  body: { hostNote, rights },
                  ...idempotent(idempotencyKey),
                }),
              );
              setIssued({ note: hostNote, secret: made.secret });
              props.onChanged();
            });
          }}
        />
      </BusyGroup>
      {issued === undefined ? null : <IssuedLink note={issued.note} secret={issued.secret} />}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <BusyGroup isBusy={isBusy}>
        <SessionPeople
          details={props.details}
          run={runRequest}
          onChanged={props.onChanged}
          onIssued={setIssued}
        />
        <ConfirmButton
          label={t("watchTogether.end")}
          consequence={t("watchTogether.endConsequence")}
          confirmLabel={t("watchTogether.endConfirm")}
          onConfirm={() => {
            runRequest(async () => {
              throwIfRefused(await api.DELETE("/together/{id}", { params: { path: { id } } }));
              props.onEnded();
            });
          }}
        />
      </BusyGroup>
    </>
  );
}

/** A member as the session's details list them. */
type SessionMember = NonNullable<SessionDetails["members"]>[number];

/**
 * Gives member a new link: someone moving to another device, not signed in, comes back by one. They
 * are removed, cutting off every device they came in on, and the link admits them again under the
 * same note (D267 §4). Returns the link to pass on.
 */
async function relink(
  api: ReturnType<typeof useServices>["api"],
  member: SessionMember & { readonly sessionId: number; readonly rights: Rights | undefined },
  idempotencyKey: string,
): Promise<{ readonly note: string; readonly secret: string }> {
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
      // The new link gives what the one it replaces gave (D271 §4).
      body: rights === undefined ? { hostNote } : { hostNote, rights },
      ...idempotent(`${idempotencyKey}-link`),
    }),
  );
  return { note: hostNote, secret: made.secret };
}

/** What a guest may do to playback, as a link gives it (D271 §4). */
type Rights = SessionDetails["nextRights"];

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
 * Making one person's link: the host's note of who it is for, which only the host sees, and what
 * they may do to playback, starting with what the last link gave (D271 §4).
 */
function MakeLink(props: {
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
      <h2>{t("watchTogether.makeLinkHeading")}</h2>
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

/** The link just made, to pass on: as text and as a QR code a phone's camera opens. */
function IssuedLink(props: { readonly note: string; readonly secret: string }): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const url = togetherUrl(navigation.origin(), props.secret);
  return (
    <div className="watch-together__issued">
      <h2>{t("watchTogether.issued", { note: props.note })}</h2>
      <QrCode text={url} label={t("watchTogether.qrLabel", { note: props.note })} />
      <Share kind="link" text={url} invitation={undefined} />
    </div>
  );
}

/** What a row of the session's people acts with: its session, and how a change is sent and told. */
interface PeopleActions {
  readonly sessionId: number;
  readonly run: (request: (idempotencyKey: string) => Promise<void>) => void;
  readonly onChanged: () => void;
  /** Told a link to pass on: shown again (D267 §5), or new, which cut off its member (§4). */
  readonly onIssued: (issued: { readonly note: string; readonly secret: string }) => void;
}

/** A session link as the host's view lists it. */
type SessionLink = NonNullable<SessionDetails["links"]>[number];

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

/** One link not used yet, with the host's note of who it is for, to show again or withdraw. */
function LinkRow(props: PeopleActions & { readonly link: SessionLink }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { link } = props;
  return (
    <li>
      {t("watchTogether.waitingLink", { note: link.hostNote })}, {rightsText(t, link.rights)}{" "}
      <button
        type="button"
        onClick={() => {
          props.onIssued({ note: link.hostNote, secret: link.secret });
        }}
      >
        {t("watchTogether.showLink")}
      </button>{" "}
      <button
        type="button"
        onClick={() => {
          sendThenReload(props, () =>
            api.DELETE("/together/{id}/links/{link}", {
              params: { path: { id: props.sessionId, link: link.id } },
            }),
          );
        }}
      >
        {t("watchTogether.withdraw")}
      </button>
    </li>
  );
}

/**
 * One member who joined, with the host's note: the link they joined by to show again, which brings
 * them back on the browser that joined (D267 §5), a new link (§4), or removing them.
 */
function MemberRow(
  props: PeopleActions & {
    readonly member: SessionMember;
    readonly joinedBy: SessionLink | undefined;
  },
): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { member, joinedBy, sessionId } = props;
  return (
    <li>
      {t("watchTogether.member", { name: member.name, note: member.hostNote ?? "" })}
      {joinedBy === undefined ? null : `, ${rightsText(t, joinedBy.rights)}`}{" "}
      {joinedBy === undefined ? null : (
        <>
          <button
            type="button"
            onClick={() => {
              props.onIssued({ note: joinedBy.hostNote, secret: joinedBy.secret });
            }}
          >
            {t("watchTogether.showLink")}
          </button>{" "}
        </>
      )}
      <button
        type="button"
        onClick={() => {
          sendThenReload(props, (idempotencyKey) =>
            api.POST("/together/{id}/eject/{member}", {
              params: { path: { id: sessionId, member: member.id } },
              ...idempotent(idempotencyKey),
            }),
          );
        }}
      >
        {t("watchTogether.eject")}
      </button>{" "}
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
    </li>
  );
}

/** Each link, waiting or used, and each member who joined, with the host's note of who each is. */
function SessionPeople(
  props: Omit<PeopleActions, "sessionId"> & { readonly details: SessionDetails },
): JSX.Element {
  const { t } = useTranslation();
  const actions = { ...props, sessionId: props.details.sessionId };
  const allLinks = props.details.links ?? [];
  const links = allLinks.filter((link) => link.usedByMemberId === undefined);
  const members = (props.details.members ?? []).filter((member) => !member.isHost);
  return (
    <>
      <h2>{t("watchTogether.people")}</h2>
      {links.length === 0 && members.length === 0 ? <p>{t("watchTogether.nobody")}</p> : null}
      <ul>
        {members.map((member) => (
          <MemberRow
            key={`member-${String(member.id)}`}
            {...actions}
            member={member}
            joinedBy={allLinks.find((link) => link.usedByMemberId === member.id)}
          />
        ))}
        {links.map((link) => (
          <LinkRow key={`link-${String(link.id)}`} {...actions} link={link} />
        ))}
      </ul>
    </>
  );
}
