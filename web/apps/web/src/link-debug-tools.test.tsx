// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { DebugToolsContext } from "./debug-tools.tsx";
import { LinkCapBadge, LinkDebugTools } from "./link-debug-tools.tsx";
import { LinkDebug } from "./link-debug.ts";
import { buttonWithText, choose, click, mount, type Mounted } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A tab's debug link keeping nothing, on the page's own clock. */
function aLinkDebug(): LinkDebug {
  return new LinkDebug(undefined, {
    nowMs: () => Date.now(),
    setTimeout: (run, ms) => setTimeout(run, ms),
    clearTimeout: (timer) => {
      clearTimeout(timer);
    },
  });
}

/** The player's debug link tools and the shell's badge, shown to a person who shows debug tools or not. */
async function shown(linkDebug: LinkDebug, isShown: boolean): Promise<Mounted> {
  mounted = await mount(
    withServices(
      <DebugToolsContext value={{ isShown, onChanged: () => undefined }}>
        <LinkCapBadge />
        <LinkDebugTools />
      </DebugToolsContext>,
      { ...testServices("token"), linkDebug },
    ),
  );
  return mounted;
}

// Chrome's DevTools throttling never reached the WebRTC carrier (VQ128's run).
describe("the debug link's tools (D380)", () => {
  it("cap the tab's download rate and drop its connection, for a person who shows debug tools", async () => {
    const linkDebug = aLinkDebug();
    const dropFor = vi.spyOn(linkDebug, "dropFor");
    const page = (await shown(linkDebug, true)).container;
    expect(page.textContent).not.toContain("Throttled");
    await choose(page.querySelector(".play__link-debug select"), "1000");
    expect(linkDebug.capKbps()).toBe(1000);
    // The page says so the whole time a cap is on (D380 §4).
    expect(page.textContent).toContain("Throttled to 1 Mb/s");
    await click(buttonWithText(page, "Drop the connection for 30 s"));
    expect(dropFor).toHaveBeenCalledWith(30_000);
    expect(page.textContent).toContain("Connection dropped for testing: back in 30 s");
    await choose(page.querySelector(".play__link-debug select"), "");
    expect(linkDebug.capKbps()).toBeUndefined();
    expect(page.textContent).not.toContain("Throttled");
  });

  it("are hidden from a person who does not show debug tools, though a cap they left on is still said", async () => {
    const linkDebug = aLinkDebug();
    linkDebug.setCap(500);
    const page = (await shown(linkDebug, false)).container;
    expect(page.querySelector(".play__link-debug")).toBeNull();
    expect(page.textContent).toBe("Throttled to 0.5 Mb/s");
  });
});
