// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Play } from "./play.tsx";
import { mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

// The owner's Fire TV pressed Back from a page and landed on a Play page in its history from before,
// which started a film watched earlier (staging, 2026-10-30).
describe("a Play page the browser's Back returns to (D383 §1)", () => {
  it("shows its title's page in its place, which the page noted once its playback said it", async () => {
    const services = servedBy({
      "POST /playback": [
        200,
        directDecision({
          resumePositionMs: 0,
          titleName: "The Paper Lantern",
          plays: { itemId: 10, kind: "movie", libraryId: 1, libraryName: "Films" },
        }),
      ],
      "GET /files/2/waits": [200, {}],
      "GET /files/2/up-next": [200, {}],
      "DELETE /playback/t": [204, null],
    });
    services.session.signIn("token", { deviceId: 5 });
    const { navigation } = services;
    navigation.go({ screen: "play", fileId: 2 });
    mounted = await mount(withServices(<Play fileId={2} />, services));
    const shown = mounted;
    await settleUntil(() => shown.container.querySelector("video") !== null);
    expect(navigation.notedInPlace()).toEqual({ screen: "title", itemId: 10 });
    navigation.go({ screen: "home" });
    expect(navigation.back()).toBe(true);
    expect(navigation.route()).toEqual({ screen: "title", itemId: 10 });
  });
});
