// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { offerAirPlayHere, type AirPlayHere } from "./airplay.ts";
import { CastTargets } from "./cast-to.tsx";
import { LiveProvider } from "./live.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import type { Services } from "./services.tsx";

const grantPath = "/api/v1/bytes/media/g/grant/master.m3u8";

/** A film this tab offers AirPlay for, faked: it notes each start's address and each stop. */
function fakeHere(isPlaying = false): AirPlayHere & { readonly did: string[] } {
  const did: string[] = [];
  return {
    did,
    mediaToken: "media",
    isPlaying: () => isPlaying,
    start: (url) => {
      did.push(`start ${url}`);
    },
    stop: () => {
      did.push("stop");
    },
  };
}

describe("AirPlay in Cast's menu (D473)", () => {
  let mounted: Mounted | undefined;
  let unoffer: (() => void) | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    unoffer?.();
    unoffer = undefined;
  });

  /** Cast's menu mounted on services, closed by a note in closed; and the requests it made. */
  async function openMenu(
    more: Partial<Services> = {},
  ): Promise<{ readonly heard: Heard[]; readonly closed: string[] }> {
    const heard: Heard[] = [];
    const closed: string[] = [];
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [] }],
        "POST /airplay/grants": [201, { masterPath: grantPath, expiresAt: "2026-11-14T12:00:00Z" }],
      },
      heard,
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets
            film={() => undefined}
            onAirPlayStarted={() => {
              closed.push("closed");
            }}
          />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket, ...more },
      ),
    );
    return { heard, closed };
  }

  it("asks for the grant as the menu opens, and the press hands the film to AirPlay at the grant's address and closes the menu", async () => {
    const here = fakeHere();
    unoffer = offerAirPlayHere(here);
    const { heard, closed } = await openMenu();
    const asked = heard.filter((each) => each.methodAndPath === "POST /airplay/grants");
    expect(asked.map((each) => each.body)).toEqual([{ mediaToken: "media" }]);
    await click(buttonWithText(mounted?.container ?? document.body, "AirPlay"));
    expect(here.did).toEqual([`start ${new URL(grantPath, globalThis.location.origin).href}`]);
    expect(closed).toEqual(["closed"]);
  });

  it("offers Stop AirPlay while the film plays on AirPlay, asking no grant, and Stop hands it back", async () => {
    const here = fakeHere(true);
    unoffer = offerAirPlayHere(here);
    const { heard } = await openMenu();
    expect(heard.map((each) => each.methodAndPath)).not.toContain("POST /airplay/grants");
    expect(buttonWithText(mounted?.container ?? document.body, "AirPlay")).toBeUndefined();
    await click(buttonWithText(mounted?.container ?? document.body, "Stop AirPlay"));
    expect(here.did).toEqual(["stop"]);
  });

  it("offers no AirPlay where this tab plays nothing that AirPlay is offered for", async () => {
    const { heard } = await openMenu();
    expect(mounted?.container.textContent).not.toContain("AirPlay");
    expect(heard.map((each) => each.methodAndPath)).not.toContain("POST /airplay/grants");
  });

  it("offers no AirPlay where the page reaches the core over WebRTC alone, which no receiver can", async () => {
    unoffer = offerAirPlayHere(fakeHere());
    const pathMoves = {
      directOrigin: () => undefined,
      isOverWebrtc: () => true,
    } as unknown as NonNullable<Services["pathMoves"]>;
    const { heard } = await openMenu({ pathMoves });
    expect(mounted?.container.textContent).not.toContain("AirPlay");
    expect(heard.map((each) => each.methodAndPath)).not.toContain("POST /airplay/grants");
  });
});
