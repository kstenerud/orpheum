// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { tabStorage } from "./browser-storage.ts";
import { castAnswerGraceMs, castAnswerWithinMs } from "./cast.ts";
import { useReceiving } from "./cast-receiver.tsx";
import { castReceivingKey } from "./cast-receiving-mark.ts";
import { CastTargets, PlayOrSendOn, sendingKnownWithinMs } from "./cast-to.tsx";
import { LiveProvider, useLive } from "./live.tsx";
import {
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { createNavigation } from "./navigation.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer, liveAndSession, type FakeTogetherServer } from "./test-together.ts";

/** The hello a signed-in page opens its live channel with (D416 §11). */
const liveHello = { type: "hello", channel: "live", sessionToken: "token" };

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

/** The cast the Laptop sends to the Fire TV, as the live connection says it (D364 §6). */
const sendingToTheTv = {
  castId: 9,
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  fileId: 4,
  itemName: "Metropolis",
  durationMs: 0,
  isAccepted: false,
};

/** Answers Use to the cast the receiver asks about (D364 §2). */
async function pressUse(container: HTMLElement): Promise<void> {
  const use = buttonWithText(container, "Use");
  expect(use).toBeDefined();
  await click(use);
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

/** What the receiver shows, and its way out, as children of the provider. */
function Leaving(): JSX.Element {
  const { receiving, leaveReceiving } = useLive();
  return (
    <>
      <p>{receiving === undefined ? "" : `receiving cast ${String(receiving.castId)}`}</p>
      <button type="button" onClick={leaveReceiving}>
        leave
      </button>
    </>
  );
}

/** The receiver of film 7's Play page, and its way out as its control is taken back (useReceiving). */
function TakingBack(): JSX.Element {
  const receiving = useReceiving(7);
  return (
    <>
      <p>{receiving === undefined ? "" : `receiving cast ${String(receiving.cast.castId)}`}</p>
      <button type="button" onClick={receiving?.onLeave}>
        take back
      </button>
    </>
  );
}

/** The film playing here as the casting page was reached: film 4, at 61 s. */
const playingHere = (): { fileId: number; positionMs: number } => ({
  fileId: 4,
  positionMs: 61_000,
});

describe("setting a cast up (D364 §1, §2; D337 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("lists the person's other devices open now, and starts a session with the one chosen, moving the film playing here", async () => {
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
          <CastTargets film={playingHere} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await server.accept();
    expect(server.heard[0]).toEqual(liveHello);
    await server.say({ type: "devices", deviceIds: [1, 2] });
    expect(mounted.container.textContent).toContain("Fire TV");
    expect(mounted.container.textContent).not.toContain("Phone, closed");
    expect(buttonWithText(mounted.container, "Laptop")).toBeUndefined();
    await click(buttonWithText(mounted.container, "Fire TV"));
    const cast = heard.find((each) => each.methodAndPath === "POST /me/casts");
    expect(cast?.body).toEqual({ toDeviceId: 2, fileId: 4, positionMs: 61_000 });
    // The receiver is asked once whether to be used (D364 §2), and this page says so till it answers.
    expect(mounted.container.textContent).toContain("Asking Fire TV…");
    await sockets.session.accept();
    expect(sockets.session.heard[0]).toMatchObject({ credential: "sender", role: "panel" });
  });

  it("starts a session with nothing playing where no film was playing here (D364 §1)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [device(1, "Laptop", true), device(2, "Fire TV")] }],
        "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
      },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "devices", deviceIds: [1, 2] });
    await click(buttonWithText(mounted.container, "Fire TV"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts")?.body).toEqual({
      toDeviceId: 2,
    });
  });

  /** The Laptop casting film 4 to the Fire TV, and what the page then shows and says. */
  async function castToTheTv(heard: Heard[]): Promise<ReturnType<typeof liveAndSession>> {
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
          <CastTargets film={playingHere} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "devices", deviceIds: [1, 2] });
    await click(buttonWithText(mounted.container, "Fire TV"));
    await sockets.session.accept();
    await sockets.session.say(castWelcome);
    return sockets;
  }

  it("says its receiver declined, offering the devices again (D358 §3)", async () => {
    const sockets = await castToTheTv([]);
    await sockets.session.say({ type: "ended", reason: "declined" });
    expect(mounted?.container.textContent).toContain("Fire TV declined.");
    expect(buttonWithText(mounted!.container, "Fire TV")).toBeDefined(); // to cast again
  });

  // The owner, 2026-11-03: the phone stayed asking, with no way out, for a cast the server had lost.
  it("cancels the casting request while its receiver is asked, offering the devices again", async () => {
    const heard: Heard[] = [];
    await castToTheTv(heard);
    await click(buttonWithText(mounted!.container, "Cancel casting request"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "stopped",
    });
    expect(mounted?.container.textContent).toContain(
      "You cancelled the casting request to Fire TV.",
    );
    expect(mounted?.container.textContent).not.toContain("Asking Fire TV…");
    expect(buttonWithText(mounted!.container, "Fire TV")).toBeDefined(); // to cast again
  });

  it("says the cast is over once the server no longer says this device sends it, lost as it restarted", async () => {
    const sockets = await castToTheTv([]);
    await sockets.live.say({ type: "sending", sending: sendingToTheTv });
    expect(mounted?.container.textContent).toContain("Asking Fire TV…");
    await sockets.live.say({ type: "sending" });
    expect(mounted?.container.textContent).toContain(
      "The cast ended: the server no longer knows it.",
    );
    expect(buttonWithText(mounted!.container, "Fire TV")).toBeDefined(); // to cast again
  });

  it("says nothing more once its receiver said Use, the casting page showing the session (D364 §2)", async () => {
    const sockets = await castToTheTv([]);
    await sockets.live.say({ type: "sending", sending: { ...sendingToTheTv, isAccepted: true } });
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(mounted?.container.textContent).not.toContain("ended before it started");
  });

  it("says its receiver did not answer, as it says or after a minute and a little more (D358 §4)", async () => {
    const said = await castToTheTv([]);
    await said.session.say({ type: "ended", reason: "unanswered" });
    expect(mounted?.container.textContent).toContain("Fire TV did not answer.");
    mounted?.unmount();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      const heard: Heard[] = [];
      await castToTheTv(heard);
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
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    expect(mounted.container.textContent).toContain(
      "None of your other devices has Orpheum open now",
    );
  });

  it("lists a device signed in while the list is open, as it connects (the owner, staging, 2026-11-07)", async () => {
    const routes: Parameters<typeof servedBy>[0] = {
      "GET /me/devices": [200, { devices: [device(1, "Mac", true)] }],
    };
    const services = servedBy(routes, [], "token");
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    expect(mounted.container.textContent).toContain("None of your other devices");
    routes["GET /me/devices"] = [200, { devices: [device(1, "Mac", true), device(2, "Phone")] }];
    await server.say({ type: "devices", deviceIds: [1, 2], inFrontDeviceIds: [2] });
    expect(mounted.container.textContent).toContain("Phone");
  });

  // The owner's S23 cast to the Mac that hosted the Watch Together session, which ended it (2026-10-30).
  it("lists no device in the Watch Together session its person is in (D384 §3)", async () => {
    const services = servedBy(
      {
        "GET /me/devices": [
          200,
          {
            devices: [
              device(1, "Phone", true),
              { ...device(2, "Mac"), isInYourWatchTogether: true },
              device(3, "Fire TV"),
            ],
          },
        ],
      },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1, 2, 3] });
    expect(buttonWithText(mounted.container, "Fire TV")).toBeDefined();
    expect(mounted.container.textContent).not.toContain("Mac");
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
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1, 2, 3], inFrontDeviceIds: [1, 3] });
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

  it("shows a device receiving a cast unavailable, and a cast to one raced is refused with its name (D386 §1)", async () => {
    const services = servedBy(
      {
        "GET /me/devices": [
          200,
          { devices: [device(1, "Laptop", true), device(2, "Fire TV"), device(3, "Pixel")] },
        ],
        "POST /me/casts": refusal(409, "receiving_a_cast"),
      },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({
      type: "devices",
      deviceIds: [1, 2, 3],
      inFrontDeviceIds: [1, 2, 3],
      receivingDeviceIds: [2],
    });
    const items = [...mounted.container.querySelectorAll(".cast-to__devices li")].map(
      (item) => item.textContent,
    );
    expect(items[0]).toContain("Showing another cast");
    expect(items[1]).toContain("Open now");
    expect(buttonWithText(mounted.container, "Fire TV")?.disabled).toBe(true);
    await click(buttonWithText(mounted.container, "Pixel"));
    expect(mounted.container.querySelector("[role=alert]")?.textContent).toBe(
      "Pixel is showing another cast. Cast to it once that cast ends.",
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
      expect(server.heard[0]).toEqual({ ...liveHello, behind: true });
      visibility = "visible";
      document.dispatchEvent(new Event("visibilitychange"));
      expect(server.heard.at(-1)).toEqual({ type: "front" });
    } finally {
      delete (document as { visibilityState?: unknown }).visibilityState;
    }
  });
});

