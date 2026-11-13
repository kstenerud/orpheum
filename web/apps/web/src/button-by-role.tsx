// SPDX-License-Identifier: MPL-2.0

import type { ComponentPropsWithRef, JSX, KeyboardEvent } from "react";

/**
 * A button by role, not a `<button>`: a `<div role="button">` in Tab's order, the one way to draw a
 * button a key moves onto on a page of a long list (the alphabet rail's letters, a pop-up's opener
 * and its choices, a list's chips, a poster still to come). Silk on a Fire TV lays the whole page
 * out each time a key focuses a `<button>`, every poster `content-visibility` skipped included: a
 * move onto one took 300 ms, the first after a load 5 s, and the same place as a button by role
 * 47 ms, a link's (the 11-24 batch's item 12; idiosyncrasies.md, "Silk lays the page out each time
 * a key focuses a button").
 *
 * It is pressed by a click, as a `<button>` is: a pointer's; OK's and a key's Enter, which the input
 * layer's pressFocused clicks (focus-navigation.ts), a `<button>`'s Enter being the browser's own;
 * and Space, here. It takes everything a `<div>` takes but its role and its keys; tabIndex is 0
 * unless given, -1 for one a roving Tab stop leaves out. Its look is a `<button>`'s
 * (base.css's `button, :where([role="button"])`).
 */
export function ButtonByRole(
  props: Omit<ComponentPropsWithRef<"div">, "role" | "onKeyDown">,
): JSX.Element {
  const { tabIndex, ...rest } = props;
  return (
    // eslint-disable-next-line jsx-a11y-x/prefer-tag-over-role -- a <button> focused by a key costs Silk on a Fire TV 300 ms of layout a move, 5 s the first (idiosyncrasies.md, "Silk lays the page out each time a key focuses a button")
    <div {...rest} role="button" tabIndex={tabIndex ?? 0} onKeyDown={pressedBySpace} />
  );
}

/** A `<button>`'s Space: a click, the page not scrolled by it. */
function pressedBySpace(event: KeyboardEvent<HTMLDivElement>): void {
  if (event.key === " ") {
    event.preventDefault();
    event.currentTarget.click();
  }
}
