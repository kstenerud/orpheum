// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { tabStorage } from "./browser-storage.ts";
import { castAnswerGraceMs, castAnswerWithinMs, isNotedSending, noteSending } from "./cast.ts";
import { castReturnKey } from "./cast-return.ts";
import { CastToDevice, PlayOrSendOn } from "./cast-to.tsx";
import { LiveProvider, useLive } from "./live.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";

/**
 * A server for the page's live connection, the first socket it opens, and one for the cast's
 * session of one, every socket after: a sender waiting for its receiver's answer, a receiver asking
 * (D358).
 */
function liveAndSession(): {
  readonly live: FakeTogetherServer;
  readonly session: FakeTogetherServer;
  readonly openSocket: FakeTogetherServer["openSocket"];
} {
  const live = fakeTogetherServer();
  const session = fakeTogetherServer();
  let opened = 0;
  return {
    live,
    session,
    openSocket: (listeners) => {
      opened += 1;
      return opened === 1 ? live.openSocket(listeners) : session.openSocket(listeners);
    },
  };
}

/** A cast's session of one welcoming a device, nothing on yet. */
const castWelcome = {
  type: "welcome",
  seq: 1,
  sessionId: 9,
  memberId: 2,
  deviceId: 7,
  isHost: true,
  rights: { canPlayPause: true, canSeek: true },
  group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [],
};

/** Answers Play to the cast the receiver asks about (D358 §1, §2). */
async function pressPlay(container: HTMLElement): Promise<void> {
  const play = buttonWithText(container, "Play");
  expect(play).toBeDefined();
  await click(play);
}

/** A device of the person's, as GET /me/devices lists it. */
function device(id: number, name: string, isThisDevice = false): Record<string, unknown> {
  return {
    id,
    name,
    platform: "web",
    addedAt: 0,
    lastUsedAt: Date.UTC(2026, 9, 25, 20, 0),
    isSignedIn: true,
    isByCode: false,
    isThisDevice,
  };
}

/** What the page's live connection says the receiver is handed, as a child of the provider. */
function Receiving(): JSX.Element {
  const { receiving } = useLive();
  return <p>{receiving === undefined ? "" : `receiving cast ${String(receiving.castId)}`}</p>;
}

/** The file the page's live connection says the receiver plays, as a child of the provider. */
function ReceivingFile(): JSX.Element {
  const { receiving } = useLive();
  return <p>{receiving === undefined ? "" : `receiving file ${String(receiving.fileId)}`}</p>;
}

/** What the receiver shows, and its two ways out, as children of the provider. */
function Leaving(): JSX.Element {
  const { receiving, endReceiving, leaveReceiving } = useLive();
  return (
    <>
      <p>{receiving === undefined ? "" : `receiving cast ${String(receiving.castId)}`}</p>
      <button type="button" onClick={leaveReceiving}>
        leave
      </button>
      <button type="button" onClick={endReceiving}>
        take back
      </button>
    </>
  );
}

