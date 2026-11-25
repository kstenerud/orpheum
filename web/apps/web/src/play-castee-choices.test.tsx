// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { CasteePlay } from "./play-castee.tsx";
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
import { IsAdminContext } from "./warning-sign.tsx";

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

/** Item 2, the cast's film, as GET /items/2 has it: the remote's way up leads to its page. */
const metropolis = {
  id: 2,
  kind: "film",
  name: "Metropolis",
  identificationState: "identified",
  art: {},
  versions: [],
  extras: [],
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
  return says({ picture });
}

/** The cast's roster, its receiver leading and saying how it sounds. */
function sounds(sound: Readonly<Record<string, unknown>>): Record<string, unknown> {
  return says({ sound: { isLevellingOn: false, audioTracks: [], ...sound } });
}

/** The cast's roster, its receiver leading and saying how it plays, converted. */
function says(playback: Readonly<Record<string, unknown>>): Record<string, unknown> {
  return {
    type: "roster",
    roster: [
      {
        id: 2,
        name: "owner",
        isHost: true,
        devices: [
          { id: 5, role: "panel" },
          {
            id: 7,
            role: "video",
            isLeader: true,
            playback: { delivery: "converted", ...playback },
          },
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

  /**
   * The remote on film 4's Play page, its person's debug tools shown or not, an admin or not, its
   * session joined.
   */
  async function casting(
    answers: Parameters<typeof servedBy>[0],
    heard: Heard[] = [],
    person: { readonly showsDebugTools?: boolean; readonly isAdmin?: boolean } = {},
  ): Promise<ReturnType<typeof liveAndSession>> {
    const { showsDebugTools = false, isAdmin = false } = person;
    const services = servedBy(
      {
        "GET /me/casts": [200, { casts: [cast] }],
        "GET /files/4/up-next": [200, {}],
        "GET /items/2": [200, metropolis],
        "GET /files/4/segments": [200, { segments: [] }],
        ...answers,
      },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <IsAdminContext value={isAdmin}>
          <DebugToolsContext value={{ isShown: showsDebugTools, onChanged: () => undefined }}>
            <LiveProvider>
              <CasteePlay castId={9} fileId={4} onPlayHere={vi.fn()} />
            </LiveProvider>
          </DebugToolsContext>
        </IsAdminContext>,
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
      [...page.querySelectorAll("[aria-haspopup]")].find(
        (button) => button.getAttribute("aria-label")?.startsWith("Subtitles") === true,
      );
    const choice = (text: string): Element | undefined =>
      [...page.querySelectorAll('dialog .list-box [role="button"]')].find(
        (button) => button.textContent === text,
      );
    await session.say(shows({ subtitles: [] }));
    // No track, yet Find subtitles, as the player's 💬 offers it with none (the plan's "The cast
    // remote's harder parts", item 3).
    expect(opener()?.getAttribute("aria-label")).toBe("Subtitles");
    const subtitles = [
      { key: "s5", label: "English" },
      { key: "s7", label: "French" },
    ];
    await session.say(shows({ subtitles }));
    // Named Subtitles, its choice named in its menu, as the player's (the owner, 2026-11-23).
    expect(opener()?.getAttribute("aria-label")).toBe("Subtitles");
    const shown = (): string[] =>
      [...page.querySelectorAll('dialog .list-box [role="button"]')]
        .filter((b) => b.closest("[hidden]") === null)
        .map((b) => b.textContent);
    await click(opener());
    expect(shown()).toEqual(["Subtitles: Off›", "Find subtitles…"]);
    await click(choice("Subtitles: Off›"));
    expect(shown()).toEqual(["‹Subtitles", "Off", "English", "French"]);

    await click(choice("French"));
    await session.say(shows({ subtitles, subtitleKey: "s7" })); // the receiver says it shows it
    await click(opener());
    expect(shown()).toEqual(["Subtitles: French›", "Find subtitles…"]);
    await click(choice("Subtitles: French›"));
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

    const { session } = await casting({}, [], { showsDebugTools: true });
    await session.say(shows({ canPlayOriginal: false }));
    expect(buttonWithText(mounted!.container, "Play the original file")).toBeUndefined();
    await session.say(shows({ canPlayOriginal: true }));
    // In ⁝, as the player's (D444).
    const settings = [...mounted!.container.querySelectorAll("[aria-haspopup]")].find(
      (button) => button.getAttribute("aria-label") === "Settings",
    );
    await click(settings);
    await click(buttonWithText(mounted!.container, "Play the original file"));
    expect(session.heard.filter((each) => each["type"] === "picture")).toEqual([
      { v: 3, type: "picture", picture: { kind: "original" } },
    ]);
  });

  /** The overlay's menu button named name, its menu opened by a click. */
  function opener(name: string): Element | undefined {
    return [...mounted!.container.querySelectorAll("[aria-haspopup]")].find(
      (button) => button.getAttribute("aria-label") === name,
    );
  }

  // The plan's "The cast remote's harder parts", item 2: the player's 💬 stepper, on the remote.
  it("moves the receiver's subtitle timing from where it says it stands, as the player's Subtitles menu does", async () => {
    const { session } = await casting({});
    const subtitles = [{ key: "s5", label: "English" }];
    await session.say(shows({ subtitles, subtitleKey: "s5", subtitleOffsetMs: 1500 }));
    await click(opener("Subtitles"));
    expect(mounted!.container.textContent).toContain("Subtitles show 1.5 s later");
    await click(buttonNamed(mounted!.container, "Subtitles later"));
    await session.say(shows({ subtitles, subtitleKey: "s5", subtitleOffsetMs: 1600 })); // it took the step
    await session.say(shows({ subtitles, subtitleKey: "s5" })); // then says it is in step
    await click(buttonNamed(mounted!.container, "Subtitles earlier"));
    expect(session.heard.filter((each) => each["type"] === "picture")).toEqual([
      { v: 3, type: "picture", picture: { kind: "subtitleOffset", ms: 1600 } },
      { v: 3, type: "picture", picture: { kind: "subtitleOffset", ms: -100 } },
    ]);
  });

  it("steps the receiver's subtitle timing from the value last pressed for two quick presses, the receiver's earlier word not snapping it back", async () => {
    const { session } = await casting({});
    const subtitles = [{ key: "s5", label: "English" }];
    await session.say(shows({ subtitles, subtitleKey: "s5", subtitleOffsetMs: 1500 }));
    await click(opener("Subtitles"));
    await click(buttonNamed(mounted!.container, "Subtitles later"));
    await click(buttonNamed(mounted!.container, "Subtitles later"));
    await session.say(shows({ subtitles, subtitleKey: "s5", subtitleOffsetMs: 1600 })); // the first step, late
    expect(mounted!.container.textContent).toContain("Subtitles show 1.7 s later");
    expect(session.heard.filter((each) => each["type"] === "picture")).toEqual([
      { v: 3, type: "picture", picture: { kind: "subtitleOffset", ms: 1600 } },
      { v: 3, type: "picture", picture: { kind: "subtitleOffset", ms: 1700 } },
    ]);
  });

  // The plan's "The cast remote's harder parts", item 3: the player's Find subtitles, on the remote.
  it("finds a subtitle for the receiver's file, even where it has none, and tells the receiver one was fetched", async () => {
    const heard: Heard[] = [];
    const { session } = await casting(
      {
        "GET /files/4/subtitle-candidates": [
          200,
          {
            allowance: { remainingDownloads: 19, allowedDownloads: 20, isVip: false },
            didAskByIds: true,
            didAskByOpensubtitlesHash: true,
            candidates: [
              {
                providerFileId: 7,
                releaseName: "Film.2002.1080p",
                language: "en",
                isOpensubtitlesHashMatch: true,
                isHearingImpaired: false,
                isMachineTranslated: false,
                downloadCount: 120,
              },
            ],
          },
        ],
        "POST /files/4/subtitles": [200, { fetchedSubtitleId: 3, remainingDownloads: 18 }],
      },
      heard,
    );
    await session.say(shows({ subtitles: [] }));
    await click(opener("Subtitles"));
    await click(buttonWithText(mounted!.container, "Find subtitles…"));
    await click(buttonWithText(document.body, "Search OpenSubtitles"));
    await click(document.body.querySelector(".subtitle-search li button") ?? undefined);
    expect(heard.at(-1)).toEqual({
      methodAndPath: "POST /files/4/subtitles",
      body: { providerFileId: 7, language: "en" },
    });
    expect(document.body.querySelector(".subtitle-search")).toBeNull(); // the dialog closed
    expect(session.heard.filter((each) => each["type"] === "picture")).toEqual([
      { v: 3, type: "picture", picture: { kind: "subtitleFetched" } },
    ]);
  });

  // The plan's "The cast remote's harder parts", item 1: the player's "This file" (D537), on the
  // remote beside That device.
  it("moves the receiver's file's own timing, and makes it everyone's for an admin, as the player's Sound menu does", async () => {
    const { session } = await casting({}, [], { isAdmin: true });
    await session.say(sounds({ soundOffsetMs: 0, fileSoundOffset: { everyonesMs: 100 } }));
    await click(opener("Sound"));
    const page = mounted!.container;
    expect(page.textContent).toContain("That device");
    expect(page.textContent).toContain("This file");
    expect(buttonWithText(page, "Make it everyone's")).toBeUndefined(); // own is every person's
    await click(buttonNamed(page, "This file later"));
    await session.say(
      sounds({ soundOffsetMs: 0, fileSoundOffset: { everyonesMs: 100, ownMs: 150 } }),
    );
    await click(buttonWithText(page, "Make it everyone's"));
    expect(session.heard.filter((each) => each["type"] === "sound")).toEqual([
      { v: 3, type: "sound", sound: { kind: "fileSoundOffset", ms: 150 } },
      { v: 3, type: "sound", sound: { kind: "fileSoundOffset", ms: 150, isEveryones: true } },
    ]);
  });

  it("tells the admin of the receiver's file's own timing for anyone else, as the player's Sound menu does", async () => {
    const { session } = await casting({});
    await session.say(sounds({ fileSoundOffset: { everyonesMs: 0, ownMs: -200 } }));
    await click(opener("Sound"));
    expect(buttonWithText(mounted!.container, "Make it everyone's")).toBeUndefined();
    await click(buttonWithText(mounted!.container, "Tell the admin"));
    // The report, the file's offset suggested in it (problem-report.tsx).
    expect(document.body.textContent).toContain(
      "It suggests this file's sound play 200 ms earlier for everyone.",
    );
    expect(session.heard.filter((each) => each["type"] === "sound")).toEqual([]);
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
      expect.objectContaining({
        methodAndPath: "POST /me/casts/9/play",
        body: { fileId: 3, itemId: 3 },
      }),
    );
  });

  it("offers no ⏮ where nothing plays before the film, as the player offers none", async () => {
    await casting({});
    expect(buttonNamed(mounted!.container, "Previous episode")).toBeUndefined();
  });
});
