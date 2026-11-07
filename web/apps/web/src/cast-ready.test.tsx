// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastReady } from "./cast-ready.tsx";
import { LiveProvider } from "./live.tsx";
import {
  buttonNamed,
  buttonWithText,
  click,
  mount,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";

/** A cast handed to this device with nothing playing: ready, from the Pixel (D364 §5). */
const ready = {
  castId: 5,
  membershipCredential: "cast",
  fileId: 0,
  fromDeviceId: 2,
  fromDeviceName: "Pixel",
};

/** A key pressed on the page, as a remote's or a keyboard's. */
function press(key: string): void {
  act(() => {
    (document.activeElement ?? document.body).dispatchEvent(
      new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true }),
    );
  });
}

describe("a cast's receiver between videos (D364 §5, §9, §10)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** The ready page of a receiver that said Use to a cast with nothing playing, handed as given. */
  async function readyPage(
    heard: Heard[],
    handed: Readonly<Record<string, unknown>> = ready,
  ): Promise<{
    readonly services: ReturnType<typeof servedBy>;
    readonly sockets: ReturnType<typeof liveAndSession>;
  }> {
    const services = servedBy(
      {
        "POST /me/casts/5/accept": [200, { castId: 5 }],
        "POST /me/casts/5/end": [200, { positionMs: 0, isPlaying: false }],
        "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
      },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastReady />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "receive", receive: handed });
    await click(buttonWithText(document.body, "Use"));
    await sockets.session.accept();
    return { services, sockets };
  }

  it("follows its person's Watch Together session as just another follower, where the cast hands it their guest's part (D371 §4)", async () => {
    const heard: Heard[] = [];
    const { sockets } = await readyPage(heard, {
      ...ready,
      togetherCredential: "guest",
      isTogetherFollower: true,
    });
    await sockets.session.acceptEach();
    expect(sockets.session.heard).toContainEqual(
      expect.objectContaining({ type: "hello", credential: "guest", role: "video" }),
    );
    await sockets.session.say({
      type: "welcome",
      seq: 1,
      memberId: 4,
      deviceId: 9,
      isHost: false,
      group: { state: "Playing", positionMs: 61_000, rate: 1, fileId: 7 },
      roster: [],
    });
    expect(heard.map((each) => each.methodAndPath)).toContain("POST /together/playback");
    expect(mounted?.container.textContent).toContain("Metropolis");
    expect(mounted?.container.textContent).not.toContain("Ready, controlled from Pixel");
  });

  it("says it is ready, controlled from the sender, holding no player", async () => {
    const { sockets } = await readyPage([]);
    expect(mounted?.container.textContent).toContain("Ready, controlled from Pixel");
    expect(mounted?.container.querySelector("video")).toBeNull();
    expect(sockets.session.heard[0]).toMatchObject({ credential: "cast", role: "panel" });
  });

  // The owner, 2026-11-07 (the 11-20 batch, finding and answer 4): with nothing playing only Escape
  // left full screen, and it ended the cast; neither leaving full screen nor any press here ends it.
  it("leaves full screen by its ⤢ and goes back into it, ending nothing, nor at Escape or full screen left", async () => {
    const heard: Heard[] = [];
    const { services } = await readyPage(heard);
    const exitFullscreen = vi.fn(() => Promise.resolve());
    const requestFullscreen = vi.fn(() => Promise.resolve());
    let shown: Element | null = document.documentElement;
    Object.defineProperty(document, "fullscreenElement", { get: () => shown, configurable: true });
    Object.defineProperty(document, "exitFullscreen", {
      value: exitFullscreen,
      configurable: true,
    });
    document.documentElement.requestFullscreen = requestFullscreen;
    try {
      act(() => {
        document.dispatchEvent(new Event("fullscreenchange"));
      });
      await click(buttonNamed(document.body, "Leave full screen"));
      expect(exitFullscreen).toHaveBeenCalledOnce();
      shown = null;
      act(() => {
        document.dispatchEvent(new Event("fullscreenchange"));
      });
      press("Escape");
      await click(buttonNamed(document.body, "Full screen"));
      expect(requestFullscreen).toHaveBeenCalledOnce();
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
      expect(heard.filter((each) => each.methodAndPath === "POST /me/casts/5/end")).toEqual([]);
      expect(services.navigation.route()).not.toEqual({ screen: "home" });
      expect(mounted?.container.textContent).toContain("Ready, controlled from Pixel");
    } finally {
      Reflect.deleteProperty(document, "fullscreenElement");
      Reflect.deleteProperty(document, "exitFullscreen");
      Reflect.deleteProperty(document.documentElement, "requestFullscreen");
    }
  });

  it("goes to its home page as the sender stops casting (D384 §1)", async () => {
    const { services, sockets } = await readyPage([]);
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
  });

  it("says it is not being cast to without a cast waiting ready", async () => {
    mounted = await mount(withServices(<CastReady />, servedBy({})));
    expect(mounted.container.textContent).toContain("This device is not being cast to.");
  });
});
