// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { currentNotice, dismissNotice, showNotice } from "./notice.ts";
import { NoticeNote } from "./notice-note.tsx";
import { buttonWithText, click, mount, type Mounted } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  const left = currentNotice();
  if (left !== undefined) {
    dismissNotice(left);
  }
});

// A page's word for the person on the screen they were taken to, such as why their Watch Together
// session ended (the owner, 2026-10-29).
describe("NoticeNote", () => {
  // The owner, 2026-10-29: the notice that a Watch Together session ended "is a modal popup", not
  // the note at the top of the page.
  it("shows the notice as a modal pop-up, until OK", async () => {
    showNotice("The host ended the Watch Together session.");
    mounted = await mount(withServices(<NoticeNote />, testServices("token", "/")));
    const dialog = mounted.container.querySelector('dialog[role="alertdialog"]');
    expect(dialog?.hasAttribute("open")).toBe(true);
    expect(dialog?.getAttribute("aria-label")).toBe("The host ended the Watch Together session.");
    expect(dialog?.textContent).toContain("The host ended the Watch Together session.");
    expect(mounted.container.querySelector("output")).toBeNull();
    const ok = buttonWithText(mounted.container, "OK");
    expect(ok).toBeDefined();
    await click(ok);
    expect(mounted.container.textContent).toBe("");
    expect(currentNotice()).toBeUndefined();
  });

  it("closes at a press outside it", async () => {
    showNotice("The host ended the Watch Together session.");
    mounted = await mount(withServices(<NoticeNote />, testServices("token", "/")));
    const dialog = mounted.container.querySelector("dialog");
    // On the backdrop: the press reaches the dialog itself, at a point outside its box.
    await act(async () => {
      dialog?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: -10, clientY: -10 }));
      await Promise.resolve();
    });
    expect(mounted.container.querySelector("dialog")).toBeNull();
    expect(currentNotice()).toBeUndefined();
  });

  it("shows one given while it is on the page", async () => {
    mounted = await mount(withServices(<NoticeNote />, testServices("token", "/")));
    expect(mounted.container.textContent).toBe("");
    await act(async () => {
      showNotice("You left the Watch Together session.");
      await Promise.resolve();
    });
    expect(mounted.container.querySelector("dialog")?.textContent).toContain(
      "You left the Watch Together session.",
    );
  });

  it("goes once the screen changes", async () => {
    const services = testServices("token", "/");
    showNotice("The host removed you from the Watch Together session.");
    mounted = await mount(withServices(<NoticeNote />, services));
    expect(mounted.container.textContent).toContain("removed you");
    await act(async () => {
      services.navigation.go({ screen: "films" });
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toBe("");
    expect(currentNotice()).toBeUndefined();
    // Back on the screen it was shown on, it does not come back.
    await act(async () => {
      services.navigation.go({ screen: "home" });
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toBe("");
  });
});