describe("Cast to another device (D337 §1, §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("lists the person's other devices open now, and casts to the one chosen from where the film stands", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/devices": [
          200,
          {
            devices: [device(1, "Laptop", true), device(2, "Fire TV"), device(3, "Phone, closed")],
          },
        ],
        "POST /me/casts": [
          201,
          { castId: 9, isSender: true, membershipCredential: "sender", receiverName: "Fire TV" },
        ],
      },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    const server = sockets.live;
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastToDevice fileId={4} positionMs={() => 61_000} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await server.accept();
    expect(server.heard[0]).toEqual({ type: "live", sessionToken: "token" });
    await server.say({ type: "devices", deviceIds: [1, 2] });
    await click(buttonWithText(mounted.container, "Cast to another device"));
    expect(mounted.container.textContent).toContain("Fire TV");
    expect(mounted.container.textContent).not.toContain("Phone, closed");
    expect(buttonWithText(mounted.container, "Laptop")).toBeUndefined();
    const before = services.navigation.route();
    await click(buttonWithText(mounted.container, "Fire TV"));
    const cast = heard.find((each) => each.methodAndPath === "POST /me/casts");
    expect(cast?.body).toEqual({ toDeviceId: 2, fileId: 4, positionMs: 61_000 });
    expect(bodyField(cast!, "toDeviceId")).toBe(2);
    // The receiver is asked first, and the film plays on here until it answers Play (D358 §3).
    expect(mounted.container.textContent).toContain("Asking Fire TV…");
    expect(services.navigation.route()).toEqual(before);
    expect(isNotedSending(tabStorage())).toBe(false);
    await sockets.session.accept();
    expect(sockets.session.heard[0]).toMatchObject({ credential: "sender", role: "panel" });
    await sockets.session.say(castWelcome);
    expect(services.navigation.route()).toEqual(before);
    // The receiver's Play puts the film on in the cast's session: the hand-off.
    await sockets.session.say({
      type: "group",
      seq: 2,
      group: { ...castWelcome.group, fileId: 4 },
    });
    expect(services.navigation.route()).toEqual({ screen: "casting" });
    // A film played here next goes on to the TV (§9 Q3).
    expect(isNotedSending(tabStorage())).toBe(true);
    noteSending(tabStorage(), false);
  });

  /** The Laptop casting film 4 to the Fire TV, and what the page then shows and says. */
  async function castToTheTv(heard: Heard[]): Promise<{
    readonly sockets: ReturnType<typeof liveAndSession>;
    readonly services: ReturnType<typeof servedBy>;
  }> {
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [device(1, "Laptop", true), device(2, "Fire TV")] }],
        "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
        "POST /me/casts/9/end": [200, { positionMs: 0, isPlaying: false }],
      },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastToDevice fileId={4} positionMs={() => 61_000} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "devices", deviceIds: [1, 2] });
    await click(buttonWithText(mounted.container, "Cast to another device"));
    await click(buttonWithText(mounted.container, "Fire TV"));
    await sockets.session.accept();
    await sockets.session.say(castWelcome);
    return { sockets, services };
  }

  it("says its receiver declined, and plays on here (D358 §3)", async () => {
    const { sockets, services } = await castToTheTv([]);
    const before = services.navigation.route();
    await sockets.session.say({ type: "ended", reason: "declined" });
    expect(mounted?.container.textContent).toContain("Fire TV declined.");
    expect(services.navigation.route()).toEqual(before);
    expect(isNotedSending(tabStorage())).toBe(false);
    expect(buttonWithText(mounted!.container, "Fire TV")).toBeDefined(); // to cast again
  });

  it("says its receiver did not answer, as it says or after a minute and a little more (D358 §4)", async () => {
    const said = await castToTheTv([]);
    await said.sockets.session.say({ type: "ended", reason: "unanswered" });
    expect(mounted?.container.textContent).toContain("Fire TV did not answer.");
    mounted?.unmount();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      const heard: Heard[] = [];
      const { services } = await castToTheTv(heard);
      const before = services.navigation.route();
      await act(async () => {
        vi.advanceTimersByTime(castAnswerWithinMs);
        await Promise.resolve();
      });
      expect(mounted?.container.textContent).toContain("Asking Fire TV…");
      await act(async () => {
        vi.advanceTimersByTime(castAnswerGraceMs);
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
      expect(mounted?.container.textContent).toContain("Fire TV did not answer.");
      expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
        reason: "unanswered",
      });
      expect(services.navigation.route()).toEqual(before);
    } finally {
      vi.useRealTimers();
    }
  });

  it("says none is open when no other device is", async () => {
    const services = servedBy(
      { "GET /me/devices": [200, { devices: [device(1, "Laptop", true), device(2, "Fire TV")] }] },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastToDevice fileId={4} positionMs={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    await click(buttonWithText(mounted.container, "Cast to another device"));
    expect(mounted.container.textContent).toContain(
      "None of your other devices has Orpheum open now",
    );
  });

  it("says a device has no Orpheum tab in front, and a cast to it is refused with what to do (D338 §2)", async () => {
    const services = servedBy(
      {
        "GET /me/devices": [
          200,
          { devices: [device(1, "Laptop", true), device(2, "Fire TV"), device(3, "Pixel")] },
        ],
        "POST /me/casts": refusal(409, "conflict"),
      },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastToDevice fileId={4} positionMs={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1, 2, 3], inFrontDeviceIds: [1, 3] });
    await click(buttonWithText(mounted.container, "Cast to another device"));
    const items = [...mounted.container.querySelectorAll(".cast-to__devices li")].map(
      (item) => item.textContent,
    );
    expect(items[0]).toContain("no Orpheum tab is in front");
    expect(items[1]).toContain("Open now");
    await click(buttonWithText(mounted.container, "Fire TV"));
    expect(mounted.container.querySelector("[role=alert]")?.textContent).toBe(
      "Fire TV has no Orpheum tab in front. Bring an Orpheum tab to the front on it, then cast again.",
    );
  });

  it("says in its hello and as the tab moves whether its tab is in front (D338 §2)", async () => {
    const services = servedBy({}, [], "token");
    const server = fakeTogetherServer();
    let visibility: DocumentVisibilityState = "hidden";
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => visibility,
    });
    try {
      mounted = await mount(
        withServices(<LiveProvider>{null}</LiveProvider>, {
          ...services,
          openTogetherSocket: server.openSocket,
        }),
      );
      await server.accept();
      expect(server.heard[0]).toEqual({ type: "live", sessionToken: "token", behind: true });
      visibility = "visible";
      document.dispatchEvent(new Event("visibilitychange"));
      expect(server.heard.at(-1)).toEqual({ type: "front" });
    } finally {
      delete (document as { visibilityState?: unknown }).visibilityState;
    }
  });
});

