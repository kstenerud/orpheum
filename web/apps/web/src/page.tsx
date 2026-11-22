// SPDX-License-Identifier: MPL-2.0

import "./page.css";

import { useId, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Section } from "./section.tsx";
import { UpTrail } from "./up-trail.tsx";
import type { Upward } from "./upward.ts";

/**
 * The frame of a routed screen (standards/ui.md, the parts; D538 §1): the way up (`UpTrail`, D367
 * §8), the page's one `h1` with the page's actions at the end of its line, the page's job in one
 * line, its one-line status where it has one, then its "Needs you" section while anything needs the
 * person (rule 13, problems first), then its content, each block a `Section`. No wider than
 * `--size-page`, its gutter included, in the middle of a wider window; narrower than 36rem of its
 * own width, the actions take a line of their own under the heading. Focus meets the trail, then
 * the actions, then the content, in the order drawn.
 */
export function Page(props: {
  /** The page's heading, its `h1`. */
  readonly heading: string;
  /** Where the page sits, for its trail up; a top page (Home, Libraries) has none. */
  readonly upward?: Upward | undefined;
  /** The page's actions, beside its heading. */
  readonly actions?: ReactNode;
  /** What the page is for, in one line under the heading ("Is the server healthy…"). */
  readonly job?: string | undefined;
  /** The page's one-line status (Update's, Network's), which comes before Needs you. */
  readonly status?: ReactNode;
  /**
   * What needs the person on this page, each item with its fix or a link to it: undefined while
   * nothing does, and then the section is not drawn at all.
   */
  readonly needsYou?: ReactNode;
  readonly children?: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const headingId = useId();
  return (
    <article className="page" aria-labelledby={headingId}>
      {props.upward === undefined ? null : <UpTrail upward={props.upward} />}
      <header className="page__header">
        <div className="page__heading-row">
          <h1 id={headingId} className="page__heading">
            {props.heading}
          </h1>
          {props.actions === undefined ? null : (
            <div className="page__actions">{props.actions}</div>
          )}
        </div>
        {props.job === undefined ? null : <p className="page__job">{props.job}</p>}
      </header>
      {props.status}
      {props.needsYou === undefined ? null : (
        <Section heading={t("page.needsYou")}>{props.needsYou}</Section>
      )}
      {props.children}
    </article>
  );
}