describe("the page's live connection, receiving (D364 §2, §3, §5; D337 §4)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** A cast handed to the receiver: cast 5, of film 7, from the Pixel. */
  const handed = {
    castId: 5,
    membershipCredential: "cast",
    fileId: 7,
    fromDeviceId: 2,
    fromDeviceName: "Pixel",
  };

  it("asks once whether to be used for playback, tells the server its Use, and opens the film's Play page", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, heard, "token");
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
    await server.say({ type: "receive", receive: { ...handed, positionMs: 61_000 } });
    expect(mounted.container.textContent).toContain("Pixel wants to use this device for playback");
    await pressUse(mounted.container);
    expect(heard.map((each) => each.methodAndPath)).toContain("POST /me/casts/5/accept");
    expect(mounted.container.textContent).toContain("receiving cast 5");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 7 });
  });

  it("goes full screen and ready between videos, then plays the next with no question (D364 §3, §5)", async () => {
    const services = servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, [], "token");
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <ReceivingFile />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "receive", receive: { ...handed, fileId: 0 } });
    await pressUse(mounted.container);
    expect(services.navigation.route()).toEqual({ screen: "cast-ready" });
    await sockets.live.say({ type: "receive", receive: { ...handed, fileId: 8 } });
    expect(buttonWithText(mounted.container, "Use")).toBeUndefined();
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 8 });
    await sockets.live.say({ type: "receive", receive: { ...handed, fileId: 0 } });
    expect(services.navigation.route()).toEqual({ screen: "cast-ready" });
    expect(mounted.container.textContent).toContain("receiving file 0");
  });

  it("goes to its home page in the page's place as a cast ends there, through the cast handed again with another film (D384 §1)", async () => {
    const services = servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, [], "token");
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
    await server.say({ type: "receive", receive: handed });
    await pressUse(mounted.container);
    // The cast handed again with another film is received at once: its device is asked no more.
    await server.say({ type: "receive", receive: { ...handed, fileId: 9 } });
    expect(buttonWithText(mounted.container, "Use")).toBeUndefined();
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 9 });
    await click(buttonWithText(mounted.container, "leave"));
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(services.navigation.behind().at(-1)).toEqual({ screen: "play", fileId: 7 });
    expect(mounted.container.textContent).not.toContain("receiving");
    expect(tabStorage().getItem(castReceivingKey)).toBeNull();
  });

  // The owner's staging, 2026-10-29: casting Forward to the Summit, the person started Watch Together;
  // the cast handed again with its host's part, of the same film, was kept as it was, and the
  // receiver stayed out of the session the person's Watch Together page showed.
  it("takes the same cast handed again with its person's part in Watch Together, and keeps it as a credential is minted afresh for the same part (D364 §11, D372 §8)", async () => {
    const services = servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, [], "token");
    const sockets = liveAndSession();
    function Part(): JSX.Element {
      const { receiving, leaveReceiving } = useLive();
      return (
        <>
          <p>{`part ${receiving?.togetherCredential ?? "none"}`}</p>
          <button type="button" onClick={leaveReceiving}>
            take back
          </button>
        </>
      );
    }
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Part />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "receive", receive: handed });
    await pressUse(mounted.container);
    expect(mounted.container.textContent).toContain("part none");
    await sockets.live.say({
      type: "receive",
      receive: { ...handed, membershipCredential: "cast-2", togetherCredential: "hosted" },
    });
    expect(mounted.container.textContent).toContain("part hosted");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 7 });
    await sockets.live.say({
      type: "receive",
      receive: { ...handed, membershipCredential: "cast-3", togetherCredential: "hosted-again" },
    });
    expect(mounted.container.textContent).toContain("part hosted");
    expect(mounted.container.textContent).not.toContain("hosted-again");
    await click(buttonWithText(mounted.container, "take back"));
  });

  // The owner's Fire TV, 2026-10-29: a cast stopped, the TV's Back then played the cast's film.
  it("goes home as casting ends, and Back from there never plays the cast's film (D384 §1, D383 §1)", async () => {
    const place = fakePlace("/titles/3");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "play", fileId: 4 });
    const services = {
      ...servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, [], "token"),
      navigation,
    };
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Leaving />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "receive", receive: handed });
    await pressUse(mounted.container);
    expect(navigation.route()).toEqual({ screen: "play", fileId: 7 });
    // The receiver's own entry, which a Fire TV's Back steps off to take control back.
    act(() => {
      navigation.go(navigation.route());
    });
    await click(buttonWithText(mounted.container, "leave"));
    expect(navigation.route()).toEqual({ screen: "home" });
    act(() => {
      place.history.back();
    });
    expect(navigation.route().screen).not.toBe("play");
  });

  it("goes home, forgetting it received, as its control is taken back here (D385 §1, amending D338 §6)", async () => {
    const services = servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, [], "token");
    const sockets = liveAndSession();
    const server = sockets.live;
    mounted = await mount(
      withServices(
        <LiveProvider>
          <TakingBack />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "receive", receive: handed });
    await pressUse(mounted.container);
    expect(mounted.container.textContent).toContain("receiving cast 5");
    await click(buttonWithText(mounted.container, "take back"));
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(mounted.container.textContent).not.toContain("receiving");
    expect(tabStorage().getItem(castReceivingKey)).toBeNull();
  });
});

