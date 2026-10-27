// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { CastReady } from "./cast-ready.tsx";
import { LiveProvider } from "./live.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
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

  /** The ready page of a receiver that said Use to a cast with nothing playing. */
  async function readyPage(heard: Heard[]): Promise<{
    readonly services: ReturnType<typeof servedBy>;
    readonly sockets: ReturnType<typeof liveAndSession>;
  }> {
    const services = servedBy(
      {
        "POST /me/casts/5/accept": [200, { castId: 5 }],
        "POST /me/casts/5/end": [200, { positionMs: 0, isPlaying: false }],
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
    await sockets.live.say({ type: "receive", receive: ready });
    await click(buttonWithText(document.body, "Use"));
    await sockets.session.accept();
    return { services, sockets };
  }

  it("says it is ready, controlled from the sender, holding no player", async () => {
    const { sockets } = await readyPage([]);
    expect(mounted?.container.textContent).toContain("Ready, controlled from Pixel");
    expect(mounted?.container.querySelector("video")).toBeNull();
    expect(sockets.session.heard[0]).toMatchObject({ credential: "cast", role: "panel" });
  });

  it("takes control back at Back, ending the session and going to its home page (D364 §9)", async () => {
    const heard: Heard[] = [];
    const { services } = await readyPage(heard);
    press("Escape");
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/5/end")?.body).toEqual({
      reason: "takenBack",
    });
    expect(services.navigation.route()).toEqual({ screen: "home" });
  });

  it("goes back to the page it was on before the session as the sender stops casting (D364 §10)", async () => {
    const { services, sockets } = await readyPage([]);
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(services.navigation.route()).toEqual({ screen: "films" });
  });

  it("says it is not being cast to without a cast waiting ready", async () => {
    mounted = await mount(withServices(<CastReady />, servedBy({})));
    expect(mounted.container.textContent).toContain("This device is not being cast to.");
  });
});
