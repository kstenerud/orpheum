// SPDX-License-Identifier: MPL-2.0

import "./watch-together.css";

import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { BusyGroup } from "./busy-group.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { FilmVolumeSlider } from "./film-volume-slider.tsx";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { ApiProblem, dataOrThrow, fieldProblemText, throwIfRefused } from "./problem.ts";
import { useServices, useToken } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
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
import { useTogether } from "./use-together.ts";
import { useResource } from "./use-resource.ts";
import { detailsOf, knownDetailsOf } from "./hosted-details.ts";
import { TogetherDebug } from "./together-debug.tsx";
import { TogetherPlays } from "./together-plays.tsx";

/**
 * The host's Watch Together screen (D246 §3, §5), where the tray's Watch Together leads (D364 §13):
 * start a session, or have back the one this tab started; what plays for the group and its controls
 * (§15); make a link for each person with a note of who it is for, which only the host sees;
 * withdraw a link, remove a member, end the session. While it is on, what the host plays plays
 * for everyone in it. Reached from anywhere, it has no Up; Back returns to where the host was (D367).
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
 * What the host does with their session, on its own screen: start one, or have back the one this tab
 * started, and run it. Leaving the film for it stops nothing (D364 §15).
 */
export function WatchTogetherControls(): JSX.Element {
  const { t } = useTranslation();
  // Started here, or had back from the server on another of the host's devices (D267 §2).
  const membership = useMembership(tabStorage, "host");
  // A session the server says is over is forgotten, so nothing asks for it again; the screen says
  // it is over until a new one starts (staging, 2026-10-28: a page asked every 5 s for one gone).
  const [isOver, setIsOver] = useState(false);
  const start = (started: Membership): void => {
    setIsOver(false);
    keepMembership(tabStorage(), "host", started);
  };
  const forget = (): void => {
    forgetMembership(tabStorage(), "host");
  };
  return (
    <>
      {membership === undefined ? (
        <>
          {isOver ? <output>{t("watchTogether.over")}</output> : null}
          <StartSession onStarted={start} />
        </>
      ) : (
        <HostedSession
          key={membership.sessionId}
          membership={membership}
          onOver={() => {
            setIsOver(true);
            forget();
          }}
          onEnded={forget}
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

/**
 * The session this tab hosts, read again as people join; over, as the server says when it is asked,
 * it is given up (onOver), and asked for no more.
 */
function HostedSession(props: {
  readonly membership: Membership;
  readonly onOver: () => void;
  readonly onEnded: () => void;
}): JSX.Element | null {
  const { api } = useServices();
  const { sessionId } = props.membership;
  const [asked, reload] = useResource(
    `together/${String(sessionId)}`,
    async () => await detailsOf(api, sessionId),
    useBackgroundChangesPoller(),
  );
  const known = knownDetailsOf(sessionId);
  const details: typeof asked =
    asked.state === "loading" && known !== undefined ? { state: "loaded", value: known } : asked;
  const isOver = details.state === "failed" && isSessionOver(details.cause);
  const { onOver } = props;
  useEffect(() => {
    if (isOver) {
      onOver();
    }
  }, [isOver, onOver]);
  if (isOver) {
    return null; // forgotten at once, and the screen offers a new one
  }
  return (
    <Loaded
      resource={details}
      render={(value) => (
        <SessionView
          membership={props.membership}
          details={value}
          onChanged={reload}
          onEnded={props.onEnded}
        />
      )}
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

/** The session page's own connection to its session, a panel: it plays nothing (TogetherPlays). */
function usePanelConnection(membership: Membership): ReturnType<typeof useTogether> {
  const token = useToken();
  return useTogether({ credential: membership.credential, sessionToken: token, role: "panel" });
}

/**
 * A session as its host runs it, on its own screen (D281 §5, D364 §15): what plays and its controls,
 * voice and each person's volume, the film's volume, the people with a dialog each, Invite, which
 * opens a dialog of its own, and ending it.
 */
function SessionView(props: {
  readonly membership: Membership;
  readonly details: SessionDetails;
  readonly onChanged: () => void;
  readonly onEnded: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const voice = useHostVoice();
  const session = usePanelConnection(props.membership);
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
      <TogetherPlays session={session} details={props.details} onChanged={props.onChanged} />
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
        <TogetherDebug snapshot={session.snapshot} />
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
