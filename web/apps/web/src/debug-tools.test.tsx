// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { DebugTool, DebugToolsContext, DebugToolsSetting } from "./debug-tools.tsx";
import { click, mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { createNavigation } from "./navigation.ts";
import { fakePlace, testServices, withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The person's Show debug tools checkbox on the page, once drawn. */
function debugToolsBox(): HTMLInputElement | null | undefined {
  return [...(mounted?.container.querySelectorAll("label") ?? [])]
    .find((label) => label.textContent.includes("Show debug tools"))
    ?.querySelector<HTMLInputElement>('input[type="checkbox"]');
}

describe("debug tools (the owner, 2026-10-29: a per-person Show debug tools)", () => {
  it("are hidden where nothing says the person shows them, and while they do not", () => {
    const tool = <DebugTool>a debug readout</DebugTool>;
    expect(renderToStaticMarkup(withServices(tool, testServices("token")))).toBe("");
    const off = (
      <DebugToolsContext value={{ isShown: false, onChanged: () => undefined }}>
        {tool}
      </DebugToolsContext>
    );
    expect(renderToStaticMarkup(withServices(off, testServices("token")))).toBe("");
    const on = (
      <DebugToolsContext value={{ isShown: true, onChanged: () => undefined }}>
        {tool}
      </DebugToolsContext>
    );
    expect(renderToStaticMarkup(withServices(on, testServices("token")))).toBe("a debug readout");
  });

  it("keeps the person's change on the server, then has their choice read again", async () => {
    const heard: Heard[] = [];
    let readAgain = 0;
    mounted = await mount(
      withServices(
        <DebugToolsContext
          value={{
            isShown: false,
            onChanged: () => {
              readAgain += 1;
            },
          }}
        >
          <DebugToolsSetting />
        </DebugToolsContext>,
        servedBy({ "PUT /me/preferences": [204, {}] }, heard),
      ),
    );
    expect(mounted.container.textContent).toContain(
      "Shows extra controls and readouts for testing.",
    );
    expect(debugToolsBox()?.checked).toBe(false);
    await click(debugToolsBox());
    await settleUntil(() => readAgain === 1);
    expect(heard.find((request) => request.methodAndPath === "PUT /me/preferences")?.body).toEqual({
      showsDebugTools: true,
    });
  });

  it("follow the signed-in person's own choice, as GET /me says it, on every screen", async () => {
    for (const showsDebugTools of [false, true]) {
      mounted = await mount(
        withServices(<App />, {
          ...servedBy(
            {
              "GET /titles/letters": [200, { letters: [] }],
              "GET /client-settings": [200, {}],
              "GET /together/mine": [200, { sessions: [] }],
              "GET /me": [
                200,
                { id: 1, name: "owner", role: "user", isAdmin: false, showsDebugTools },
              ],
              "GET /me/devices": [200, { devices: [] }],
              "GET /me/own-link": [404, {}],
              "GET /me/preferences": [
                200,
                { isDialogueBoostOn: false, isLevellingOn: true, isAutoplayOn: true },
              ],
            },
            [],
            "token",
          ),
          navigation: createNavigation(fakePlace("/account")),
        }),
      );
      await settleUntil(() => debugToolsBox() !== undefined && debugToolsBox() !== null);
      await settleUntil(() => debugToolsBox()?.checked === showsDebugTools);
      expect(debugToolsBox()?.checked).toBe(showsDebugTools);
      mounted.unmount();
      mounted = undefined;
    }
  });
});
