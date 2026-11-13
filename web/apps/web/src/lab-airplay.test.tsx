// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AirPlayControls } from "./lab-airplay.tsx";
import { useLog } from "./lab-log.ts";
import { createPathMoves, type PathMoves } from "./services.tsx";
import {
  bodyField,
  buttonNamed,
  click,
  fire,
  mount,
  refusal,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

/** The elements Safari's picker was shown for, in order. */
let pickedOn: HTMLMediaElement[] = [];

beforeEach(() => {
  pickedOn = [];
  // Safari's own, which happy-dom has not.
  Object.defineProperty(HTMLMediaElement.prototype, "webkitShowPlaybackTargetPicker", {
    configurable: true,
    value(this: HTMLMediaElement) {
      pickedOn.push(this);
    },
  });
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  Reflect.deleteProperty(HTMLMediaElement.prototype, "webkitShowPlaybackTargetPicker");
});

const masterPath = "/api/v1/bytes/media/g/the-grant/master.m3u8";
const made = { masterPath, expiresAt: "2026-11-13T21:00:00Z" };

/** The controls for a playback, media-1, playing on main, its log shown as the page shows it. */
function Controls(props: {
  readonly main: HTMLVideoElement;
  readonly letGoOfHls: () => void;
}): JSX.Element {
  const [lines, log] = useLog();
  return (
    <AirPlayControls
      mediaToken="media-1"
      video={() => props.main}
      letGoOfHls={props.letGoOfHls}
      log={log}
      lines={lines}
    />
  );
}

/** The controls mounted, served answers, what was asked kept in heard. */
async function mountControls(
  answers: Parameters<typeof servedBy>[0],
  heard: Heard[],
  given: { readonly letGoOfHls?: () => void; readonly pathMoves?: PathMoves } = {},
): Promise<{ readonly page: Mounted; readonly main: HTMLVideoElement }> {
  const { letGoOfHls = () => undefined, pathMoves } = given;
  const services = { ...servedBy(answers, heard), pathMoves };
  services.session.signIn("token");
  const main = document.createElement("video");
  main.disableRemotePlayback = true;
  main.currentTime = 42;
  mounted = await mount(withServices(<Controls main={main} letGoOfHls={letGoOfHls} />, services));
  return { page: mounted, main };
}

/** The page's log, a line each. */
function logOf(page: Mounted): string {
  return [...page.container.querySelectorAll(".lab__log li")]
    .map((line) => line.textContent)
    .join("\n");
}

describe("the AirPlay lab (airplay.md's spike)", () => {
  it("fetches the grant as the menu opens, so way B shows the picker on this player within the press", async () => {
    const heard: Heard[] = [];
    const letGoOfHls = vi.fn();
    const { page, main } = await mountControls({ "POST /airplay/grants": [200, made] }, heard, {
      letGoOfHls,
    });
    expect(buttonNamed(page.container, "Way B: this player")).toBeUndefined();
    await click(buttonNamed(page.container, "AirPlay menu"));
    await settleUntil(() => page.container.textContent.includes("Grant: "));
    const url = new URL(masterPath, globalThis.location.origin).href;
    expect(page.container.textContent).toContain(`Grant: ${url}`);
    expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /airplay/grants"]);
    expect(bodyField(heard[0]!, "mediaToken")).toBe("media-1");
    await click(buttonNamed(page.container, "Way B: this player"));
    expect(letGoOfHls).toHaveBeenCalledOnce();
    expect(pickedOn).toEqual([main]);
    expect(main.disableRemotePlayback).toBe(false);
    expect(main.src).toBe(url);
    expect(logOf(page)).toContain("menu opened: fetching the grant before the press");
    expect(logOf(page)).toContain(`grant fetched: ${url}`);
    expect(logOf(page)).toContain(
      "way B: the page's element given the grant at 42.0 s, hls.js let go of",
    );
    expect(logOf(page)).toContain("picker shown");
    expect(heard).toHaveLength(1);
  });

  it("way A gives a second player the grant and shows its picker, and logs what it says of its target", async () => {
    const { page, main } = await mountControls({ "POST /airplay/grants": [200, made] }, []);
    await click(buttonNamed(page.container, "AirPlay menu"));
    await settleUntil(() => page.container.textContent.includes("Grant: "));
    expect(page.container.querySelector("video")?.hasAttribute("src")).toBe(false);
    await click(buttonNamed(page.container, "Way A: a second player"));
    const second = page.container.querySelector("video");
    expect(second).not.toBeNull();
    expect(second).not.toBe(main);
    expect(second?.getAttribute("x-webkit-airplay")).toBe("allow");
    expect(second?.src).toBe(new URL(masterPath, globalThis.location.origin).href);
    expect(pickedOn).toEqual([second]);
    expect(main.paused).toBe(true);
    await fire(second, new Event("webkitcurrentplaybacktargetiswirelesschanged"));
    await settleUntil(() => logOf(page).includes("webkitcurrentplaybacktargetiswirelesschanged"));
    expect(logOf(page)).toContain(
      "the second element: webkitcurrentplaybacktargetiswirelesschanged",
    );
    expect(logOf(page)).toContain("way A: a second element given the grant at 42.0 s");
  });

  it("Stop ends playing, revokes the grant and shows the revocation was answered", async () => {
    const heard: Heard[] = [];
    const { page, main } = await mountControls(
      {
        "POST /airplay/grants": [200, made],
        "DELETE /airplay/grants": [200, { revoked: 1 }],
      },
      heard,
    );
    await click(buttonNamed(page.container, "AirPlay menu"));
    await settleUntil(() => page.container.textContent.includes("Grant: "));
    await click(buttonNamed(page.container, "Way A: a second player"));
    await click(buttonNamed(page.container, "Stop"));
    await settleUntil(() => page.container.textContent.includes("Stop answered"));
    expect(page.container.textContent).toContain("Stop answered: 1 grant(s) revoked");
    expect(page.container.textContent).toContain("No grant fetched");
    expect(page.container.querySelector("video")?.hasAttribute("src")).toBe(false);
    expect(main.hasAttribute("src")).toBe(false);
    expect(heard.map((each) => each.methodAndPath)).toEqual([
      "POST /airplay/grants",
      "DELETE /airplay/grants",
    ]);
    expect(logOf(page)).toContain("revocation answered: 1 revoked");
  });

  it("under a discovery's loader, puts the grant on the core's name the page reaches, not the client origin it is at (the owner's staging test, 2026-11-13)", async () => {
    const lanName = "https://192-168-1-2.k7xq2.orpheum-core.com:8443";
    const { page, main } = await mountControls({ "POST /airplay/grants": [200, made] }, [], {
      pathMoves: { ...createPathMoves(), directOrigin: () => lanName },
    });
    await click(buttonNamed(page.container, "AirPlay menu"));
    await settleUntil(() => page.container.textContent.includes("Grant: "));
    const url = `${lanName}${masterPath}`;
    expect(page.container.textContent).toContain(`Grant: ${url}`);
    await click(buttonNamed(page.container, "Way B: this player"));
    expect(main.src).toBe(url);
    expect(pickedOn).toEqual([main]);
  });

  it("over WebRTC alone, says AirPlay cannot work, asks for no grant and shows no picker (airplay.md item 2)", async () => {
    const heard: Heard[] = [];
    const { page } = await mountControls({ "POST /airplay/grants": [200, made] }, heard, {
      pathMoves: {
        ...createPathMoves(),
        isOverWebrtc: () => true,
        directOrigin: () => undefined,
      },
    });
    await click(buttonNamed(page.container, "AirPlay menu"));
    await settleUntil(() => logOf(page).includes("WebRTC alone"));
    expect(logOf(page)).toContain(
      "the core is reached over WebRTC alone: a receiver cannot reach it, so AirPlay cannot work here",
    );
    await click(buttonNamed(page.container, "Way B: this player"));
    expect(pickedOn).toEqual([]);
    expect(heard).toEqual([]);
    expect(page.container.textContent).toContain("No grant fetched");
  });

  it("logs a grant refused, as while the lab is off, and way B then shows no picker", async () => {
    const { page } = await mountControls({ "POST /airplay/grants": refusal(404, "notFound") }, []);
    await click(buttonNamed(page.container, "AirPlay menu"));
    await settleUntil(() => logOf(page).includes("the grant was refused"));
    await click(buttonNamed(page.container, "Way B: this player"));
    expect(pickedOn).toEqual([]);
    expect(logOf(page)).toContain(
      "way B: the grant is not fetched yet; open the menu and wait for it",
    );
    expect(page.container.textContent).toContain("No grant fetched");
  });
});
