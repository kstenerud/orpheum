// SPDX-License-Identifier: MPL-2.0

import "./status-tray.css";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";
import { WarningIcon } from "./warning-sign.tsx";
import { WatchTogetherControls } from "./watch-together.tsx";

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

/** Two heads facing each other: Watch Together, in the status blue. */
function TogetherIcon(): JSX.Element {
  return (
    <svg className="status-tray__icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="7" cy="8" r="3.3" />
      <path d="M10.1 7.4 11.6 9l-1.6.4z" />
      <path d="M1.2 21c0-4.4 2.6-7.2 5.8-7.2 2.2 0 3.9 1.3 4.8 3.4V21z" />
      <circle cx="17" cy="8" r="3.3" />
      <path d="M13.9 7.4 12.4 9l1.6.4z" />
      <path d="M22.8 21c0-4.4-2.6-7.2-5.8-7.2-2.2 0-3.9 1.3-4.8 3.4V21z" />
    </svg>
  );
}

/**
 * The Watch Together sign, which opens the session's controls over whatever screen is shown: a
 * modal drop-down, the page behind it taking no press, so a control under it half hidden is never
 * pressed by mistake on a small screen; a press anywhere outside it, Escape or Close shuts it (the
 * owner, 2026-10-11).
 */
function TogetherStatus(): JSX.Element {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
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
          <WatchTogetherControls />
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
  const { rootCount, diskCount, waitingRootCount, unreadCount } = counts.value;
  if (rootCount + diskCount + waitingRootCount + unreadCount === 0) {
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
