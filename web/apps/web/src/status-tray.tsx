// SPDX-License-Identifier: MPL-2.0

import "./status-tray.css";

import { lazy, Suspense, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";
import { WarningIcon } from "./warning-sign.tsx";
import { usePrefetchedHostedDetails } from "./hosted-details.ts";

/**
 * The status tray at the banner's end (D280): what is on, in view on every screen, in the status
 * blue. While this tab hosts Watch Together its sign is there, and opens the session's controls
 * over the screen, so the host manages it without leaving the film, which stops it for everyone
 * (D246 §5). For an admin, the warning sign leads to what needs them, when anything does; it alone
 * is yellow (D278).
 */
export function StatusTray(props: {
  readonly isHostingTogether: boolean;
  readonly isAdmin: boolean;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (!props.isHostingTogether && !props.isAdmin) {
    return null;
  }
  return (
    <section className="status-tray" aria-label={t("status.tray")}>
      {props.isAdmin ? <NeedsAttentionSign /> : null}
      {props.isHostingTogether ? <TogetherStatus /> : null}
    </section>
  );
}

/**
 * A screen playing, two people before it: Watch Together, in the status blue. The owner's choice,
 * drawn by Claude Design (2026-10-12).
 */
export function TogetherIcon(): JSX.Element {
  return (
    <svg className="status-tray__icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        fillRule="evenodd"
        d="M4 2h16a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2zm6 2.8v5.4l4.6-2.7z"
      />
      <circle cx="7.5" cy="17.3" r="2.3" />
      <circle cx="16.5" cy="17.3" r="2.3" />
      <path d="M3.2 23a4.3 3.6 0 0 1 8.6 0z" />
      <path d="M12.2 23a4.3 3.6 0 0 1 8.6 0z" />
    </svg>
  );
}

/**
 * Watch Together's controls, whose code loads as the tray first opens them: with them, the player's
 * volume, and hls.js with it, was in the script every page loaded before it drew (the audit of
 * 2026-10-25, W1).
 */
const WatchTogetherControls = lazy(async () => ({
  default: (await import("./watch-together.tsx")).WatchTogetherControls,
}));

/**
 * The Watch Together sign, which opens the session's controls over whatever screen is shown: a
 * modal drop-down, the page behind it taking no press, so a control under it half hidden is never
 * pressed by mistake on a small screen; a press anywhere outside it, Escape or Close shuts it (the
 * owner, 2026-10-11).
 */
function TogetherStatus(): JSX.Element {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  usePrefetchedHostedDetails();
  const close = (): void => {
    setIsOpen(false);
  };
  return (
    <div className="status-tray__item">
      <button
        type="button"
        className="status-tray__button"
        aria-label={t("status.together")}
        aria-expanded={isOpen}
        onClick={() => {
          setIsOpen(!isOpen);
        }}
      >
        <TogetherIcon />
      </button>
      {isOpen ? (
        <ModalDialog
          className="status-tray__panel modal-dialog--drop-down"
          label={t("watchTogether.heading")}
          onClose={close}
          shouldCloseOnOutsidePress
        >
          <div className="status-tray__panel-head">
            <h2>{t("watchTogether.heading")}</h2>
            <button type="button" onClick={close}>
              {t("status.close")}
            </button>
          </div>
          <Suspense fallback={<p>{t("common.loading")}</p>}>
            <WatchTogetherControls />
          </Suspense>
        </ModalDialog>
      ) : null}
    </div>
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
