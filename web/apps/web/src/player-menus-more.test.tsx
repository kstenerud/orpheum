// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { QualityChoices } from "./data-cap-choices.tsx";
import { DataCaps, dataCapsKey, deviceDataCaps } from "./data-cap.ts";
import { DebugToolsContext } from "./debug-tools.tsx";

import { ListBox } from "./list-box.tsx";
import type { Navigation } from "./navigation.ts";
import { PlayerMenus, PlayerMenusContext } from "./player-menus.tsx";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import { fire, mount, type Mounted } from "./test-dom.tsx";
import { FakeConnection, heldStorage } from "./test-data-cap.ts";
import { testServices, withServices } from "./test-services.tsx";
import {
  tracksNoting,
  ways,
  everyMenu,
  drawnMenus,
  opened,
  places,
  choosing,
  videoOnTheDefaultLadder,
  drawnOnTheLadder,
  focusedText,
  currentPlaces,
  offered,
} from "./test-player-menus.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The overlay's menus over tracks, the drawing this file's afterEach unmounts. */
async function drawn(tracks: PlayerTracks, told: boolean[] = []): Promise<Mounted> {
  mounted = await drawnMenus(tracks, told);
  return mounted;
}

/** ⁝ over a film on the core's default rungs, the drawing this file's afterEach unmounts. */
async function drawnLadder(services = testServices()): Promise<Mounted> {
  mounted = await drawnOnTheLadder(services);
  return mounted;
}

