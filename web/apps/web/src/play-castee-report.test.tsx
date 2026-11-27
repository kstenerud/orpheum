// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { CasteePlay } from "./play-castee.tsx";
import { DebugToolsContext } from "./debug-tools.tsx";
import { LiveProvider } from "./live.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  enter,
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

describe("Report a problem from the cast's remote (D524 §2)", () => {
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
      {
        "GET /me/casts": [200, { casts: [cast] }],
        "GET /files/4/up-next": [200, {}],
        "GET /files/4/trickplay": [404, {}],
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
        <DebugToolsContext value={{ isShown: showsDebugTools, onChanged: () => undefined }}>
          <LiveProvider>
            <CasteePlay castId={9} fileId={4} onPlayHere={vi.fn()} />
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

  /** Opens ⁝ and chooses Report a problem. */
  async function chooseReport(): Promise<void> {
    const settings = [...mounted!.container.querySelectorAll("[aria-haspopup]")].find(
      (button) => button.getAttribute("aria-label") === "Settings",
    );
    await click(settings);
    await click(buttonWithText(mounted!.container, "Report a problem"));
  }

  it("sends the receiver's playback, as the cast's session says it, in the player's place", async () => {
    const heard: Heard[] = [];
    const { session } = await casting({ "POST /problem-reports": [201, { id: 7 }] }, heard);
    await session.say({
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
              playback: { delivery: "converted", qualityKbps: 4000, linkKbps: 12_000 },
            },
          ],
        },
      ],
    });
    await chooseReport();
    const dialog = mounted!.container.querySelector<HTMLElement>(
      'dialog[aria-label="Report a problem"]',
    );
    await enter(dialog?.querySelector("textarea"), "no sound");
    await click(buttonWithText(dialog ?? mounted!.container, "Send"));
    const sent = heard.find((each) => each.methodAndPath === "POST /problem-reports");
    expect(sent).toBeDefined();
    expect(bodyField(sent!, "playback")).toEqual({
      fileId: 4,
      positionMs: 61_000,
      quality: "converted, 4000 kb/s over 12000 kb/s",
      state: "paused",
    });
  });

  it("draws no ⁝ before the receiver has said how it plays, which holds nothing else to choose", async () => {
    await casting({});
    const settings = [...mounted!.container.querySelectorAll("[aria-haspopup]")].find(
      (button) => button.getAttribute("aria-label") === "Settings",
    );
    expect(settings).toBeUndefined();
  });
});
