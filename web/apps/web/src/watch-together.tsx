// SPDX-License-Identifier: MPL-2.0

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
import {
  forgetMembership,
  keepMembership,
  readMembership,
  type Membership,
} from "./together-storage.ts";
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
  const [membership, setMembership] = useState(() => readMembership(tabStorage(), "host"));
  const start = (started: Membership): void => {
    keepMembership(tabStorage(), "host", started);
    setMembership(started);
  };
  return (
    <section className="watch-together" aria-labelledby="watch-together-heading">
      <h1 id="watch-together-heading">{t("watchTogether.heading")}</h1>
      {membership === undefined ? (
        <StartSession onStarted={start} />
      ) : (
        <HostedSession
          key={membership.sessionId}
          membership={membership}
          onStarted={start}
          onEnded={() => {
            forgetMembership(tabStorage(), "host");
            setMembership(undefined);
          }}
        />
      )}
    </section>
  );
}

/** How a refusal on this screen is told: a role that may not host says so. */
function hostingWords(t: (key: string) => string): Parameters<typeof fieldProblemText>[2] {
  return {
    fields: { "body.adminNote": t("watchTogether.note") },
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
          onMake={(adminNote) => {
            runRequest(async (idempotencyKey) => {
              const made = dataOrThrow(
                await api.POST("/together/{id}/links", {
                  params: { path: { id } },
                  body: { adminNote },
                  ...idempotent(idempotencyKey),
                }),
              );
              setIssued({ note: adminNote, secret: made.secret });
              props.onChanged();
            });
          }}
        />
      </BusyGroup>
      {issued === undefined ? null : <IssuedLink note={issued.note} secret={issued.secret} />}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <BusyGroup isBusy={isBusy}>
        <SessionPeople details={props.details} run={runRequest} onChanged={props.onChanged} />
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

/** Making one person's link: the host's note of who it is for, which only the host sees. */
function MakeLink(props: { readonly onMake: (adminNote: string) => void }): JSX.Element {
  const { t } = useTranslation();
  const [adminNote, setAdminNote] = useState("");
  return (
    <FormRow
      onSubmit={() => {
        props.onMake(adminNote);
      }}
    >
      <h2>{t("watchTogether.makeLinkHeading")}</h2>
      <label>
        {t("watchTogether.note")}{" "}
        <input
          required
          maxLength={200}
          placeholder={t("watchTogether.noteExample")}
          value={adminNote}
          onChange={(event) => {
            setAdminNote(event.target.value);
          }}
        />
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

/** Each link, waiting or used, and each member who joined, with the host's note of who each is. */
function SessionPeople(props: {
  readonly details: SessionDetails;
  readonly run: (request: (idempotencyKey: string) => Promise<void>) => void;
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const id = props.details.sessionId;
  const links = (props.details.links ?? []).filter((link) => link.usedByMemberId === undefined);
  const members = (props.details.members ?? []).filter((member) => !member.isHost);
  const sendThenReload = (request: (idempotencyKey: string) => Promise<Answer>): void => {
    props.run(async (idempotencyKey) => {
      throwIfRefused(await request(idempotencyKey));
      props.onChanged();
    });
  };
  return (
    <>
      <h2>{t("watchTogether.people")}</h2>
      {links.length === 0 && members.length === 0 ? <p>{t("watchTogether.nobody")}</p> : null}
      <ul>
        {members.map((member) => (
          <li key={`member-${String(member.id)}`}>
            {t("watchTogether.member", { name: member.name, note: member.adminNote ?? "" })}{" "}
            <button
              type="button"
              onClick={() => {
                sendThenReload((idempotencyKey) =>
                  api.POST("/together/{id}/eject/{member}", {
                    params: { path: { id, member: member.id } },
                    ...idempotent(idempotencyKey),
                  }),
                );
              }}
            >
              {t("watchTogether.eject")}
            </button>
          </li>
        ))}
        {links.map((link) => (
          <li key={`link-${String(link.id)}`}>
            {t("watchTogether.waitingLink", { note: link.adminNote })}{" "}
            <button
              type="button"
              onClick={() => {
                sendThenReload(() =>
                  api.DELETE("/together/{id}/links/{link}", {
                    params: { path: { id, link: link.id } },
                  }),
                );
              }}
            >
              {t("watchTogether.withdraw")}
            </button>
          </li>
        ))}
      </ul>
    </>
  );
}
