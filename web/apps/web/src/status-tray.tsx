// SPDX-License-Identifier: MPL-2.0

import "./status-tray.css";

import type { CastSending } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { Icon } from "./icon.tsx";
import { Link } from "./link.tsx";
import { useLive } from "./live.tsx";
import { dataOrThrow } from "./problem.ts";
import type { Route } from "./router.ts";
import { useRoute, useServices } from "./services.tsx";
import { useMembership } from "./together-membership.ts";
import { useResource } from "./use-resource.ts";
import { WarningIcon } from "./warning-sign.tsx";
import { usePrefetchedHostedDetails } from "./hosted-details.ts";

/**
 * The status tray at the banner's end (D280), on every screen. Watch Together's and Cast's icons are
 * always in it, the entry points to both, white while their function is off and blue, pulsing
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
  // The route shown now: Cast's sign on a film's Play page takes that film with it (D364 §1).
  const route = useRoute();
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const isViewing = useMembership(browserStorage, "viewer") !== undefined;
  const { sending } = useLive();
  return (
    <section className="status-tray" aria-label={t("status.tray")}>
      {props.isAdmin ? <NeedsAttentionSign /> : null}
      {props.canHostWatchTogether || isHosting || isViewing ? (
        <TogetherSign isHosting={isHosting} isViewing={isViewing} />
      ) : null}
      <CastSign sending={sending} isHosting={isHosting} route={route} />
    </section>
  );
}

/** A sign's class: the tray's button, white while its function is off, blue while it is on. */
function signClass(isOn: boolean): string {
  return isOn
    ? "status-tray__button status-tray__button--on"
    : "status-tray__button status-tray__button--off";
}

/**
 * Watch Together's sign (D364 §13): while its session is on, the session's page, the host's or a
 * signed-in guest's, where its controls are, Stop casting with them while a cast is on too; while it
 * is off, the page that starts one. The hosted session's details are fetched as it shows, so its
 * page shows them at once (hosted-details.ts).
 */
function TogetherSign(props: {
  readonly isHosting: boolean;
  readonly isViewing: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  usePrefetchedHostedDetails();
  const isOn = props.isHosting || props.isViewing;
  const to: Route =
    props.isViewing && !props.isHosting ? { screen: "together" } : { screen: "watch-together" };
  return (
    <Link
      to={to}
      className={signClass(isOn)}
      label={t(isOn ? "status.togetherOn" : "status.together")}
    >
      <TogetherIcon />
    </Link>
  );
}

/**
 * Cast's sign (D364 §13): while a cast is on, its remote, the casting page, or Watch Together's page
 * while that is on too, which holds the controls of both; while none is, the devices to cast to,
 * taking the film this device plays, if any, with it (§1).
 */
function CastSign(props: {
  readonly sending: CastSending | undefined;
  readonly isHosting: boolean;
  readonly route: Route;
}): JSX.Element {
  const { t } = useTranslation();
  const { sending, isHosting, route } = props;
  const to = castSignRoute(sending !== undefined, isHosting, route);
  const label =
    sending === undefined
      ? t("status.cast")
      : t(sending.fileId === 0 ? "status.castingReady" : "status.castingFilm", {
          name: sending.receiverName,
          film: sending.titleName,
        });
  return (
    <Link to={to} className={signClass(sending !== undefined)} label={label}>
      <CastIcon />
    </Link>
  );
}

/**
 * Where Cast's sign leads, by the owner's table (D364 §13): with a cast and Watch Together both on,
 * Watch Together's page; else the casting page, with the film shown now, from its Play page, for a
 * cast to start with.
 */
function castSignRoute(isCasting: boolean, isHosting: boolean, route: Route): Route {
  if (isCasting && isHosting) {
    return { screen: "watch-together" };
  }
  return !isCasting && route.screen === "play"
    ? { screen: "casting", fileId: route.fileId }
    : { screen: "casting" };
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
      className="status-tray__button status-tray__button--warning"
      label={t("status.needsAttention")}
    >
      <WarningIcon />
    </Link>
  );
}
