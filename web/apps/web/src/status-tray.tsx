// SPDX-License-Identifier: MPL-2.0

import "./status-tray.css";

import type { CastSending } from "@orpheum/sdk";
import { useEffect, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { Icon } from "./icon.tsx";
import { IconButton } from "./icon-button.tsx";
import { Link } from "./link.tsx";
import { useLive } from "./live.tsx";
import { dataOrThrow } from "./problem.ts";
import { routeHref } from "./router.ts";
import { useServices } from "./services.tsx";
import { closeTrayDialog, openTrayDialog, useTrayDialog } from "./tray-dialog.ts";
import { useMembership } from "./together-membership.ts";
import { CastDialog, TogetherDialog, useLoadsTrayDialogsAhead } from "./tray-dialog-code.tsx";
import { LaterScreen } from "./screen-load.tsx";
import { useResource } from "./use-resource.ts";
import { WarningIcon } from "./warning-sign.tsx";
import { usePrefetchedHostedDetails } from "./hosted-details.ts";

/**
 * The status tray at the banner's end (D280), on every screen. Watch Together's and Cast's icons are
 * always in it, the entry points to both, each opening its dialog over the page (D384 §2), white while their function is off and blue, pulsing
 * gently, while it is on (D364 §12, §13); Watch Together's is shown to a person who may host it, and
 * to a signed-in guest in a session (§14). For an admin, the warning sign leads to what needs them,
 * when anything does; it alone is yellow (D278).
 */
export function StatusTray(props: {
  readonly isAdmin: boolean;
  /** Whether the person's role may host Watch Together, whose icon then shows while it is off. */
  readonly canHostWatchTogether: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const isViewing = useMembership(browserStorage, "viewer") !== undefined;
  const { sending } = useLive();
  useClosesTheDialogAsThePageMoves();
  useLoadsTrayDialogsAhead();
  return (
    <section className="status-tray icon-tray" aria-label={t("status.tray")}>
      {props.isAdmin ? <NeedsAttentionSign /> : null}
      {props.canHostWatchTogether || isHosting || isViewing ? (
        <TogetherSign isHosting={isHosting} isViewing={isViewing} />
      ) : null}
      <CastSign sending={sending} />
    </section>
  );
}

/**
 * Closes the tray's dialog as the page moves to another address, as by its own way back to a film:
 * the dialog belongs to the page it opened over. A step of the history that leaves the address as it
 * was, as a dialog opened from it closing, leaves it open.
 */
function useClosesTheDialogAsThePageMoves(): void {
  const { navigation } = useServices();
  useEffect(() => {
    let shown = routeHref(navigation.route());
    return navigation.subscribe(() => {
      const now = routeHref(navigation.route());
      if (now !== shown) {
        shown = now;
        closeTrayDialog();
      }
    });
  }, [navigation]);
}

/** A sign's mark: blue and pulsing while its function is on, the banner's white while it is off. */
function signMark(isOn: boolean): "running" | undefined {
  return isOn ? "running" : undefined;
}

/**
 * Watch Together's sign (D364 §13, D384 §2): it opens Watch Together's dialog over the page, to start
 * a session or run it, or, for a signed-in guest, to go back to the group's film, and never moves the
 * page; blue while a session is on. The hosted session's details are fetched as it shows, so its
 * dialog shows them at once (hosted-details.ts).
 */
function TogetherSign(props: {
  readonly isHosting: boolean;
  readonly isViewing: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  usePrefetchedHostedDetails();
  const isOn = props.isHosting || props.isViewing;
  return (
    <>
      <IconButton
        label={t(isOn ? "status.togetherOn" : "status.together")}
        icon={<TogetherIcon />}
        mark={signMark(isOn)}
        onClick={() => {
          openTrayDialog("together");
        }}
      />
      {useTrayDialog() === "together" ? (
        <LaterScreen resetKey="together-dialog">
          <TogetherDialog onClose={closeTrayDialog} />
        </LaterScreen>
      ) : null}
    </>
  );
}

/**
 * Cast's sign (D364 §13, D384 §2): it opens Cast's dialog over the page, to choose a device or stop,
 * and never moves the page; blue while a cast is on, named by its receiver and what plays.
 */
function CastSign(props: { readonly sending: CastSending | undefined }): JSX.Element {
  const { t } = useTranslation();
  const { sending } = props;
  const label =
    sending === undefined
      ? t("status.cast")
      : t(sending.fileId === 0 ? "status.castingReady" : "status.castingFilm", {
          name: sending.receiverName,
          film: sending.titleName,
        });
  return (
    <>
      <IconButton
        label={label}
        icon={<CastIcon />}
        mark={signMark(sending !== undefined)}
        onClick={() => {
          openTrayDialog("cast");
        }}
      />
      {useTrayDialog() === "cast" ? (
        <LaterScreen resetKey="cast-dialog">
          <CastDialog onClose={closeTrayDialog} />
        </LaterScreen>
      ) : null}
    </>
  );
}

/**
 * A screen playing, two people before it: Watch Together, in the colour of the text it sits in. The
 * owner's choice, drawn by Claude Design (2026-10-12).
 */
export function TogetherIcon(): JSX.Element {
  return (
    <Icon className="status-tray__icon">
      <path
        fillRule="evenodd"
        d="M4 2h16a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2zm6 2.8v5.4l4.6-2.7z"
      />
      <circle cx="7.5" cy="17.3" r="2.3" />
      <circle cx="16.5" cy="17.3" r="2.3" />
      <path d="M3.2 23a4.3 3.6 0 0 1 8.6 0z" />
      <path d="M12.2 23a4.3 3.6 0 0 1 8.6 0z" />
    </Icon>
  );
}

/**
 * A screen playing, a phone beside it: casting to another device (D364), the tray's Cast. The owner's
 * choice (2026-10-27), in the colour of the text it sits in.
 */
export function CastIcon(): JSX.Element {
  return (
    <Icon className="status-tray__icon">
      <path
        fillRule="evenodd"
        d="M7 2h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H10.5a1 1 0 0 1-1-1v-1.4a3.1 3.1 0 0 0-3.1-3.1H6a1 1 0 0 1-1-1V4a2 2 0 0 1 2-2zM12.8 5.8v5.4l4.6-2.7z"
      />
      <path d="M13 15h2v4.5h-2z" />
      <path d="M10.5 19.5h7a1 1 0 0 1 0 2h-7a1 1 0 0 1 0-2z" />
      <path
        fillRule="evenodd"
        d="M2.6 11h3.8A1.6 1.6 0 0 1 8 12.6v8.8A1.6 1.6 0 0 1 6.4 23H2.6A1.6 1.6 0 0 1 1 21.4v-8.8A1.6 1.6 0 0 1 2.6 11zM3.3 20.4h2.4v1.1H3.3z"
      />
    </Icon>
  );
}

/** For an admin, the warning sign when anything needs them, leading to the page that lists it. */
function NeedsAttentionSign(): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const [counts] = useResource("status-needs-attention", async () =>
    dataOrThrow(await api.GET("/needs-attention")),
  );
  if (counts.state !== "loaded") {
    return null;
  }
  const { rootCount, diskCount, waitingRootCount, unreadCount, matchCount } = counts.value;
  const readWrongly = counts.value.seriesReadWrongly ?? [];
  if (
    rootCount + diskCount + waitingRootCount + unreadCount + matchCount + readWrongly.length ===
    0
  ) {
    return null;
  }
  return (
    <Link
      to={{ screen: "needs-attention" }}
      className="icon-button status-tray__button status-tray__button--warning"
      label={t("status.needsAttention")}
    >
      <WarningIcon />
    </Link>
  );
}
