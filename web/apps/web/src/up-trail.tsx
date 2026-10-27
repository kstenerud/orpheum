// SPDX-License-Identifier: MPL-2.0

import "./up-trail.css";

import type { JSX, MouseEvent, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { useBackTarget } from "./input-layer.tsx";
import { routeHref } from "./router.ts";
import { useServices } from "./services.tsx";
import { isTelevision } from "./tv-device.ts";
import { parentOf, type Crumb, type LibraryContext, type Upward } from "./upward.ts";

/**
 * A link up to an ancestor of the page (D367 §7, research R5): back to the nearest entry behind that
 * shows it, else it in place of the page; never a new entry, so Back after it never returns down.
 */
export function UpLink(props: {
  readonly crumb: Crumb;
  readonly className?: string;
  readonly children: ReactNode;
}): JSX.Element {
  const { navigation } = useServices();
  const { crumb } = props;
  const follow = (event: MouseEvent<HTMLAnchorElement>): void => {
    const isPlain =
      event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
    if (isPlain) {
      event.preventDefault();
      navigation.up(crumb.to, crumb.isIt);
    }
  };
  return (
    <a href={routeHref(crumb.to)} className={props.className} onClick={follow}>
      {props.children}
    </a>
  );
}

/**
 * Where the page is and the way up (D367 §8): on a computer or a phone a trail, "Series › Copper Kettle
 * › S1E1 Pilot", each ancestor a link up to it, the one before the last the parent; on a television
 * one "‹ Copper Kettle", one stop for focus. A top page has none. A remote's Back with nothing of the
 * app behind goes to the parent too (D367 §5), as does a television's Back at the base laid under a
 * page it opened on.
 */
export function UpTrail(props: { readonly upward: Upward }): JSX.Element | null {
  const parent = parentOf(props.upward);
  return parent === undefined ? null : <ShownTrail upward={props.upward} parent={parent} />;
}

/** The trail of a page that has a parent. */
function ShownTrail(props: { readonly upward: Upward; readonly parent: Crumb }): JSX.Element {
  const { t } = useTranslation();
  const { parent, upward } = props;
  useBackTarget(parent.to);
  if (isTelevision(navigator.userAgent)) {
    return (
      <nav className="up-trail" aria-label={t("trail.label")}>
        <UpLink crumb={parent} className="up-trail__up">
          {t("trail.up", { name: parent.label })}
        </UpLink>
      </nav>
    );
  }
  return (
    <nav className="up-trail" aria-label={t("trail.label")}>
      <ol className="up-trail__crumbs">
        {upward.crumbs.map((crumb) => (
          <li key={routeHref(crumb.to)}>
            <UpLink crumb={crumb}>{crumb.label}</UpLink>
          </li>
        ))}
        <li aria-current="page">{upward.here}</li>
      </ol>
    </nav>
  );
}

/** What a page needs to name its crumbs and find them behind it in the app's history. */
export function useLibraryContext(): LibraryContext {
  const { t } = useTranslation();
  const { navigation } = useServices();
  return { t, behind: navigation.behind() };
}
