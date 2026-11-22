// SPDX-License-Identifier: MPL-2.0

import "./help-button.css";

import type { JSX } from "react";

import { AboutIcon } from "./icons.tsx";
import { PopoverButton } from "./popover.tsx";

/**
 * A button that opens a few sentences of help most people do not need (D538, interface rule 3): an
 * icon button, ⓘ, whose press, tap, Enter or a remote's OK opens what it explains in a pop-up at it
 * (popover.tsx), and closes it again; nothing happens on a hover, which a phone and a television do
 * not have, and nothing is in a `title`. The pop-up closes at Escape, Back, a press outside or the
 * button again, focus going back to the button. The help is a plain paragraph, nothing to press:
 * focus is on the pop-up holding it, which a screen reader reads and a keyboard scrolls; nothing a person needs to use the
 * control is here: that is visible text. label says what it is about ("About subtitle delay"); help
 * is its paragraph, or its paragraphs, the short words first and more after them.
 */
export function HelpButton(props: {
  readonly label: string;
  readonly help: string | readonly string[];
}): JSX.Element {
  const paragraphs = typeof props.help === "string" ? [props.help] : props.help;
  return (
    <PopoverButton
      label={props.label}
      opener={<AboutIcon />}
      openerClassName="icon-button help-button"
      openerNaming={{ "aria-label": props.label }}
      isFromItsTop={false}
    >
      <div className="help-button__note">
        {paragraphs.map((paragraph) => (
          <p key={paragraph}>{paragraph}</p>
        ))}
      </div>
    </PopoverButton>
  );
}