describe("⁝'s data use (D488)", () => {
  afterEach(() => {
    window.localStorage.removeItem(dataCapsKey);
  });

  it("lists Automatic, then one choice per rung the film is offered, highest first, in GB per hour, then the original (D538 §10)", async () => {
    mounted = await mount(
      withServices(
        <DebugToolsContext value={{ isShown: true, onChanged: () => undefined }}>
          <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
            <PlayerMenusContext value={{ ...ways, onOriginalChosen: () => undefined }}>
              <PlayerMenus
                videoRef={videoOnTheDefaultLadder()}
                shown={everyMenu}
                onOpenChange={() => undefined}
              />
            </PlayerMenusContext>
          </PlayerTracksContext>
        </DebugToolsContext>,
      ),
    );
    await opened(mounted, "Settings");
    // The original is Quality's, chosen in its list, not ⁝'s own (D538 §10).
    expect(places(mounted)).not.toContain("Play the original file");
    await choosing(mounted, "Quality: Automatic›");
    const perHour = ["2.7", "1.4", "0.9", "0.6", "0.4", "0.2"].map((gb) => `${gb} GB per hour`);
    expect(places(mounted)).toEqual(expect.arrayContaining(["‹Quality", "Automatic", ...perHour]));
    const listed = places(mounted);
    expect(listed.slice(listed.indexOf("‹Quality"), listed.indexOf("‹Quality") + 9)).toEqual([
      "‹Quality",
      "Automatic",
      ...perHour,
      "Play the original file",
    ]);
    expect(currentPlaces(mounted)).toContain("Automatic");
    await choosing(mounted, "0.6 GB per hour");
    expect(JSON.parse(window.localStorage.getItem(dataCapsKey) ?? "")).toEqual({ unknown: 1296 });
    await opened(mounted, "Settings");
    await choosing(mounted, "Quality: 0.6 GB per hour›");
    expect(currentPlaces(mounted)).toEqual(["0.6 GB per hour"]);
    // A capped device plays a rung, never the film's own file (the owner, 2026-11-14).
    expect(places(mounted)).not.toContain("Play the original file");
  });

  it("offers the original in Quality only while the person shows debug tools, and plays it for this playback (D538 §10)", async () => {
    const chosen: string[] = [];
    const menus = (isShown: boolean): JSX.Element => (
      <DebugToolsContext value={{ isShown, onChanged: () => undefined }}>
        <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
          <PlayerMenusContext value={{ ...ways, onOriginalChosen: () => chosen.push("original") }}>
            <PlayerMenus
              videoRef={videoOnTheDefaultLadder()}
              shown={everyMenu}
              onOpenChange={() => undefined}
            />
          </PlayerMenusContext>
        </PlayerTracksContext>
      </DebugToolsContext>
    );
    mounted = await mount(withServices(menus(false)));
    await opened(mounted, "Settings");
    await choosing(mounted, "Quality: Automatic›");
    expect(places(mounted)).not.toContain("Play the original file");
    mounted.unmount();
    mounted = await mount(withServices(menus(true)));
    await opened(mounted, "Settings");
    await choosing(mounted, "Quality: Automatic›");
    await choosing(mounted, "Play the original file");
    expect(chosen).toEqual(["original"]);
    expect(mounted.container.querySelector("dialog")).toBeNull();
    // Chosen for one playback: no cap is kept by it.
    expect(window.localStorage.getItem(dataCapsKey)).toBeNull();
  });

  it("orders ⁝ as the spec does: Start over, Quality, then Report a problem and How it plays last, the subtitles' timing gone to Subtitles", async () => {
    mounted = await mount(
      withServices(
        <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
          <PlayerMenusContext
            value={{ ...ways, playback: { delivery: "converted", qualityKbps: 4200 } }}
          >
            <PlayerMenus
              videoRef={videoOnTheDefaultLadder()}
              shown={everyMenu}
              onOpenChange={() => undefined}
            />
          </PlayerMenusContext>
        </PlayerTracksContext>,
      ),
    );
    await opened(mounted, "Settings");
    const shown = places(mounted);
    const at = (start: string): number => shown.findIndex((place) => place.startsWith(start));
    expect(at("Start over")).toBe(0);
    expect(at("Quality")).toBe(1);
    expect(at("Subtitle timing")).toBe(-1);
    expect(at("Report a problem")).toBe(2);
    expect(shown.at(-1)).toBe("Playing: Converted 4.2 Mb/s");
  });

  it("offers nothing while the film plays no ladder, its own file", async () => {
    await drawn(tracksNoting({ subtitles: [], audio: [] }));
    await opened(mounted!, "Settings");
    expect(places(mounted!).filter((place) => place.startsWith("Quality"))).toEqual([]);
  });

  it("names the network its cap is for where the browser says which, and follows the device's move", async () => {
    const connection = new FakeConnection("cellular");
    const { storage } = heldStorage();
    const caps = new DataCaps(storage, connection);
    caps.choose(496);
    mounted = await mount(
      <ListBox isFromItsTop>
        <QualityChoices videoRef={videoOnTheDefaultLadder()} caps={caps} />
      </ListBox>,
    );
    const entry = (): string | null | undefined =>
      mounted?.container.querySelector(".list-box__submenu-entry")?.textContent;
    const heading = (): string | null | undefined =>
      mounted?.container.querySelector(".list-box__submenu-back")?.textContent;
    const current = (): (string | null)[] =>
      [...(mounted?.container.querySelectorAll('[aria-current="true"]') ?? [])].map(
        (place) => place.textContent,
      );
    expect(entry()).toBe("Quality on mobile data: 0.2 GB per hour›");
    await fire(
      mounted.container.querySelector(".list-box__submenu-entry"),
      new MouseEvent("click", { bubbles: true }),
    );
    expect(heading()).toBe("‹Quality on mobile data");
    expect(current()).toEqual(["0.2 GB per hour"]);
    act(() => {
      connection.moveTo("wifi");
    });
    expect(heading()).toBe("‹Quality on Wi-Fi");
    expect(current()).toEqual(["Automatic"]);
  });

  it("lists the film's own picture as one more rung, and offers its original under a cap at or above the file's rate (D491)", async () => {
    mounted = await mount(
      withServices(
        <DebugToolsContext value={{ isShown: true, onChanged: () => undefined }}>
          <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
            <PlayerMenusContext
              value={{ ...ways, fileBitRateKbps: 9096, onOriginalChosen: () => undefined }}
            >
              <PlayerMenus
                videoRef={videoOnTheDefaultLadder(8000)}
                shown={everyMenu}
                onOpenChange={() => undefined}
              />
            </PlayerMenusContext>
          </PlayerTracksContext>
        </DebugToolsContext>,
      ),
    );
    await opened(mounted, "Settings");
    await choosing(mounted, "Quality: Automatic›");
    const listed = places(mounted);
    // The own picture's 8000 kb/s, its sound's with it, is 3.6 GB an hour.
    expect(listed.slice(listed.indexOf("Automatic"), listed.indexOf("Automatic") + 3)).toEqual([
      "Automatic",
      "3.6 GB per hour",
      "2.7 GB per hour",
    ]);
    await choosing(mounted, "3.6 GB per hour");
    expect(JSON.parse(window.localStorage.getItem(dataCapsKey) ?? "")).toEqual({ unknown: 8000 });
    await opened(mounted, "Settings");
    await choosing(mounted, "Quality: 3.6 GB per hour›");
    expect(currentPlaces(mounted)).toEqual(["3.6 GB per hour"]);
    // The file's own 9096 kb/s, every track of it, is over the cap.
    expect(places(mounted)).not.toContain("Play the original file");
    act(() => {
      deviceDataCaps().choose(9096);
    });
    expect(places(mounted)).toContain("Play the original file");
  });
});

