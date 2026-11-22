// SPDX-License-Identifier: MPL-2.0

import "./banner.css";

import { useRef, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { focusTargetsIn } from "./focus-engine.ts";
import { IconButton } from "./icon-button.tsx";
import { AboutIcon, WarningIcon } from "./icons.tsx";
import { CloseIcon } from "./player-icons.tsx";

/** A banner's tone (D538): information, in the status blue, or a warning, the triangle's yellow. */
export type BannerTone = "information" | "warning";

/**
 * How a banner is put away, its one way (D538): a Dismiss, a quiet "Remind me tomorrow" in its
 * place, or nothing, where it goes by itself as its cause ends.
 */
export type BannerPutAway =
  | { readonly kind: "dismiss"; readonly onDismiss: () => void }
  | { readonly kind: "remind tomorrow"; readonly onRemind: () => void }
  | { readonly kind: "none" };

/** A notice a page's banners may show (Banners): its key, its tone and what it says and does. */
export interface BannerNotice {
  readonly key: string;
  readonly tone: BannerTone;
  /** Whether it came after the page was drawn, so a screen reader says it at once (role alert). */
  readonly isAnnounced?: boolean;
  /** Its words, a line or two. */
  readonly text: ReactNode;
  /** A link after its words: its words and where it goes. */
  readonly link?: { readonly label: string; readonly href: string };
  readonly putAway: BannerPutAway;
}

/** The most banners shown at once (D538); the rest wait until one goes. */
export const mostBannersShown = 2;

/**
 * Which of a page's notices its banners show (D538): warnings first, then information, each kind in
 * the order given, and at most two; the rest wait until one is put away or goes.
 */
export function bannersShown(notices: readonly BannerNotice[]): readonly BannerNotice[] {
  const warnings = notices.filter((notice) => notice.tone === "warning");
  const information = notices.filter((notice) => notice.tone === "information");
  return [...warnings, ...information].slice(0, mostBannersShown);
}

/**
 * A page's banners, at the top of its main area under the top bar, in the page's width (D538): the
 * notices bannersShown picks, each a Banner.
 */
export function Banners(props: { readonly notices: readonly BannerNotice[] }): JSX.Element | null {
  const shown = bannersShown(props.notices);
  if (shown.length === 0) {
    return null;
  }
  return (
    <div className="banners">
      {shown.map((notice) => (
        <Banner key={notice.key} notice={notice} />
      ))}
    </div>
  );
}

/**
 * A standing note across the top of a page (D538), not the answer to something the person just did:
 * its icon, so its tone is never told by colour alone, its words, a link after them, and its one
 * way to be put away. Put away, focus moves to the next thing in the page after it, never to
 * nothing.
 */
export function Banner(props: { readonly notice: BannerNotice }): JSX.Element {
  const { notice } = props;
  const ref = useRef<HTMLDivElement>(null);
  return (
    <div
      ref={ref}
      className={`banner banner--${notice.tone === "warning" ? "warning" : "information"}`}
      role={notice.isAnnounced === true ? "alert" : "note"}
    >
      <span className="banner__icon" aria-hidden="true">
        {notice.tone === "warning" ? <WarningIcon /> : <AboutIcon />}
      </span>
      <span className="banner__words">
        <span>{notice.text}</span>
        {notice.link === undefined ? null : (
          <a className="banner__link" href={notice.link.href}>
            {notice.link.label}
          </a>
        )}
      </span>
      <PutAway
        putAway={notice.putAway}
        onPutAway={() => {
          focusPastBanner(ref.current);
        }}
      />
    </div>
  );
}

/** A banner's way to be put away (BannerPutAway): its Dismiss, its Remind me tomorrow, or none. */
function PutAway(props: {
  readonly putAway: BannerPutAway;
  readonly onPutAway: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { putAway } = props;
  if (putAway.kind === "none") {
    return null;
  }
  if (putAway.kind === "remind tomorrow") {
    return (
      <button
        type="button"
        className="banner__later"
        onClick={() => {
          props.onPutAway();
          putAway.onRemind();
        }}
      >
        {t("banner.remindTomorrow")}
      </button>
    );
  }
  return (
    <IconButton
      label={t("banner.dismiss")}
      icon={<CloseIcon />}
      onClick={() => {
        props.onPutAway();
        putAway.onDismiss();
      }}
    />
  );
}

/** Moves focus to the first thing after banner in the page, as it is put away. */
function focusPastBanner(banner: HTMLElement | null): void {
  if (banner === null) {
    return;
  }
  const after = focusTargetsIn(document).find(
    (target) =>
      !banner.contains(target) &&
      (banner.compareDocumentPosition(target) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0,
  );
  after?.focus();
}
