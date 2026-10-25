// SPDX-License-Identifier: MPL-2.0

import "./watch-together.css";

import type { Api } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { BusyGroup } from "./busy-group.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { FilmVolumeSlider } from "./film-volume-slider.tsx";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { ApiProblem, dataOrThrow, fieldProblemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useMembership } from "./together-membership.ts";
import {
  ActionDialog,
  IssuedLink,
  MakeLink,
  SessionPeople,
  type Issued,
  type SessionDetails,
} from "./together-people.tsx";
import { forgetMembership, keepMembership, type Membership } from "./together-storage.ts";
import { useHostVoice, VoiceControls } from "./together-voice.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

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
/**
 * The last details of each session this tab hosts, by its id: the tray's pop-up shows them the
 * moment it opens, and the server's answer replaces them as it comes. Opened, it waited for the
 * server first, and the owner found it slow: "Ideally that should be instantaneous" (2026-10-12).
 */
const knownDetails = new Map<number, SessionDetails>();

/** Asks for a session's details, and keeps them as its last known. */
async function detailsOf(api: Api, sessionId: number): Promise<SessionDetails> {
  const details = dataOrThrow(
    await api.GET("/together/{id}", { params: { path: { id: sessionId } } }),
  );
  knownDetails.set(sessionId, details);
  return details;
}

/** Has the details of the session this tab hosts at hand before its controls open (knownDetails). */
export function usePrefetchedHostedDetails(): void {
  const { api } = useServices();
  const membership = useMembership(tabStorage, "host");
  const sessionId = membership?.sessionId;
  useEffect(() => {
    if (sessionId !== undefined && !knownDetails.has(sessionId)) {
      detailsOf(api, sessionId).catch(() => undefined); // the controls ask again as they open
    }
  }, [api, sessionId]);
}

function HostedSession(props: {
  readonly membership: Membership;
  readonly onStarted: (membership: Membership) => void;
  readonly onEnded: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { sessionId } = props.membership;
  const [asked, reload] = useResource(
    `together/${String(sessionId)}`,
    async () => await detailsOf(api, sessionId),
    useBackgroundChangesPoller(),
  );
  const known = knownDetails.get(sessionId);
  const details: typeof asked =
    asked.state === "loading" && known !== undefined ? { state: "loaded", value: known } : asked;
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

/** Whether someone has joined by the link whose secret is secret. */
function isUsed(details: SessionDetails, secret: string): boolean {
  return (details.links ?? []).some(
    (link) => link.secret === secret && link.usedByMemberId !== undefined,
  );
}

/**
 * Closes a link's dialog once someone joins by it while it is open: the person it was shown for is
 * in, and the host need not close it themselves (the owner, 2026-10-12). A used link shown again to
 * pass on stays open.
 */
function useClosedOnJoining(
  dialog: SessionDialog,
  details: SessionDetails,
  setDialog: (dialog: SessionDialog) => void,
): void {
  const isJoined = dialog?.kind === "issued" && !dialog.wasUsed && isUsed(details, dialog.secret);
  useEffect(() => {
    if (isJoined) {
      setDialog(undefined);
    }
  }, [isJoined, setDialog]);
}

/** Which dialog is over the session's controls: inviting, or a link to pass on (D281 §5). */
type SessionDialog =
  | { readonly kind: "makeLink" }
  | ({ readonly kind: "issued"; readonly wasUsed: boolean } & Issued)
  | undefined;

/** Ending the session, apart from the people above it, so it is never pressed for theirs. */
function EndSession(props: {
  readonly sessionId: number;
  readonly run: (request: (idempotencyKey: string) => Promise<void>) => void;
  readonly onEnded: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const id = props.sessionId;
  return (
    <div className="watch-together__end">
      <ConfirmButton
        label={t("watchTogether.end")}
        consequence={t("watchTogether.endConsequence")}
        confirmLabel={t("watchTogether.endConfirm")}
        onConfirm={() => {
          props.run(async () => {
            throwIfRefused(await api.DELETE("/together/{id}", { params: { path: { id } } }));
            props.onEnded();
          });
        }}
      />
    </div>
  );
}

/** The dialog open over the session's controls: making a link, or a link to pass on (D281 §5). */
function SessionDialogs(props: {
  readonly dialog: SessionDialog;
  readonly details: SessionDetails;
  readonly run: (request: (idempotencyKey: string) => Promise<void>) => void;
  readonly onChanged: () => void;
  readonly onIssued: (issued: Issued) => void;
  readonly onClose: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { dialog } = props;
  const id = props.details.sessionId;
  if (dialog?.kind === "issued") {
    return (
      <ActionDialog
        heading={t("watchTogether.issued", { note: dialog.note })}
        onClose={props.onClose}
      >
        <IssuedLink note={dialog.note} secret={dialog.secret} link={dialog.link} />
      </ActionDialog>
    );
  }
  if (dialog?.kind !== "makeLink") {
    return null;
  }
  return (
    <ActionDialog
      heading={t("watchTogether.makeLinkHeading")}
      onClose={props.onClose}
      shouldFocusFirstField
    >
      <MakeLink
        nextRights={props.details.nextRights}
        onMake={(hostNote, rights) => {
          props.run(async (idempotencyKey) => {
            const made = dataOrThrow(
              await api.POST("/together/{id}/links", {
                params: { path: { id } },
                body: { hostNote, rights },
                ...idempotent(idempotencyKey),
              }),
            );
            props.onIssued({ note: hostNote, secret: made.secret, link: made.link });
            props.onChanged();
          });
        }}
      />
    </ActionDialog>
  );
}

/**
 * A session as its host runs it, on its own screen or in the status tray's pop-up during the film
 * (D281 §5): voice and each person's volume, the film's volume, the people with a dialog each,
 * Invite, which opens a dialog of its own, and ending it.
 */
function SessionView(props: {
  readonly details: SessionDetails;
  readonly onChanged: () => void;
  readonly onEnded: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const voice = useHostVoice();
  const [dialog, setDialog] = useState<SessionDialog>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, hostingWords(t)),
  );
  const id = props.details.sessionId;
  const showIssued = (issued: Issued): void => {
    setDialog({ kind: "issued", ...issued, wasUsed: isUsed(props.details, issued.secret) });
  };
  useClosedOnJoining(dialog, props.details, setDialog);
  return (
    <div className="watch-together__controls">
      <p>{t("watchTogether.onExplain")}</p>
      {voice === undefined ? null : <VoiceControls {...voice} />}
      <FilmVolumeSlider />
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <BusyGroup isBusy={isBusy}>
        <div className="watch-together__people-head">
          <h2>{t("watchTogether.people")}</h2>
          <button
            type="button"
            onClick={() => {
              setDialog({ kind: "makeLink" });
            }}
          >
            {t("watchTogether.makeLinkHeading")}
          </button>
        </div>
        <SessionPeople
          details={props.details}
          run={runRequest}
          onChanged={props.onChanged}
          onIssued={showIssued}
        />
        <SessionDialogs
          dialog={dialog}
          details={props.details}
          run={runRequest}
          onChanged={props.onChanged}
          onIssued={showIssued}
          onClose={() => {
            setDialog(undefined);
          }}
        />
        <EndSession sessionId={id} run={runRequest} onEnded={props.onEnded} />
      </BusyGroup>
    </div>
  );
}
