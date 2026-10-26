// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { NowPlaying } from "./now-playing.tsx";
import { enter, mount, servedBy, submit, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const playing = {
  id: 3,
  userId: 2,
  personName: "partner",
  deviceName: "Firefox on Linux",
  fileId: 7,
  name: "Metropolis",
  delivery: "hls",
  profile: "h264",
  notDirectReasons: ["container"],
  positionMs: 60_000,
  startedAt: 1_790_000_000_000,
  lastHeardAt: 1_790_000_060_000,
};

it("shows who plays what, where and how, and stops one with a message (D243 §9)", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <NowPlaying />,
      servedBy(
        {
          "GET /now-playing": [
            [200, { playbacks: [playing] }],
            [200, { playbacks: [] }],
          ],
          "POST /now-playing/3/stop": [204, null],
        },
        heard,
      ),
    ),
  );
  const shown = mounted.container.textContent;
  expect(shown).toContain("partner on Firefox on Linux: Metropolis");
  expect(shown).toContain("Converted, because of: container.");
  const form = mounted.container.querySelector("form");
  await enter(form?.querySelector("input"), "Back in five minutes");
  await submit(form);
  expect(heard.find((each) => each.methodAndPath === "POST /now-playing/3/stop")?.body).toEqual({
    message: "Back in five minutes",
  });
  expect(mounted.container.textContent).toContain("Nothing is playing.");
});
