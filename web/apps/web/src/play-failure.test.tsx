// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { PlayView } from "./play.tsx";
import { fire, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("a playback that fails (D241 §7)", () => {
  it("asks the server why, and says its file's folder is missing", async () => {
    const services = servedBy({
      "GET /files/2/availability": [
        200,
        { unavailable: { reason: "root_missing", nodeName: "devnas", path: "/media/films" } },
      ],
    });
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    await fire(mounted.container.querySelector("video"), new Event("error"));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "This cannot be played now: the folder /media/films on devnas is missing.",
    );
  });

  it("keeps the player's own words when the file can be read", async () => {
    const services = servedBy({ "GET /files/2/availability": [200, {}] });
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    await fire(mounted.container.querySelector("video"), new Event("error"));
    const alert = mounted.container.querySelector('[role="alert"]')?.textContent ?? "";
    expect(alert).not.toContain("cannot be played now");
    expect(alert).not.toBe("");
  });
});
