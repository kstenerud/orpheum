// SPDX-License-Identifier: MPL-2.0

import type { JSX, ReactNode } from "react";

import { ListBox } from "./list-box.tsx";
import { PopoverButton, type OpenerNaming } from "./popover.tsx";

/**
 * The one drop-down: a button that opens a list in a pop-up at it (popover.tsx, list-box.tsx), a
 * chooser's (route-chooser.tsx), the banner's menus (app-shell.tsx) and a page's ⁝ (more-actions.tsx)
 * alike, the owner, 2026-10-28: "the menu should share code with it, the only real difference being
 * that there's no extra visual indicator that it's a dropdown", and "the drop-down menu should be
 * modal". The pop-up closes at a choice, a press outside, Escape, Back, Left and Right, and carries
 * no Close (D408 §2). It opens at its button, a big screen's list its current choice laid over the
 * button as a select's is, a phone's a pop-up over the button (the owner, 2026-10-28: "the chooser
 * opening a window in a different location feels weird"), on its current choice with an entry
 * either side in view (D408 §3); a menu, which has no choice, from its top on its first entry
 * (isFromItsTop). Focus goes back to the button (D361 §3).
 */
export function DropDown(props: {
  /** The list's name for a screen reader. */
  readonly label: string;
  /** What the button shows: a chooser's choice and its chevron, the menu's icon. */
  readonly opener: ReactNode;
  readonly openerClassName: string;
  readonly openerNaming: OpenerNaming;
  /**
   * Whether the list opens showing its top, on its first entry, its top at the button's: a menu's,
   * which has no current choice to lay over its button, though the page it is on is marked.
   */
  readonly isFromItsTop?: boolean;
  /** Told each time the list opens or closes, as the player's overlay holds its time (D407 §6). */
  readonly onOpenChange?: ((isOpen: boolean) => void) | undefined;
  /** Its entries: ListOption and ListEntry (list-box.tsx). */
  readonly children: ReactNode;
}): JSX.Element {
  const isFromItsTop = props.isFromItsTop === true;
  return (
    <PopoverButton
      label={props.label}
      opener={props.opener}
      openerClassName={props.openerClassName}
      openerNaming={props.openerNaming}
      isFromItsTop={isFromItsTop}
      onOpenChange={props.onOpenChange}
    >
      <ListBox isFromItsTop={isFromItsTop}>{props.children}</ListBox>
    </PopoverButton>
  );
}
