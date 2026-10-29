// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { currentNotice, dismissNotice, showNotice } from "./notice.ts";
import { NoticeNote } from "./notice-note.tsx";
import { buttonNamed, click, mount, type Mounted } from "./test-dom.tsx";
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
  it("shows the notice as a status, until Close", async () => {
    showNotice("The host ended the Watch Together session.");
    mounted = await mount(withServices(<NoticeNote />, testServices("token", "/")));
    const status = mounted.container.querySelector("output");
    expect(status?.textContent).toBe("The host ended the Watch Together session.");
    const close = buttonNamed(mounted.container, "Close");
    expect(close).toBeDefined();
    await click(close);
    expect(mounted.container.textContent).toBe("");
    expect(currentNotice()).toBeUndefined();
  });

  it("shows one given while it is on the page", async () => {
    mounted = await mount(withServices(<NoticeNote />, testServices("token", "/")));
    expect(mounted.container.textContent).toBe("");
    await act(async () => {
      showNotice("You left the Watch Together session.");
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toBe("You left the Watch Together session.");
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
