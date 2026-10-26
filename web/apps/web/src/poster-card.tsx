// SPDX-License-Identifier: MPL-2.0

import "./poster-card.css";

import type { JSX, ReactNode } from "react";

import { Link } from "./link.tsx";
import { Poster } from "./poster.tsx";
import type { Route } from "./router.ts";

/**
 * One poster as every list shows it, the grid's, Continue Watching's and the home page's rows alike
 * (the owner, 2026-10-25: "make all poster views the same. Regardless of where they are, they show
 * the icon tray and the play status if mid-play. Then they all react the same way, fill in the same
 * way"): the poster, its name and the lines under it, a link to where it leads, the status cues'
 * tray over its top-right corner, and how far in it is watched as a bar along its bottom; and what a
 * list may add under it, Continue Watching's Hide. Its size is the list's: a grid's column, a row's
 * --size-poster-min.
 */
export function PosterCard(props: {
  readonly to: Route;
  readonly posterSha256: string | undefined;
  /**
   * Its art in place of the portrait poster made from posterSha256: an other video's 16:9 tile
   * (D349 §9.1), whose own Still already shows a named blank where it has none.
   */
  readonly art?: ReactNode;
  readonly name: string;
  /** The lines under the name: a year, a file's name, why it cannot play now. */
  readonly details?: ReactNode;
  /** How far in it is watched, a whole percent, as a bar along the poster's bottom. */
  readonly progress?: { readonly percent: number; readonly label: string } | undefined;
  /** Shown faded: one that cannot play now. */
  readonly isDimmed?: boolean;
  /** The status cues' tray, over the poster's top-right corner (StatusTray). */
  readonly cues?: ReactNode;
  /** What may be done with it beside opening it, under the card: Continue Watching's Hide. */
  readonly actions?: ReactNode;
  /** Watches the item, for a list that loads more as its end comes into view. */
  readonly watch?: ((element: Element | null) => void) | undefined;
}): JSX.Element {
  const { to, posterSha256, art, name, details, progress, isDimmed, cues, actions, watch } = props;
  const linkClass =
    isDimmed === true ? "poster-card__link poster-card__link--dimmed" : "poster-card__link";
  return (
    <li ref={watch} className="poster-card">
      <Link to={to} className={linkClass}>
        <span className="poster-card__art">
          {art ?? <Poster sourceSha256={posterSha256} />}
          {progress === undefined ? null : (
            <progress value={progress.percent} max={100} aria-label={progress.label} />
          )}
        </span>
        <span className="poster-card__name">{name}</span>
        {details}
      </Link>
      {cues}
      {actions}
    </li>
  );
}

/** One line under a poster's name, quieter than it: a year, a file, why it cannot play now. */
export function PosterDetail(props: {
  readonly children: ReactNode;
  readonly title?: string | undefined;
}): JSX.Element {
  return (
    <span className="poster-card__detail" title={props.title}>
      {props.children}
    </span>
  );
}