describe("a film played on a device that casts (D364 §3, §4; D384 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** The cast this device sends, as GET /me/casts lists it, its session's credential the remote's. */
  const listed = {
    castId: 9,
    isSender: true,
    fileId: 8,
    itemId: 2,
    itemName: "Metropolis",
    durationMs: 600_000,
    senderDeviceId: 1,
    senderName: "Pixel",
    receiverDeviceId: 2,
    receiverName: "Fire TV",
    membershipCredential: "remote",
  };

  /**
   * The Play page's film 8, from 30 s, on a device whose live connection says what it sends; the page
   * shows where its film plays here from.
   */
  async function playOn(
    heard: Heard[],
    sending: Readonly<Record<string, unknown>> | undefined,
    shouldPlayHere = false,
  ): Promise<
    ReturnType<typeof servedBy> & { readonly sockets: ReturnType<typeof liveAndSession> }
  > {
    const services = servedBy(
      {
        "POST /me/casts/9/play": [200, { castId: 9, isSender: true }],
        "GET /me/casts": [200, { casts: [listed] }],
        "GET /files/8/up-next": [200, {}],
        "GET /files/4/up-next": [200, {}],
      },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <PlayOrSendOn
            fileId={8}
            atMs={30_000}
            shouldPlayHere={shouldPlayHere}
            playHere={(from) => <p>{`playing here from ${String(from.atMs)}`}</p>}
          />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say(
      sending === undefined ? { type: "sending" } : { type: "sending", sending },
    );
    return { ...services, sockets };
  }

  /** The cast's session as its remote joins it: the receiver's film paused at 61 s. */
  async function remoteJoined(session: FakeTogetherServer): Promise<void> {
    await session.accept();
    await session.say({
      type: "welcome",
      seq: 3,
      sessionId: 9,
      memberId: 2,
      deviceId: 5,
      isHost: true,
      rights: { canPlayPause: true, canSeek: true },
      group: { state: "Paused", positionMs: 61_000, rate: 1, fileId: 8 },
      roster: [],
    });
  }

  it("goes on to the receiver, from where it was to start, the page staying where it is as the cast's remote", async () => {
    const heard: Heard[] = [];
    const services = await playOn(heard, { ...sendingToTheTv, isAccepted: true });
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/play")?.body).toEqual({
      fileId: 8,
      positionMs: 30_000,
    });
    await services.sockets.live.say({
      type: "sending",
      sending: { ...sendingToTheTv, fileId: 8, isAccepted: true },
    });
    expect(services.navigation.route()).toEqual({ screen: "films" });
    expect(mounted?.container.textContent).toContain("Playing on Fire TV");
    expect(mounted?.container.textContent).not.toContain("playing here");
  });

  it("plays here where the device casts nothing, its receiver has not said Use, or the remote said to play here", async () => {
    for (const [sending, shouldPlayHere] of [
      [undefined, false],
      [sendingToTheTv, false],
      [{ ...sendingToTheTv, isAccepted: true }, true],
    ] as const) {
      const heard: Heard[] = [];
      await playOn(heard, sending, shouldPlayHere);
      expect(mounted?.container.textContent).toContain("playing here from 30000");
      expect(heard).toEqual([]);
      mounted?.unmount();
      mounted = undefined;
    }
  });

  // The owner's S23, 2026-10-30: a cast left the page cast from for a casting page of its own.
  it("becomes the cast's remote as a cast started over the page takes its film, and plays it here again from where the receiver stood as the cast is stopped", async () => {
    const heard: Heard[] = [];
    const { sockets } = await playOn(heard, undefined);
    expect(mounted?.container.textContent).toContain("playing here from 30000");
    await sockets.live.say({
      type: "sending",
      sending: { ...sendingToTheTv, fileId: 8, isAccepted: true },
    });
    await remoteJoined(sockets.session);
    expect(mounted?.container.textContent).toContain("Playing on Fire TV");
    expect(mounted?.container.textContent).not.toContain("playing here");
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/play")).toBe(false);
    await sockets.session.say({ type: "ended", reason: "stopped" });
    await sockets.live.say({ type: "sending" });
    expect(mounted?.container.textContent).toContain("playing here from 61000");
  });

  it("plays here at once with the test services' own stand-in socket, which says the device casts nothing", async () => {
    mounted = await mount(
      withServices(
        <LiveProvider>
          <PlayOrSendOn fileId={8} shouldPlayHere={false} playHere={() => <p>playing here</p>} />
        </LiveProvider>,
        servedBy({}, [], "token"),
      ),
    );
    expect(mounted.container.textContent).toContain("playing here");
  });

  it("plays here once the server has not said what the device sends within a few seconds", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      const server = fakeTogetherServer();
      mounted = await mount(
        withServices(
          <LiveProvider>
            <PlayOrSendOn fileId={8} shouldPlayHere={false} playHere={() => <p>playing here</p>} />
          </LiveProvider>,
          { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket },
        ),
      );
      expect(mounted.container.textContent).not.toContain("playing here");
      await act(async () => {
        vi.advanceTimersByTime(sendingKnownWithinMs);
        await Promise.resolve();
      });
      expect(mounted.container.textContent).toContain("playing here");
    } finally {
      vi.useRealTimers();
    }
  });
});
