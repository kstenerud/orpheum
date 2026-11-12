// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { tabStorage } from "./browser-storage.ts";
import { useReceiving } from "./cast-receiver.tsx";
import { castReceivingKey } from "./cast-receiving-mark.ts";
import { LiveProvider, useLive } from "./live.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { currentNotice, dismissNotice } from "./notice.ts";
import {
  buttonWithText,
  click,
  mount,
  pressToStartTheCast,
  refusal,
  servedBy,
  settleUntil,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { createNavigation } from "./navigation.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";

// The receiving side of the page's live connection, apart from setting a cast up (cast-to.test.tsx).

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

describe("the page's live connection, receiving (D364 §2, §3, §5; D337 §4; D468)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    const notice = currentNotice();
    if (notice !== undefined) {
      dismissNotice(notice);
    }
  });

  /** A cast handed to the receiver: cast 5, of film 7, from the Pixel. */
  const handed = {
    castId: 5,
    membershipCredential: "cast",
    fileId: 7,
    fromDeviceId: 2,
    fromDeviceName: "Pixel",
  };

  /** The receiver's provider over Receiving, its connection accepted, and what its server heard. */
  async function receiver(answers: Answers = { "POST /me/casts/5/accept": [200, { castId: 5 }] }) {
    const heard: Heard[] = [];
    const services = servedBy(answers, heard, "token");
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
    const accepts = (): number =>
      heard.filter((each) => each.methodAndPath === "POST /me/casts/5/accept").length;
    return { services, server, accepts, container: mounted.container };
  }

  it("takes a cast at once on a page pressed since it loaded, asking nothing, and opens the film's Play page (D468 §1)", async () => {
    Object.defineProperty(navigator, "userActivation", {
      configurable: true,
      value: { isActive: false, hasBeenActive: true },
    });
    try {
      const { services, server, accepts, container } = await receiver();
      await server.say({ type: "receive", receive: { ...handed, positionMs: 61_000 } });
      await settleUntil(() => container.textContent.includes("receiving cast 5"));
      expect(accepts()).toBe(1);
      expect(services.navigation.route()).toEqual({ screen: "play", fileId: 7 });
      expect(document.body.querySelector("button")).toBeNull();
    } finally {
      Reflect.deleteProperty(navigator, "userActivation");
    }
  });

  it("takes a cast at once on a page never pressed, so its sender hands it off, and holds the film for the press that starts it (D468 §2, §3)", async () => {
    const { services, server, accepts, container } = await receiver();
    const before = services.navigation.route();
    await server.say({ type: "receive", receive: { ...handed, positionMs: 61_000 } });
    await settleUntil(() => buttonWithText(document.body, "Click to start") !== undefined);
    expect(accepts()).toBe(1);
    expect(container.textContent).not.toContain("receiving");
    expect(services.navigation.route()).toEqual(before);
    await pressToStartTheCast();
    expect(accepts()).toBe(1);
    expect(container.textContent).toContain("receiving cast 5");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 7 });
    expect(buttonWithText(document.body, "Click to start")).toBeUndefined();
  });

  it("says its sender stopped where the cast is gone as it is taken, and receives nothing", async () => {
    const { services, server, container } = await receiver({
      "POST /me/casts/5/accept": refusal(404, "notFound"),
    });
    const before = services.navigation.route();
    await server.say({ type: "receive", receive: handed });
    await settleUntil(() => currentNotice() !== undefined);
    expect(currentNotice()?.text).toBe("Pixel stopped casting.");
    expect(container.textContent).not.toContain("receiving");
    expect(buttonWithText(document.body, "Click to start")).toBeUndefined();
    expect(services.navigation.route()).toEqual(before);
  });

  it("heads its player's overlay with Stop casting here, which a device playing its own film has not (D446 §3)", async () => {
    const services = servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, [], "token");
    const sockets = liveAndSession();
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    mounted = await mount(
      withServices(
        <LiveProvider>
          <div className="film-player__stage">
            <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
          </div>
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    const overlay = (): HTMLElement =>
      mounted?.container.querySelector<HTMLElement>(".player-overlay") ?? document.body;
    expect(buttonWithText(overlay(), "Stop casting here")).toBeUndefined();
    await sockets.live.accept();
    await sockets.live.say({ type: "receive", receive: handed });
    await pressToStartTheCast();
    expect(buttonWithText(overlay(), "Stop casting here")).toBeDefined();
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
    await pressToStartTheCast();
    expect(services.navigation.route()).toEqual({ screen: "cast-ready" });
    await sockets.live.say({ type: "receive", receive: { ...handed, fileId: 8 } });
    expect(buttonWithText(document.body, "Click to start")).toBeUndefined();
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
    await pressToStartTheCast();
    // The cast handed again with another film is received at once: its device is asked no more.
    await server.say({ type: "receive", receive: { ...handed, fileId: 9 } });
    expect(buttonWithText(document.body, "Click to start")).toBeUndefined();
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
    await pressToStartTheCast();
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
    await pressToStartTheCast();
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
    await pressToStartTheCast();
    expect(mounted.container.textContent).toContain("receiving cast 5");
    await click(buttonWithText(mounted.container, "take back"));
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(mounted.container.textContent).not.toContain("receiving");
    expect(tabStorage().getItem(castReceivingKey)).toBeNull();
  });
});
