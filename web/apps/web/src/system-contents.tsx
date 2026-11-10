// SPDX-License-Identifier: MPL-2.0

import "./system-words.ts";

import "./system.css";

import type { JSX, MouseEvent } from "react";
import { useTranslation } from "react-i18next";

/** One section the contents row leads to: its heading's id, and its name as the heading says it. */
export interface ContentsEntry {
  readonly headingId: string;
  readonly name: string;
}

/**
 * The System page's contents (ui-audit-2026-11-04.md §3, System): a row of links, one to each
 * section the page always has and each machine's, in the page's order. Following one moves focus
 * to its section's heading, which brings it into view, so the next Tab or arrow starts there.
 */
export function SystemContents(props: { readonly entries: readonly ContentsEntry[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <nav aria-label={t("system.contents")}>
      <ul className="system__contents">
        {props.entries.map((entry) => (
          <li key={entry.headingId}>
            <a
              href={`#${entry.headingId}`}
              onClick={(event) => {
                goToHeading(event, entry.headingId);
              }}
            >
              {entry.name}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

/**
 * Focuses the heading named, in place of following the link: the address keeps no fragment, which
 * the router would read. A heading takes focus from a script only (tabindex -1), never by Tab or by
 * an arrow (focus-engine.ts takes no element with a negative tabindex).
 */
function goToHeading(event: MouseEvent<HTMLAnchorElement>, headingId: string): void {
  const heading = document.getElementById(headingId);
  if (heading === null) {
    return;
  }
  event.preventDefault();
  heading.tabIndex = -1;
  heading.focus();
}
