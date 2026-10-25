// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { AutoplaySetting } from "./autoplay-setting.tsx";
import { click, mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The person's own switches as the server answers them, autoplay as given. */
function switches(isAutoplayOn: boolean): [number, object] {
  return [200, { isDialogueBoostOn: false, isLevellingOn: true, isAutoplayOn }];
}

describe("the person's autoplay (plans/up-next.md)", () => {
  it("shows the server's setting and keeps a change there, drawn again from its answer", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <AutoplaySetting />,
        servedBy(
          {
            "GET /me/preferences": [switches(true), switches(false)],
            "PUT /me/preferences": [204, {}],
          },
          heard,
        ),
      ),
    );
    const boxNow = (): HTMLInputElement | null | undefined =>
      mounted?.container.querySelector<HTMLInputElement>('input[type="checkbox"]');
    await settleUntil(() => boxNow() !== null);
    expect(mounted.container.textContent).toContain("Play the next episode automatically");
    expect(boxNow()?.checked).toBe(true);
    await click(boxNow());
    await settleUntil(() => boxNow()?.checked === false);
    expect(boxNow()?.checked).toBe(false);
    expect(heard.find((request) => request.methodAndPath === "PUT /me/preferences")?.body).toEqual({
      isAutoplayOn: false,
    });
  });
});
