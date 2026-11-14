// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ButtonByRole } from "./button-by-role.tsx";
import { useOpensAtThePointer, type PictureTap } from "./player-taps.ts";
import { mount, type Mounted } from "./test-dom.tsx";

/**
 * An overlay's ground with a button, a button by role (the subtitle pop-up's opener) and an open
 * pop-up on it, as the player's overlay lies over the film.
 */
function Overlay(props: {
  readonly open: () => void;
  readonly onPictureTap: (tap: PictureTap) => void;
}): JSX.Element {
  useOpensAtThePointer(props.open, { isHeld: () => false, onPictureTap: props.onPictureTap });
  return (
    <div className="player-overlay" data-player-controls="">
      <button type="button">Play</button>
      <ButtonByRole aria-label="Subtitles">CC</ButtonByRole>
      <dialog open>
        <p>Off</p>
      </dialog>
    </div>
  );
}

/** A finger's tap on target, and the click the browser sends after. */
function tap(target: Element): void {
  const init = { bubbles: true, pointerType: "touch", pointerId: 7, clientX: 50, clientY: 50 };
  act(() => {
    target.dispatchEvent(new PointerEvent("pointerdown", init));
    target.dispatchEvent(new PointerEvent("pointerup", init));
    target.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1, clientX: 50 }));
  });
}

// The owner, 2026-11-14 (VQ249): on a phone on its side, outside full screen, the overlay's scrim
// lay over three quarters of the film, and a tap there brought it up but never took it down.
describe("a finger's tap on the overlay (D415 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  async function drawn(): Promise<{ open: () => void; onPictureTap: (tap: PictureTap) => void }> {
    const answers = { open: vi.fn(), onPictureTap: vi.fn() };
    mounted = await mount(<Overlay {...answers} />);
    return answers;
  }

  it("is the picture's on the overlay's own ground, its scrim between its rows", async () => {
    const answers = await drawn();
    const ground = mounted?.container.querySelector(".player-overlay");
    if (!(ground instanceof Element)) {
      throw new Error("the overlay was not drawn");
    }
    tap(ground);
    expect(answers.onPictureTap).toHaveBeenCalledOnce();
    expect(answers.open).not.toHaveBeenCalled();
  });

  it("is the control's on a button, which opens as a click does", async () => {
    const answers = await drawn();
    const button = mounted?.container.querySelector("button");
    if (!(button instanceof Element)) {
      throw new Error("the button was not drawn");
    }
    tap(button);
    expect(answers.open).toHaveBeenCalledOnce();
    expect(answers.onPictureTap).not.toHaveBeenCalled();
  });

  // The owner, 2026-11-14: the subtitle button, a button by role, opened its pop-up, then the tap
  // counted as the picture's hid the overlay, and the pop-up, modal but drawn nowhere, held the page.
  it("is the control's on a button by role, as it is on a button", async () => {
    const answers = await drawn();
    const opener = mounted?.container.querySelector("[role='button']");
    if (!(opener instanceof Element)) {
      throw new Error("the button by role was not drawn");
    }
    tap(opener);
    expect(answers.open).toHaveBeenCalledOnce();
    expect(answers.onPictureTap).not.toHaveBeenCalled();
  });

  it("is the pop-up's anywhere inside an open one, never the picture's", async () => {
    const answers = await drawn();
    const inside = mounted?.container.querySelector("dialog p");
    if (!(inside instanceof Element)) {
      throw new Error("the pop-up was not drawn");
    }
    tap(inside);
    expect(answers.onPictureTap).not.toHaveBeenCalled();
  });
});
