// SPDX-License-Identifier: MPL-2.0

import "./status-tray.css";

import { createElement, useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Icon } from "./icon.tsx";
import { IconButton } from "./icon-button.tsx";
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
 * A screen playing, a phone beside it: casting to another device (D364), for the top bar's Cast and
 * the sending device's tray. The owner's choice (2026-10-27), in the colour of the text it sits in.
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

/**
 * Watch Together's controls, whose code loads as the sign appears, not with every page: with them,
 * the player's volume, and hls.js with it, was in the script every page loaded before it drew (the
 * audit of 2026-10-25, W1). Asked for with the sign, they show the moment the tray opens, as the
 * session's details do (the owner, 2026-10-12).
 */
type Controls = () => JSX.Element;

function useControlsCode(): Controls | undefined {
  const [controls, setControls] = useState<{ readonly shown: Controls } | undefined>(undefined);
  useEffect(() => {
    let isCurrent = true;
    void import("./watch-together.tsx").then((loaded) => {
      if (isCurrent) {
        setControls({ shown: loaded.WatchTogetherControls });
      }
    });
    return () => {
      isCurrent = false;
    };
  }, []);
  return controls?.shown;
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
  usePrefetchedHostedDetails();
  const WatchTogetherControls = useControlsCode();
  const close = (): void => {
    setIsOpen(false);
  };
  return (
    <div className="status-tray__item">
      <IconButton
        className="status-tray__button"
        label={t("status.together")}
        icon={<TogetherIcon />}
        isExpanded={isOpen}
        onClick={() => {
          setIsOpen(!isOpen);
        }}
      />
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
          {WatchTogetherControls === undefined ? (
            <p>{t("common.loading")}</p>
          ) : (
            createElement(WatchTogetherControls)
          )}
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
