// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { PlainHttpNote } from "./plain-http-note.tsx";
import { buttonWithText, click, mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

// A browser keeps the microphone, the clipboard and installing the app to secure pages; over plain
// http they only failed when tried, with no word why (D334 §1).
describe("PlainHttpNote", () => {
  it("says what plain http turns off, until dismissed", async () => {
    mounted = await mount(<PlainHttpNote isSecure={false} />);
    expect(mounted.container.textContent).toContain("plain http");
    expect(mounted.container.textContent).toContain("voice");
    await click(buttonWithText(mounted.container, "Dismiss"));
    expect(mounted.container.textContent).toBe("");
  });

  it("says nothing on a secure page", async () => {
    mounted = await mount(<PlainHttpNote isSecure={true} />);
    expect(mounted.container.textContent).toBe("");
  });
});