describe("⁝'s submenus (D490)", () => {
  afterEach(() => {
    window.localStorage.removeItem(dataCapsKey);
  });

  it("names Quality and its value on one entry, its list taking ⁝'s place at a click, the back row first, on its choice", async () => {
    const on = await drawnLadder();
    await opened(on, "Settings");
    expect(places(on)).toEqual(expect.arrayContaining(["Start over", "Quality: Automatic›"]));
    expect(places(on)).not.toContain("Automatic");
    await choosing(on, "Quality: Automatic›");
    expect(places(on)[0]).toBe("‹Quality");
    expect(places(on)).not.toContain("Start over");
    expect(focusedText()).toBe("Automatic");
    expect(on.container.querySelector(".list-box__submenu-back")?.getAttribute("aria-label")).toBe(
      "Back to Quality",
    );
    await choosing(on, "‹Quality");
    expect(on.container.querySelector("dialog")).not.toBeNull();
    expect(places(on)).toEqual(expect.arrayContaining(["Start over", "Quality: Automatic›"]));
    expect(places(on)).not.toContain("‹Quality");
    expect(focusedText()).toBe("Quality: Automatic›");
  });

  it("takes a TV's keys: OK or Right on the entry opens it, Left or Back in it returns to the entry, ⁝ open", async () => {
    const on = await drawnLadder();
    await opened(on, "Settings");
    const entry = on.container.querySelector<HTMLElement>(".list-box__submenu-entry");
    act(() => {
      entry?.focus();
    });
    await fire(entry, new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    expect(places(on)[0]).toBe("‹Quality");
    expect(focusedText()).toBe("Automatic");
    expect(offered(document.activeElement, "left")).toBe(true);
    expect(places(on)).toContain("Start over");
    expect(document.activeElement).toBe(entry);
    expect(offered(entry, "right")).toBe(true);
    expect(focusedText()).toBe("Automatic");
    expect(offered(document.activeElement, "back")).toBe(true);
    expect(on.container.querySelector("dialog")).not.toBeNull();
    expect(places(on)).toContain("Start over");
    expect(document.activeElement).toBe(entry);
  });

  // Silk on a Fire TV sends its remote's Back as the browser's, never a key (modal-layer.tsx).
  it("returns at the browser's Back, held over ⁝'s own while its list is open", async () => {
    const services = testServices();
    const holds: (() => void)[] = [];
    const navigation: Navigation = {
      ...services.navigation,
      holdBack: (onBack) => {
        holds.push(onBack);
        return services.navigation.holdBack(onBack);
      },
    };
    const on = await drawnLadder({ ...services, navigation });
    await opened(on, "Settings");
    await choosing(on, "Quality: Automatic›");
    expect(holds).toHaveLength(2);
    act(() => {
      holds[1]?.();
    });
    expect(on.container.querySelector("dialog")).not.toBeNull();
    expect(places(on)).toContain("Start over");
    expect(focusedText()).toBe("Quality: Automatic›");
  });
});
