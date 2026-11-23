// SPDX-License-Identifier: MPL-2.0

import "./attention-list.css";

import type { JSX, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { attentionLine } from "./attention-lines.ts";
import type { AttentionItem } from "./attention-pages.ts";
import { WarningIcon } from "./icons.tsx";
import { Link } from "./link.tsx";

/**
 * What needs an admin on one page, one line to an item (D538 §12): a problem with the warning icon,
 * which the server puts before the notices, each line a link to the exact place it is fixed at. An
 * item of a kind this app does not know is skipped (D432 §2); more lines (a title's own) may follow.
 * The Needs attention page lists each page's so, and each page's Needs you its own.
 */
export function AttentionList(props: {
  readonly items: readonly AttentionItem[];
  /** Lines of the page's own that follow the items' (Titles to check's titles, by name). */
  readonly children?: ReactNode;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  return (
    <ul className="attention-list">
      {props.items.map((item, index) => {
        const line = attentionLine(t, item, i18n.language);
        if (line === undefined) {
          return null;
        }
        return (
          <li key={`${item.kind}-${String(index)}`}>
            {item.isProblem ? <WarningIcon /> : null} <Link to={line.to}>{line.text}</Link>
          </li>
        );
      })}
      {props.children}
    </ul>
  );
}
