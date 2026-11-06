// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, useLayoutEffect, useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { useLatest } from "./latest.ts";
import { mount, type Mounted } from "./test-dom.tsx";

/**
 * A value shown by a press of its button, kept in useLatest and read as the drawing that shows it runs
 * its layout effects, before anything after the drawing runs: an event as soon after the press as one
 * can come.
 */
function Shown(props: { readonly read: string[] }): JSX.Element {
  const { read } = props;
  const [shown, setShown] = useState("hidden");
  const shownRef = useLatest(shown);
  useLayoutEffect(() => {
    if (shown === "full") {
      read.push(shownRef.current);
    }
  }, [shown, shownRef, read]);
  return (
    <button
      type="button"
      onClick={() => {
        setShown("full");
      }}
    >
      {shown}
    </button>
  );
}

describe("useLatest", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  // A Fire TV's Back just after Up showed the player's controls was heard by the handler of the
  // controls still hidden (the overlay's browser test, 2026-11-05).
  it("holds the drawn page's value before the page's passive effects run", async () => {
    const read: string[] = [];
    mounted = await mount(<Shown read={read} />);
    act(() => {
      mounted?.container.querySelector("button")?.click();
    });
    expect(mounted.container.textContent).toBe("full");
    expect(read).toEqual(["full"]);
  });
});
