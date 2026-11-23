// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { sessionAirPlayGrants } from "./airplay.ts";
import { offerAirPlayHere } from "./airplay-here.ts";
import { CastDialog } from "./cast-dialog.tsx";
import { LiveProvider, useLive } from "./live.tsx";
import { CastingLeft } from "./old-addresses.tsx";
import {
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

/** The cast this device sends, as the live connection says it, its receiver having said Use. */
const sending = {
  castId: 9,
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  fileId: 4,
  itemName: "Metropolis",
  durationMs: 0,
  isAccepted: true,
};

describe("Cast's dialog (D384 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** The dialog over film 4's Play page, its live connection open; what it was told of closing. */
  async function opened(answers: Answers = {}): Promise<{
    readonly live: ReturnType<typeof fakeTogetherServer>;
    readonly onClose: ReturnType<typeof vi.fn>;
  }> {
    const live = fakeTogetherServer();
    const services = servedBy(
      { "GET /me/devices": [200, { devices: [] }], ...answers },
      [],
      "token",
    );
    services.navigation.go({ screen: "play", fileId: 4 });
    const onClose = vi.fn();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastDialog onClose={onClose} />
        </LiveProvider>,
        { ...services, openTogetherSocket: live.openSocket },
      ),
    );
    await live.accept();
    return { live, onClose };
  }

  it("closes once a cast started from it is taken, the page under it its remote", async () => {
    const { live, onClose } = await opened();
    await live.say({ type: "sending" });
    await live.say({ type: "sending", sending: { ...sending, isAccepted: false } });
    expect(onClose).not.toHaveBeenCalled();
    await live.say({ type: "sending", sending });
    expect(onClose).toHaveBeenCalledOnce();
  });

  // One file's bytes filed as a film and as an episode play as the title they were opened from
  // (B0 question 20): the cast the dialog starts plays the Play page's file as its route's title.
  it("starts a cast of the Play page's film as the title it was opened from", async () => {
    const heard: Heard[] = [];
    const live = fakeTogetherServer();
    const device = (id: number, name: string): Record<string, unknown> => ({
      id,
      name,
      platform: "web",
      addedAt: 0,
      lastUsedAt: 0,
      isSignedIn: true,
      isByCode: false,
      isThisDevice: id === 1,
    });
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [device(1, "Laptop"), device(2, "Fire TV")] }],
        "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
      },
      heard,
      "token",
    );
    services.navigation.go({ screen: "play", fileId: 4, itemId: 40 });
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastDialog onClose={vi.fn()} />
        </LiveProvider>,
        { ...services, openTogetherSocket: live.openSocket },
      ),
    );
    await live.accept();
    await live.say({ type: "sending" });
    await live.say({ type: "devices", deviceIds: [1, 2] });
    await click(buttonWithText(mounted.container, "Fire TV"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts")?.body).toMatchObject({
      toDeviceId: 2,
      fileId: 4,
      itemId: 40,
    });
  });

  it("closes as the film goes to AirPlay from it (D473)", async () => {
    const started: string[] = [];
    const unoffer = offerAirPlayHere({
      mediaToken: "media",
      grants: sessionAirPlayGrants,
      isPlaying: () => false,
      start: (url) => {
        started.push(url);
      },
      showControls: () => undefined,
    });
    try {
      const live = fakeTogetherServer();
      const grant = { masterPath: "/g/master.m3u8", expiresAt: "2026-11-14T12:00:00Z" };
      const services = servedBy(
        { "GET /me/devices": [200, { devices: [] }], "POST /airplay/grants": [201, grant] },
        [],
        "token",
      );
      const onClose = vi.fn();
      mounted = await mount(
        withServices(
          <LiveProvider>
            <CastDialog onClose={onClose} />
          </LiveProvider>,
          { ...services, openTogetherSocket: live.openSocket },
        ),
      );
      await live.accept();
      await live.say({ type: "sending" });
      await click(buttonWithText(mounted.container, "AirPlay"));
      expect(started).toHaveLength(1);
      expect(onClose).toHaveBeenCalledOnce();
    } finally {
      unoffer();
    }
  });

  it("offers no AirPlay while a cast to a device is on, one cast at a time (D523 §7)", async () => {
    const unoffer = offerAirPlayHere({
      mediaToken: "media",
      grants: sessionAirPlayGrants,
      isPlaying: () => false,
      start: () => undefined,
      showControls: () => undefined,
    });
    try {
      const grant = { masterPath: "/g/master.m3u8", expiresAt: "2026-11-14T12:00:00Z" };
      const { live } = await opened({ "POST /airplay/grants": [201, grant] });
      await live.say({ type: "sending" });
      expect(document.querySelector("dialog")?.textContent).toContain("AirPlay");
      await live.say({ type: "sending", sending });
      expect(document.querySelector("dialog")?.textContent).toContain("Fire TV");
      expect(document.querySelector("dialog")?.textContent).not.toContain("AirPlay");
    } finally {
      unoffer();
    }
  });

  it("stays open over a cast on as it opened, saying what plays where", async () => {
    const live = fakeTogetherServer();
    const onClose = vi.fn();
    /** The dialog, opened once the live connection says a cast is on. */
    function OpenedWhileCasting(): JSX.Element | null {
      return useLive().sending === undefined ? null : <CastDialog onClose={onClose} />;
    }
    mounted = await mount(
      withServices(
        <LiveProvider>
          <OpenedWhileCasting />
        </LiveProvider>,
        { ...servedBy({}, [], "token"), openTogetherSocket: live.openSocket },
      ),
    );
    await live.accept();
    await live.say({ type: "sending", sending });
    await live.say({ type: "sending", sending: { ...sending, fileId: 5 } });
    expect(onClose).not.toHaveBeenCalled();
    expect(document.querySelector("dialog")?.textContent).toContain(
      "Playing Metropolis on Fire TV",
    );
  });

  /** The dialog opened over a cast on, the end request answered as given; what it was told of closing. */
  async function openedWhileCasting(
    end: Answers[string],
    heard: Heard[],
  ): Promise<ReturnType<typeof vi.fn>> {
    const live = fakeTogetherServer();
    const onClose = vi.fn();
    /** The dialog, opened once the live connection says a cast is on. */
    function OpenedWhileCasting(): JSX.Element | null {
      return useLive().sending === undefined ? null : <CastDialog onClose={onClose} />;
    }
    mounted = await mount(
      withServices(
        <LiveProvider>
          <OpenedWhileCasting />
        </LiveProvider>,
        {
          ...servedBy({ "POST /me/casts/9/end": end }, heard, "token"),
          openTogetherSocket: live.openSocket,
        },
      ),
    );
    await live.accept();
    await live.say({ type: "sending", sending });
    return onClose;
  }

  // dialogs-spec: while casting, its foot is Cancel then Stop casting, its main action, where focus
  // arrives; Cancel closes it, the cast going on.
  it("draws Cancel then Stop casting at its foot while casting, focus on Stop, and Cancel stops nothing", async () => {
    const heard: Heard[] = [];
    const onClose = await openedWhileCasting([200, { positionMs: 0, isPlaying: false }], heard);
    const foot = [...document.querySelectorAll("dialog .modal-dialog__foot button")];
    expect(foot.map((each) => each.textContent)).toEqual(["Cancel", "Stop casting"]);
    expect(document.activeElement).toBe(foot[1]);
    await click(buttonWithText(document.body, "Cancel"));
    expect(onClose).toHaveBeenCalledOnce();
    expect(heard.map((each) => each.methodAndPath)).not.toContain("POST /me/casts/9/end");
  });

  // Principles §28: its Stop hid a failure, the cast going on while the person took it for stopped.
  it("stops casting, tried once more where the first try fails, and closes", async () => {
    const heard: Heard[] = [];
    const onClose = await openedWhileCasting(
      [refusal(503, "unavailable"), [200, { positionMs: 0, isPlaying: false }]],
      heard,
    );
    await click(buttonWithText(document.body, "Stop casting"));
    expect(heard.filter((each) => each.methodAndPath === "POST /me/casts/9/end")).toHaveLength(2);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("says why Stop casting failed where both tries do, and stays open to try again", async () => {
    const onClose = await openedWhileCasting(
      [refusal(503, "unavailable"), refusal(503, "unavailable")],
      [],
    );
    await click(buttonWithText(document.body, "Stop casting"));
    expect(onClose).not.toHaveBeenCalled();
    expect(document.querySelector("dialog [role=alert]")).not.toBeNull();
  });
});

describe("an old address of the casting page (D384 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** /casting, as a history entry holds it, once the live connection says what this device sends. */
  async function leftAt(
    said: Readonly<Record<string, unknown>>,
  ): Promise<ReturnType<typeof servedBy>> {
    const live = fakeTogetherServer();
    const services = servedBy({}, [], "token");
    services.navigation.go({ screen: "casting" });
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastingLeft />
        </LiveProvider>,
        { ...services, openTogetherSocket: live.openSocket },
      ),
    );
    await live.accept();
    expect(services.navigation.route()).toEqual({ screen: "casting" });
    await live.say(said);
    return services;
  }

  it("goes to the Play page of the film the cast plays, its remote, in its place", async () => {
    const services = await leftAt({ type: "sending", sending });
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 4 });
    expect(services.navigation.behind()).toEqual([{ screen: "films" }]);
  });

  it("goes home where this device casts no film", async () => {
    const services = await leftAt({ type: "sending" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
  });
});
