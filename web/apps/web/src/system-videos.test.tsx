// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { DebugToolsContext } from "./debug-tools.tsx";
import { System } from "./system.tsx";
import { SystemVideos } from "./system-videos.tsx";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const made = {
  nodeName: "nas",
  folder: "/var/orpheum/system-videos",
  videos: [
    {
      purpose: "sound-timing",
      codec: "h264-aac",
      fileName: "sound-timing.h264-aac.mp4",
      isReady: true,
      fileId: 77,
      sizeBytes: 2048,
    },
  ],
};

describe("the server's own videos on the System page (D435 §7)", () => {
  it("lists each video and where it is kept, and makes them again on the admin's word", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <SystemVideos />,
        servedBy(
          { "GET /system/videos": [200, made], "POST /system/videos/rebuild": [204, undefined] },
          heard,
        ),
      ),
    );
    const page = mounted;
    await settleUntil(() => page.container.textContent.includes("sound-timing.h264-aac.mp4"));
    expect(page.container.textContent).toContain("Kept on nas in /var/orpheum/system-videos.");
    expect(page.container.textContent).toContain("sound-timing.h264-aac.mp4: made");
    await click(buttonWithText(page.container, "Make them again"));
    await settleUntil(() =>
      heard.some((request) => request.methodAndPath === "POST /system/videos/rebuild"),
    );
    await settleUntil(
      () => heard.filter((request) => request.methodAndPath === "GET /system/videos").length === 2,
    );
  });

  it("is a debug tool: the System page shows it only while Show debug tools is on", async () => {
    mounted = await mount(
      withServices(
        <DebugToolsContext value={{ isShown: true, onChanged: () => undefined }}>
          <System />
        </DebugToolsContext>,
        servedBy({
          "GET /system": [500, { title: "down" }],
          "GET /backups": [500, { title: "down" }],
          "GET /system/resource-use": [500, { title: "down" }],
          "GET /config/unused": [200, { unusedSettings: [] }],
          "GET /filing-reports": [200, { reports: [] }],
          "GET /system/discovery": [500, { title: "down" }],
          "GET /now-playing": [200, { playbacks: [] }],
          "GET /system/reachability": [500, { title: "down" }],
          "GET /system/videos": [200, made],
        }),
      ),
    );
    const page = mounted;
    await settleUntil(() => page.container.textContent.includes("The server's own videos"));
  });
});
