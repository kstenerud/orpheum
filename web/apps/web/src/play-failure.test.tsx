// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { sendFailuresTo } from "@orpheum/sdk";
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

  it("says the film could not play, not a prepared copy, for a film on its ladder (the owner's test, 2026-10-18)", async () => {
    const services = servedBy({ "GET /files/2/availability": [200, {}] });
    const onLadder = {
      url: "/api/v1/bytes/media/chunks/t/master.m3u8",
      startAtBps: 1_896_000,
      qualityKbps: 1200,
      height: 480,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
      isReady: true,
      filmPictureKbps: 4000,
      linkSharePercent: 70,
    };
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={directDecision({ onLadder })}
          onDecideAgain={() => undefined}
        />,
        services,
      ),
    );
    await fire(mounted.container.querySelector("video"), new Event("error"));
    const alert = mounted.container.querySelector('[role="alert"]')?.textContent ?? "";
    expect(alert).toMatch(/^The film could not play: /u);
    expect(alert).not.toContain("prepared copy");
  });

  it("tells the server the player failed, in its own words (the owner's test, 2026-10-18)", async () => {
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    const services = servedBy({ "GET /files/2/availability": [200, {}] });
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    await fire(mounted.container.querySelector("video"), new Event("error"));
    expect(sent.some((text) => text.startsWith("Error: the player failed: "))).toBe(true);
    sendFailuresTo(() => undefined);
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
