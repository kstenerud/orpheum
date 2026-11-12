// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { CastToReceive } from "@orpheum/sdk";
import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { CastStartPress, pressOf, type CastAnswer } from "./cast-start.tsx";
import { click, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";

/** A cast handed to this device: Metropolis, from the Pixel. */
const cast: CastToReceive = {
  castId: 5,
  membershipCredential: "receiver",
  fileId: 7,
  itemName: "Metropolis",
  positionMs: 61_000,
  fromDeviceId: 2,
  fromDeviceName: "Pixel",
};

/** The page's own full screen, as a test's browser answers asking for it: answer, and how often. */
function pageFullScreen(answer: "grant" | "refuse"): { readonly asked: () => number } {
  let asked = 0;
  document.documentElement.requestFullscreen = (): Promise<void> => {
    asked += 1;
    return answer === "grant" ? Promise.resolve() : Promise.reject(new TypeError("no press"));
  };
  return { asked: () => asked };
}

describe("a cast taken on a page never pressed, held for the press that starts it (D468 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    Reflect.deleteProperty(document.documentElement, "requestFullscreen");
  });

  /** CastStartPress for cast, what its press told, and how often it was told the cast is gone. */
  async function held(): Promise<{
    readonly pressed: CastAnswer[];
    readonly gone: () => number;
    readonly server: FakeTogetherServer;
  }> {
    const pressed: CastAnswer[] = [];
    let gone = 0;
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <CastStartPress
          cast={cast}
          onPressed={(answer) => {
            pressed.push(answer);
          }}
          onGone={() => {
            gone += 1;
          }}
        />,
        { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket },
      ),
    );
    return { pressed, gone: () => gone, server };
  }

  /** The one button the held cast shows. */
  function startButton(): HTMLButtonElement {
    const buttons = [...document.body.querySelectorAll("button")];
    expect(buttons).toHaveLength(1);
    const [button] = buttons;
    if (button === undefined) {
      throw new Error("no button");
    }
    return button;
  }

  it("shows one button asking for a press to start, focused for a remote's OK, and nothing to decline", async () => {
    await held();
    const button = startButton();
    expect(button.textContent).toBe("Click to start"); // a pointer's words; a television's below
    expect(document.activeElement).toBe(button);
  });

  it("says to press OK on a television", async () => {
    const userAgent = Object.getOwnPropertyDescriptor(Navigator.prototype, "userAgent");
    Object.defineProperty(Navigator.prototype, "userAgent", {
      configurable: true,
      get: () => "Mozilla/5.0 (Linux; Android 9; AFTKA) AppleWebKit/537.36 Silk/131.4.1",
    });
    try {
      await held();
      expect(startButton().textContent).toBe("Press OK to start");
    } finally {
      if (userAgent !== undefined) {
        Object.defineProperty(Navigator.prototype, "userAgent", userAgent);
      }
    }
  });

  it("keeps the focus, so an arrow moving it away leaves OK still answering it", async () => {
    await held();
    const button = startButton();
    const elsewhere = document.createElement("input");
    document.body.append(elsewhere);
    try {
      act(() => {
        elsewhere.focus();
      });
      expect(document.activeElement).toBe(button);
    } finally {
      elsewhere.remove();
    }
  });

  it("asks for the page full screen in its own press, and says by what press it started (D358 §2)", async () => {
    const fullScreen = pageFullScreen("grant");
    const { pressed } = await held();
    await click(startButton()); // a remote's OK: a click with no count
    expect(fullScreen.asked()).toBe(1);
    expect(pressed).toHaveLength(1);
    expect(pressed[0]).toMatchObject({ castId: 5, by: "key" });
    await expect(pressed[0]?.fullScreen).resolves.toBe("entered");
  });

  it("says a refused full screen as refused", async () => {
    pageFullScreen("refuse");
    const { pressed } = await held();
    await click(startButton());
    await expect(pressed[0]?.fullScreen).resolves.toBe("refused");
  });

  it("closes when the cast ends before its press, its sender gone", async () => {
    const { pressed, gone, server } = await held();
    await server.accept();
    expect(server.heard[0]).toMatchObject({ type: "hello", credential: "receiver" });
    await server.say({ type: "ended", reason: "stopped" });
    expect(gone()).toBe(1);
    expect(pressed).toEqual([]);
  });
});

describe("how a press was made", () => {
  it("tells a key's click, a tap and a mouse's click apart", () => {
    expect(pressOf(new MouseEvent("click", { detail: 0 }))).toBe("key");
    expect(pressOf(new PointerEvent("click", { detail: 1, pointerType: "touch" }))).toBe("tap");
    expect(pressOf(new PointerEvent("click", { detail: 1, pointerType: "mouse" }))).toBe("click");
    expect(pressOf(new MouseEvent("click", { detail: 1 }))).toBe("click");
  });
});
