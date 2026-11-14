// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { AutoplaySetting } from "./autoplay-setting.tsx";
import { MeContext, type Me } from "./signed-in-me.ts";
import { click, mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** Who is signed in, carrying the person's own switches (D480 §2), autoplay as given. */
function signedIn(isAutoplayOn: boolean): Me {
  return {
    id: 1,
    name: "owner",
    role: "user",
    isAdmin: false,
    canManageAccount: true,
    canHostWatchTogether: false,
    hasNoLibrary: false,
    showsDebugTools: false,
    preferences: {
      isDialogueBoostOn: false,
      isLevellingOn: true,
      isAutoplayOn,
      leftOutLibraryIds: [],
      leftOutOfHomeLibraryIds: [],
    },
  };
}

/** The setting's checkbox, once drawn. */
function boxNow(): HTMLInputElement | null | undefined {
  return mounted?.container.querySelector<HTMLInputElement>('input[type="checkbox"]');
}

describe("the person's autoplay (plans/up-next.md)", () => {
  it("shows the server's setting and keeps a change there, drawn again from its answer", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <AutoplaySetting />,
        servedBy(
          {
            "GET /me": [
              [200, signedIn(true)],
              [200, signedIn(false)],
            ],
            "PUT /me/preferences": [204, {}],
          },
          heard,
        ),
      ),
    );
    await settleUntil(() => boxNow() !== null);
    expect(mounted.container.textContent).toContain("Play the next episode automatically");
    expect(boxNow()?.checked).toBe(true);
    await click(boxNow());
    await settleUntil(() => boxNow()?.checked === false);
    expect(boxNow()?.checked).toBe(false);
    expect(heard.find((request) => request.methodAndPath === "PUT /me/preferences")?.body).toEqual({
      isAutoplayOn: false,
    });
    // Drawn alone, it reads who is signed in, and again after the change: no choices of its own.
    expect(heard.map((request) => request.methodAndPath)).toEqual([
      "GET /me",
      "PUT /me/preferences",
      "GET /me",
    ]);
  });

  it("takes the person's choices from who the shell read as signed in, asking for nothing to show them, and has it read again after a change (D480 §2)", async () => {
    const heard: Heard[] = [];
    let readsAgain = 0;
    const me = { state: "loaded", value: signedIn(true) } as const;
    mounted = await mount(
      withServices(
        <MeContext
          value={{
            me,
            readMeAgain: () => {
              readsAgain += 1;
            },
          }}
        >
          <AutoplaySetting />
        </MeContext>,
        servedBy({ "PUT /me/preferences": [204, {}] }, heard),
      ),
    );
    await settleUntil(() => boxNow() !== null);
    expect(boxNow()?.checked).toBe(true);
    expect(heard).toEqual([]);
    await click(boxNow());
    await settleUntil(() => readsAgain === 1);
    expect(heard.map((request) => request.methodAndPath)).toEqual(["PUT /me/preferences"]);
  });
});
