// SPDX-License-Identifier: MPL-2.0

/**
 * A modal dialog as a real browser lays it out (vitest's browser mode): modal-dialog.css alone keeps
 * the page behind it still while it is open (the owner, 2026-10-21).
 */

import "./tokens.css";
import "./base.css";

import { describe, expect, it } from "vitest";

import { ModalDialog } from "./modal-dialog.tsx";
import { mount } from "./test-dom.tsx";

describe("a modal dialog, laid out by a real browser", () => {
  it("keeps the page behind it from scrolling while it is open, and lets it scroll again once closed", async () => {
    const page = document.documentElement;
    const mounted = await mount(
      <>
        <div style={{ height: "5000px" }} />
        <ModalDialog label="What it is for" onClose={() => undefined}>
          <p>Short.</p>
        </ModalDialog>
      </>,
    );
    expect(getComputedStyle(page).overflow).toBe("hidden");
    expect(
      getComputedStyle(mounted.container.querySelector("dialog") ?? page).overscrollBehavior,
    ).toBe("contain");
    mounted.unmount();
    expect(getComputedStyle(page).overflow).not.toBe("hidden");
  });
});