describe("the page's live connection (D337 §2, §4)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("receives a cast handed to it, opening the film's Play page", async () => {
    const services = servedBy({}, [], "token");
    const sockets = liveAndSession();
    const server = sockets.live;
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Receiving />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await server.accept();
    await server.say({
      type: "receive",
      receive: {
        castId: 5,
        membershipCredential: "cast",
        fileId: 7,
        positionMs: 61_000,
        fromDeviceId: 2,
        fromDeviceName: "Pixel",
      },
    });
    await pressPlay(mounted.container);
    expect(mounted.container.textContent).toContain("receiving cast 5");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 7 });
  });

  it("goes back to the page a cast took it over from as casting ends there, through a cast that replaced it (D338 §1)", async () => {
    const services = servedBy({}, [], "token");
    const before = services.navigation.route();
    expect(before.screen).not.toBe("play");
    const sockets = liveAndSession();
    const server = sockets.live;
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Leaving />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await server.accept();
    const handed = {
      castId: 5,
      membershipCredential: "cast",
      fileId: 7,
      fromDeviceId: 2,
      fromDeviceName: "Pixel",
    };
    await server.say({ type: "receive", receive: handed });
    await pressPlay(mounted.container);
    // A cast replacing the one received is received at once: its device is asked no more.
    await server.say({ type: "receive", receive: { ...handed, castId: 6, fileId: 9 } });
    expect(buttonWithText(mounted.container, "Play")).toBeUndefined();
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 9 });
    await click(buttonWithText(mounted.container, "leave"));
    expect(services.navigation.route()).toEqual(before);
    expect(mounted.container.textContent).not.toContain("receiving");
    expect(tabStorage().getItem(castReturnKey)).toBeNull();
  });

  it("stays on the film, forgetting the page before, as its control is taken back here (D338 §6)", async () => {
    const services = servedBy({}, [], "token");
    const sockets = liveAndSession();
    const server = sockets.live;
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Leaving />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await server.accept();
    await server.say({
      type: "receive",
      receive: { castId: 5, membershipCredential: "c", fileId: 7, fromDeviceId: 2 },
    });
    await pressPlay(mounted.container);
    await click(buttonWithText(mounted.container, "take back"));
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 7 });
    expect(tabStorage().getItem(castReturnKey)).toBeNull();
  });

  it("plays the next episode when the same cast is handed again with it (plans/up-next.md)", async () => {
    const services = servedBy({}, [], "token");
    const sockets = liveAndSession();
    const server = sockets.live;
    mounted = await mount(
      withServices(
        <LiveProvider>
          <ReceivingFile />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await server.accept();
    const handed = {
      castId: 5,
      membershipCredential: "cast",
      fileId: 7,
      fromDeviceId: 2,
      fromDeviceName: "Pixel",
    };
    await server.say({ type: "receive", receive: handed });
    await pressPlay(mounted.container);
    await server.say({
      type: "receive",
      receive: { ...handed, fileId: 8, membershipCredential: "again" },
    });
    expect(mounted.container.textContent).toContain("receiving file 8");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 8 });
  });
});

describe("a film played on a device that casts (§9 Q3)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    noteSending(tabStorage(), false);
  });

  it("goes on to the receiver, the device staying its remote", async () => {
    noteSending(tabStorage(), true);
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/casts": [200, { casts: [{ castId: 3, isSender: true, receiverDeviceId: 2 }] }],
        "POST /me/casts": [201, { castId: 4, isSender: true }],
      },
      heard,
      "token",
    );
    mounted = await mount(
      withServices(
        <PlayOrSendOn fileId={8}>
          <p>playing here</p>
        </PlayOrSendOn>,
        services,
      ),
    );
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts")?.body).toEqual({
      toDeviceId: 2,
      fileId: 8,
    });
    expect(services.navigation.route()).toEqual({ screen: "casting" });
    expect(mounted.container.textContent).not.toContain("playing here");
  });

  it("plays here once the server says the device casts no more, and asks nothing where it never cast", async () => {
    noteSending(tabStorage(), true);
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <PlayOrSendOn fileId={8}>
          <p>playing here</p>
        </PlayOrSendOn>,
        servedBy({ "GET /me/casts": [200, { casts: [] }] }, heard, "token"),
      ),
    );
    expect(mounted.container.textContent).toContain("playing here");
    expect(isNotedSending(tabStorage())).toBe(false);
    mounted.unmount();
    heard.length = 0;
    mounted = await mount(
      withServices(
        <PlayOrSendOn fileId={8}>
          <p>playing here</p>
        </PlayOrSendOn>,
        servedBy({}, heard, "token"),
      ),
    );
    expect(mounted.container.textContent).toContain("playing here");
    expect(heard).toEqual([]);
  });
});
