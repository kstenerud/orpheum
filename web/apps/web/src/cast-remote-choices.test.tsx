// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { CastRemote } from "./cast-remote.tsx";
import { DebugToolsContext } from "./debug-tools.tsx";
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
import { withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";

/** The cast this device sends, as GET /me/casts lists it. */
const cast = {
  castId: 9,
  isSender: true,
  fileId: 4,
  itemId: 2,
  itemName: "Metropolis",
  durationMs: 600_000,
  senderDeviceId: 1,
  senderName: "Pixel",
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  membershipCredential: "remote",
};

/** The remote's welcome, the receiver's film paused at a minute. */
const welcome = {
  type: "welcome",
  seq: 3,
  sessionId: 9,
  memberId: 2,
  deviceId: 5,
  isHost: true,
  rights: { canPlayPause: true, canSeek: true },
  group: { state: "paused", positionMs: 61_000, rate: 1, fileId: 4 },
  roster: [],
};

/** The cast this device sends, as the live connection says it, its receiver having said Use. */
const sending = {
  castId: 9,
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  fileId: 4,
  itemName: "Metropolis",
  durationMs: 600_000,
  isAccepted: true,
};

/** The cast's roster, its receiver leading and saying what it shows. */
function shows(picture: Readonly<Record<string, unknown>>): Record<string, unknown> {
  return {
    type: "roster",
    roster: [
      {
        id: 2,
        name: "owner",
        isHost: true,
        devices: [
          { id: 5, role: "panel" },
          { id: 7, role: "video", isLeader: true, playback: { delivery: "converted", picture } },
        ],
      },
    ],
  };
}

describe("the cast's remote choosing what the receiver shows, and its ⏮ (D440 §5)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.sessionStorage.clear();
  });

  /** The remote on film 4's Play page, its person's debug tools shown or not, its session joined. */
  async function casting(
    answers: Parameters<typeof servedBy>[0],
    heard: Heard[] = [],
    showsDebugTools = false,
  ): Promise<ReturnType<typeof liveAndSession>> {
    const services = servedBy(
      { "GET /me/casts": [200, { casts: [cast] }], "GET /files/4/up-next": [200, {}], ...answers },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <DebugToolsContext value={{ isShown: showsDebugTools, onChanged: () => undefined }}>
          <LiveProvider>
            <CastRemote castId={9} fileId={4} onPlayHere={vi.fn()} />
          </LiveProvider>
        </DebugToolsContext>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "sending", sending });
    await sockets.session.accept();
    await sockets.session.say(welcome);
    return sockets;
  }

  it("shows the receiver's subtitles in the overlay's Subtitles menu as it says them, and sends the person's choice of them to the receiver (D444)", async () => {
    const { session } = await casting({});
    const page = mounted!.container;
    const opener = (): Element | undefined =>
      [...page.querySelectorAll("button[aria-haspopup]")].find(
        (button) => button.getAttribute("aria-label")?.startsWith("Subtitles") === true,
      );
    const choice = (text: string): Element | undefined =>
      [...page.querySelectorAll("dialog .list-box button")].find(
        (button) => button.textContent === text,
      );
    await session.say(shows({ subtitles: [] }));
    expect(opener()).toBeUndefined();
    const subtitles = [
      { key: "s5", label: "English" },
      { key: "s7", label: "French" },
    ];
    await session.say(shows({ subtitles }));
    expect(opener()?.getAttribute("aria-label")).toBe("Subtitles: Off");
    await click(opener());
    expect([...page.querySelectorAll("dialog .list-box button")].map((b) => b.textContent)).toEqual(
      ["Off", "English", "French"],
    );

    await click(choice("French"));
    await session.say(shows({ subtitles, subtitleKey: "s7" })); // the receiver says it shows it
    expect(opener()?.getAttribute("aria-label")).toBe("Subtitles: French");
    await click(opener());
    await click(choice("Off"));
    expect(session.heard.filter((each) => each["type"] === "picture")).toEqual([
      { v: 3, type: "picture", picture: { kind: "subtitles", key: "s7" } },
      { v: 3, type: "picture", picture: { kind: "subtitles" } },
    ]);
  });

  it("offers the receiver's original, as the player does, to a person who shows debug tools (D418 §4)", async () => {
    const hidden = await casting({});
    await hidden.session.say(shows({ canPlayOriginal: true }));
    expect(buttonWithText(mounted!.container, "Play the original file")).toBeUndefined();
    mounted?.unmount();

    const { session } = await casting({}, [], true);
    await session.say(shows({ canPlayOriginal: false }));
    expect(buttonWithText(mounted!.container, "Play the original file")).toBeUndefined();
    await session.say(shows({ canPlayOriginal: true }));
    // In ⁝, as the player's (D444).
    const settings = [...mounted!.container.querySelectorAll("button[aria-haspopup]")].find(
      (button) => button.getAttribute("aria-label") === "Settings",
    );
    await click(settings);
    await click(buttonWithText(mounted!.container, "Play the original file"));
    expect(session.heard.filter((each) => each["type"] === "picture")).toEqual([
      { v: 3, type: "picture", picture: { kind: "original" } },
    ]);
  });

  it("plays the episode before on the receiver at ⏮", async () => {
    const heard: Heard[] = [];
    const pilot = {
      itemId: 3,
      name: "Pilot",
      groupNumber: 1,
      numberInGroup: 1,
      fileId: 3,
      isPartway: false,
    };
    await casting(
      {
        "GET /files/4/up-next": [200, { previous: pilot }],
        "POST /me/casts/9/play": [200, { ...cast, fileId: 3, itemName: "Pilot" }],
      },
      heard,
    );
    await click(buttonNamed(mounted!.container, "Previous episode"));
    expect(heard).toContainEqual(
      expect.objectContaining({ methodAndPath: "POST /me/casts/9/play", body: { fileId: 3 } }),
    );
  });

  it("offers no ⏮ where nothing plays before the film, as the player offers none", async () => {
    await casting({});
    expect(buttonNamed(mounted!.container, "Previous episode")).toBeUndefined();
  });
});
