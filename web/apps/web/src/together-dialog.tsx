// SPDX-License-Identifier: MPL-2.0

import "./watch-together.css";

import { useEffect, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { problemText } from "./problem.ts";
import { useServices, useToken } from "./services.tsx";
import { LeaveButton } from "./together-leave.tsx";
import { useMembership } from "./together-membership.ts";
import { moveHostingHere, useHostedElsewhereHere, type HostedElsewhere } from "./together-mine.ts";
import { useSessionEnd } from "./together-session-end.ts";
import type { Membership } from "./together-storage.ts";
import { useRequest } from "./use-request.ts";
import { useTogether } from "./use-together.ts";
import { WatchTogetherControls } from "./watch-together.tsx";

/**
 * Watch Together's dialog, which the tray's Watch Together opens over whatever page is shown and which
 * never moves it (D384 §2; the owner: "Watch together keeps track of the shared video being played
 * and offers a way to get back to it, but ultimately there is one playback page that everything goes
 * through"): for one who may host, starting a session and running it, what plays with the way back
 * to its film's Play page and Stop for everyone, voice and the volumes, the people and Invite, and
 * ending it (watch-together.tsx), which closes the dialog, nothing opening it again (D385's Also); for
 * a signed-in guest, the way back to the group's film and Leave; for a host whose session is on another of
 * their devices, where it is, and Move it here (D442 §2). The sign over the playing film opens the
 * same dialog, the session's panel moved in under its controls (D433).
 */
export function TogetherDialog(props: {
  readonly onClose: () => void;
  /**
   * Where the session's panel is moved, under what plays, when the sign over the playing film
   * opens the dialog (D433 §2, together-over-the-film.tsx); nothing from the tray. With it, the panel
   * holds Talk and the volumes, which the host's controls then leave to it, and the guest's Leave; a
   * guest, at the group's film already, is offered no way back to it.
   */
  readonly panelPlace?: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const viewing = useMembership(browserStorage, "viewer");
  const hasPanel = props.panelPlace !== undefined;
  const isGuest = viewing !== undefined && !isHosting;
  const elsewhere = useHostedElsewhereHere();
  const isElsewhere = elsewhere !== undefined;
  return (
    <ModalDialog label={t("watchTogether.heading")} onClose={props.onClose}>
      <section className="watch-together" aria-labelledby="watch-together-heading">
        <h2 id="watch-together-heading">{t("watchTogether.heading")}</h2>
        {isGuest && !hasPanel ? (
          <GuestsSession membership={viewing} onEnded={props.onClose} />
        ) : null}
        {isElsewhere ? <OnElsewhere elsewhere={elsewhere} /> : null}
        {isGuest || isElsewhere ? null : (
          <WatchTogetherControls
            onEnded={props.onClose}
            panelPlace={isHosting ? props.panelPlace : undefined}
          />
        )}
        {/* The host's panel stands under what plays, among the controls; any other at the foot. */}
        {isHosting ? null : props.panelPlace}
      </section>
    </ModalDialog>
  );
}

/**
 * A signed-in guest's session, in Watch Together's dialog: the way back to the group's film, whose
 * Play page is theirs in the session (voice, their volume and Leave under it), or that nothing plays
 * yet, with Leave beside it, the viewer page's own, so a device whose Play page is out of reach (as
 * under Android's own full screen) still leaves (the owner's 11-20 finding 2, 2026-11-07); left, the
 * session ends here as any end does. Read by a panel of the session while the dialog is open, which plays nothing. A session the
 * server no longer has (lost with a restart, or ended unseen) ends here as any end does: the
 * membership forgotten, the page home and told so, the dialog closed (the owner's S23, staging,
 * 2026-10-30: the sign stayed blue and the dialog said "Loading…" for good).
 */
function GuestsSession(props: {
  readonly membership: Membership;
  readonly onEnded: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const { snapshot } = useTogether({
    credential: props.membership.credential,
    sessionToken: useToken(),
    role: "panel",
  });
  const { over } = useSessionEnd(true, undefined);
  const endReason = snapshot?.status === "ended" ? (snapshot.endReason ?? "gone") : undefined;
  const { onEnded } = props;
  useEffect(() => {
    if (endReason !== undefined) {
      onEnded();
      over.end(endReason);
    }
  }, [endReason, onEnded, over]);
  if (snapshot?.status !== "connected") {
    return <p>{t("common.loading")}</p>;
  }
  const fileId = snapshot.group?.fileId ?? 0;
  const leave = (
    <LeaveButton
      membership={props.membership}
      onLeft={(reason) => {
        onEnded();
        over.end(reason);
      }}
    />
  );
  if (fileId === 0) {
    return (
      <>
        <p>{t("watchTogether.nothingPlays")}</p>
        {leave}
      </>
    );
  }
  return (
    <div className="together-plays__buttons">
      <button
        type="button"
        className="title__play"
        onClick={() => {
          navigation.go({ screen: "play", fileId });
        }}
      >
        {t("watchTogether.backToTheFilm")}
      </button>
      {leave}
    </div>
  );
}

/**
 * The session the person hosts on another of their devices, in Watch Together's dialog: that it is
 * on, there, and Move it here, which makes this tab its host's one device, the other told it moved
 * (D442 §2). Starting another is not offered: it would end this one (D372 §8).
 */
function OnElsewhere(props: { readonly elsewhere: HostedElsewhere }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const { sessionId, deviceName } = props.elsewhere;
  return (
    <>
      <p>
        {deviceName === undefined
          ? t("watchTogether.onElsewhere")
          : t("watchTogether.onElsewhereNamed", { device: deviceName })}
      </p>
      <button
        type="button"
        className="watch-together__start"
        disabled={isBusy}
        onClick={() => {
          runRequest(async (idempotencyKey) => {
            await moveHostingHere(api, sessionId, idempotencyKey);
          });
        }}
      >
        {t("watchTogether.moveHere")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}
