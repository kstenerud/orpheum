// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { sessionAirPlayGrants } from "./airplay.ts";
import { offerAirPlayHere, type AirPlayHere } from "./airplay-here.ts";
import { CastTargets } from "./cast-to.tsx";
import { LiveProvider } from "./live.tsx";
import {
  buttonWithText,
  click,
  mount,
  mountToRevisit,
  servedBy,
  unanswered,
  type Answers,
  type Heard,
  type Mounted,
  type Revisitable,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import type { Services } from "./services.tsx";

const grantPath = "/api/v1/bytes/media/g/grant/master.m3u8";

/** A film this tab offers AirPlay for, faked: it notes each start's address and each showing of its controls. */
function fakeHere(isPlaying = false): AirPlayHere & { readonly did: string[] } {
  const did: string[] = [];
  return {
    did,
    mediaToken: "media",
    grants: sessionAirPlayGrants,
    isPlaying: () => isPlaying,
    start: (url) => {
      did.push(`start ${url}`);
    },
    showControls: () => {
      did.push("controls");
    },
  };
}

describe("AirPlay in Cast's menu (D473, D523)", () => {
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
    grants: Answers[string] = [201, { masterPath: grantPath, expiresAt: "2026-11-14T12:00:00Z" }],
    mountTree: (tree: ReactNode) => Promise<Mounted> = mount,
  ): Promise<{ readonly heard: Heard[]; readonly closed: string[] }> {
    const heard: Heard[] = [];
    const closed: string[] = [];
    const services = servedBy(
      { "GET /me/devices": [200, { devices: [] }], "POST /airplay/grants": grants },
      heard,
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mountTree(
      withServices(
        <LiveProvider>
          <CastTargets
            film={() => undefined}
            onAirPlayPressed={() => {
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

  it("says the film plays on AirPlay, asking no grant, with AirPlay controls, which shows AirPlay's picker and no Stop (D523 §6)", async () => {
    const here = fakeHere(true);
    unoffer = offerAirPlayHere(here);
    const { heard } = await openMenu();
    const menu = mounted?.container ?? document.body;
    expect(heard.map((each) => each.methodAndPath)).not.toContain("POST /airplay/grants");
    expect(menu.querySelector("output")?.textContent).toBe("Playing on AirPlay");
    expect(buttonWithText(menu, "AirPlay")).toBeUndefined();
    expect(menu.textContent).not.toContain("Stop");
    await click(buttonWithText(menu, "AirPlay controls"));
    expect(here.did).toEqual(["controls"]);
  });

  it("closes the menu at AirPlay controls, for AirPlay's picker to show over the page (D523 §6)", async () => {
    const here = fakeHere(true);
    unoffer = offerAirPlayHere(here);
    const { closed } = await openMenu();
    await click(buttonWithText(mounted?.container ?? document.body, "AirPlay controls"));
    expect(closed).toEqual(["closed"]);
  });

  it("lists no device while the film plays on AirPlay, one cast at a time (D523 §7)", async () => {
    unoffer = offerAirPlayHere(fakeHere(true));
    await openMenu();
    const menu = mounted?.container ?? document.body;
    expect(menu.textContent).not.toContain("None of your other devices");
    expect(menu.textContent).toContain("Playing on AirPlay");
    mounted?.unmount();
    unoffer();
    unoffer = offerAirPlayHere(fakeHere(false));
    await openMenu();
    expect(mounted?.container.textContent).toContain("None of your other devices");
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

  it("asks a grant of its own each time the menu opens, never pressable on an earlier opening's (D473, D478)", async () => {
    unoffer = offerAirPlayHere(fakeHere());
    let menu: Revisitable | undefined;
    const granted = [201, { masterPath: grantPath, expiresAt: "2026-11-14T12:00:00Z" }] as const;
    await openMenu({}, [granted, unanswered], async (tree) => {
      menu = await mountToRevisit(tree);
      return menu;
    });
    const airPlay = (): HTMLElement | undefined =>
      buttonWithText(mounted?.container ?? document.body, "AirPlay");
    expect(airPlay()?.hasAttribute("disabled")).toBe(false);
    await menu?.leave();
    await menu?.comeBack();
    expect(airPlay()?.hasAttribute("disabled")).toBe(true);
  });
});
