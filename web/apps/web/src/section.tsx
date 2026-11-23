// SPDX-License-Identifier: MPL-2.0

import "./section.css";

import { useId, type JSX, type ReactNode } from "react";

import { WarningIcon } from "./icons.tsx";

/**
 * A named block of a page (standards/ui.md, the parts; D538 §1): its `h2` heading, the block's
 * actions at the end of the heading's line ("See all", "Back up now"), then its content. The heading
 * is never a focus stop; the actions are, before the content. Narrower than 36rem of its own
 * container, the actions take a line of their own under the heading. A heading with an action
 * beside it is always a Section, never a hand-made row.
 */
export function Section(props: {
  /** The heading's words. */
  readonly heading: string;
  /** The block's actions, buttons or links, in reading order: each at least a fingertip tall. */
  readonly actions?: ReactNode;
  /** The section's own id, where a link lands on it (`settings-section-<section>`). */
  readonly id?: string | undefined;
  /**
   * What its heading says beside its words, where it says anything: "warning", the warning icon
   * before them, as Needs attention's page sections have (D538 §12); the words say it too.
   */
  readonly mark?: "warning" | undefined;
  readonly children?: ReactNode;
}): JSX.Element {
  const headingId = useId();
  return (
    <section className="section" id={props.id} aria-labelledby={headingId}>
      <div className="section__header">
        <h2 id={headingId} className="section__heading">
          {props.mark === "warning" ? (
            <>
              <WarningIcon />{" "}
            </>
          ) : null}
          {props.heading}
        </h2>
        {props.actions === undefined ? null : (
          <div className="section__actions">{props.actions}</div>
        )}
      </div>
      {props.children}
    </section>
  );
}
