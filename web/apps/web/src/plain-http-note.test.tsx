// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { Banners } from "./banner.tsx";
import { plainHttpNoteDismissedKey, usePlainHttpNotice } from "./plain-http-note.tsx";
import { buttonNamed, click, mount, type Mounted } from "./test-dom.tsx";

/** The note as the shell draws it, an information Banner (D538 §1). */
function PlainHttpNote(props: { readonly isSecure: boolean }): JSX.Element {
  const notice = usePlainHttpNotice(props.isSecure);
  return <Banners notices={notice === undefined ? [] : [notice]} />;
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  localStorage.clear();
});

// A browser keeps the microphone, the clipboard and installing the app to secure pages; over plain
// http they only failed when tried, with no word why (D334 §1).
describe("PlainHttpNote", () => {
  it("says what plain http turns off, until dismissed", async () => {
    mounted = await mount(<PlainHttpNote isSecure={false} />);
    expect(mounted.container.textContent).toContain("plain http");
    expect(mounted.container.textContent).toContain("voice");
    await click(buttonNamed(mounted.container, "Dismiss"));
    expect(mounted.container.textContent).toBe("");
  });

  it("stays dismissed when the page is loaded again", async () => {
    mounted = await mount(<PlainHttpNote isSecure={false} />);
    await click(buttonNamed(mounted.container, "Dismiss"));
    expect(localStorage.getItem(plainHttpNoteDismissedKey)).toBe("1");
    mounted.unmount();
    mounted = await mount(<PlainHttpNote isSecure={false} />);
    expect(mounted.container.textContent).toBe("");
  });

  it("says nothing on a secure page", async () => {
    mounted = await mount(<PlainHttpNote isSecure={true} />);
    expect(mounted.container.textContent).toBe("");
  });
});
