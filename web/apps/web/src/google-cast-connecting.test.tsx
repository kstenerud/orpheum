// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useLayoutEffect, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ChromecastConnectingOverTheFilm } from "./chromecast-connecting.tsx";
import { CastTargets, PlayOrSendOn } from "./cast-to.tsx";
import { useEndsTheConnectingOnceTaken } from "./google-cast.ts";
import { useChromecastConnecting } from "./google-cast-connecting.ts";
import { LiveProvider } from "./live.tsx";
import { currentNotice, dismissNotice } from "./notice.ts";
import { cast, filmReads, sending } from "./test-castee.tsx";
import {
  buttonWithText,
  click,
  mount,
  refusal,
  settleUntil,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import {
  chromeHints,
  givenCastSdk,
  givenHints,
  underTheLoader,
  type FakeCastDevice,
} from "./test-google-cast.ts";
import { withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";

/** This laptop, as GET /me/devices lists it. */
const thisLaptop = { id: 1, name: "Laptop", kind: "computer", isThisDevice: true };

/** How many times the film here was put on the page: once while it plays on unbroken. */
const filmMounts = { count: 0 };

/** The connect's notice over the film here, what each render of the film here showed of it. */
function FilmHere(props: { readonly seen: boolean[] }): JSX.Element {
  const isConnecting = useChromecastConnecting() !== undefined;
  useLayoutEffect(() => {
    props.seen.push(isConnecting);
  });
  useLayoutEffect(() => {
    filmMounts.count += 1;
  }, []);
  return (
    <div className="film-here">
      <ChromecastConnectingOverTheFilm />
    </div>
  );
}

/** What the status tray does for the connect: ends it once the live connection says it taken. */
function EndsTheConnect(): null {
  useEndsTheConnectingOnceTaken();
  return null;
}

describe("a Chromecast being connected, on the film's page (D570)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    // The connect is the page's, kept past its mount: one left by a test is given up.
    act(() => {
      cancel()?.click();
    });
    mounted?.unmount();
    mounted = undefined;
    Reflect.deleteProperty(navigator, "userAgentData");
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  /** Film 4's Play page playing here, its Chromecast button pressed, Chrome's picker open. */
  async function picked(
    answers: Answers = {},
    onChromecastPicked?: () => void,
  ): Promise<{
    readonly device: FakeCastDevice;
    readonly heard: Heard[];
    readonly sockets: ReturnType<typeof liveAndSession>;
    readonly seen: boolean[];
  }> {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    const heard: Heard[] = [];
    const sockets = liveAndSession();
    const services = underTheLoader(
      {
        "GET /me/devices": [200, { devices: [thisLaptop] }],
        "GET /me/casts": [200, { casts: [cast] }],
        "GET /files/4/up-next": [200, {}],
        "GET /files/4/trickplay": [404, {}],
        ...filmReads,
        ...answers,
      },
      heard,
      sockets.live,
    );
    givenHints(chromeHints);
    const device = givenCastSdk(vi.stubGlobal);
    const seen: boolean[] = [];
    mounted = await mount(
      withServices(
        <LiveProvider>
          <EndsTheConnect />
          <PlayOrSendOn
            fileId={4}
            atMs={61_000}
            shouldPlayHere
            playHere={() => <FilmHere seen={seen} />}
          />
          <CastTargets
            film={() => ({ fileId: 4, positionMs: 61_000 })}
            onChromecastPicked={onChromecastPicked}
          />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "devices", deviceIds: [1] });
    await sockets.live.say({ type: "sending" });
    await settleUntil(() => buttonWithText(page(), "Chromecast") !== undefined);
    return { device, heard, sockets, seen };
  }

  const page = (): HTMLElement => mounted?.container ?? document.body;
  const notice = (): string | undefined =>
    page().querySelector(".chromecast-connecting .pill")?.textContent ?? undefined;
  const cancel = (): HTMLElement | undefined =>
    [...page().querySelectorAll<HTMLElement>(".chromecast-connecting button")].find(
      (each) => each.textContent === "Cancel",
    );
  const made = (heard: readonly Heard[]): boolean =>
    heard.some((each) => each.methodAndPath === "POST /me/casts");

  it("says ‹TV› is being connected from the pick, with Cancel, over the film playing here (§2)", async () => {
    const { device } = await picked();
    await click(buttonWithText(page(), "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    expect(notice()).toBe("Connecting to Living room TV…");
    expect(cancel()).toBeDefined();
    expect(page().querySelector(".film-here")).not.toBeNull();
  });

  it("shows nothing where the picker is closed with no TV picked (§1)", async () => {
    const { device } = await picked();
    device.picksNone();
    await click(buttonWithText(page(), "Chromecast"));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });
    expect(device.sent).toHaveLength(0);
    expect(page().querySelector(".chromecast-connecting")).toBeNull();
  });

  it("tells, on the film's page, why a connect failed once the picker closed the menu, in the menu's own words (§3)", async () => {
    const onPicked = vi.fn();
    const { device } = await picked(
      {
        "POST /me/casts": refusal(409, "conflict"),
      },
      onPicked,
    );
    await click(buttonWithText(page(), "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    expect(onPicked).toHaveBeenCalledOnce();
    await act(async () => {
      device.say({ type: "ready", deviceId: 12 });
      await Promise.resolve();
    });
    await settleUntil(() => currentNotice() !== undefined);
    const told = currentNotice();
    if (told !== undefined) {
      dismissNotice(told);
    }
    expect(told?.text).toBe(
      "Living room TV has no Orpheum tab in front. Bring an Orpheum tab to the front on it, then cast again.",
    );
    expect(page().querySelector(".chromecast-connecting")).toBeNull();
  });

  it("gives the cast up at Cancel before the TV's receiver is ready: its Cast session ended, no cast made, the film on here (§3)", async () => {
    const { device, heard } = await picked({
      "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
    });
    await click(buttonWithText(page(), "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    const endingsBefore = device.endings();
    const mountsBefore = filmMounts.count;
    await click(cancel());
    expect(page().querySelector(".chromecast-connecting")).toBeNull();
    expect(device.endings()).toBeGreaterThan(endingsBefore); // at once, not at the receiver's ready
    await act(async () => {
      device.say({ type: "ready", deviceId: 12 });
      await vi.advanceTimersByTimeAsync(1_000);
    });
    expect(made(heard)).toBe(false);
    expect(page().querySelector(".film-here")).not.toBeNull();
    expect(filmMounts.count).toBe(mountsBefore); // the film here plays on, never put on afresh
  });

  it("clears the notice as the connect fails (§3)", async () => {
    const { device, heard } = await picked({ "POST /me/casts": [500, {}] });
    await click(buttonWithText(page(), "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    await act(async () => {
      device.say({ type: "ready", deviceId: 12 });
      await Promise.resolve();
    });
    await settleUntil(() => made(heard));
    await settleUntil(() => page().querySelector(".chromecast-connecting") === null);
    expect(page().querySelector(".film-here")).not.toBeNull();
  });

  it("hands over to the cast's page as its receiver takes the cast, the film here never drawn without the notice meanwhile (§4)", async () => {
    const { device, heard, sockets, seen } = await picked({
      "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
    });
    await click(buttonWithText(page(), "Chromecast"));
    await settleUntil(() => device.sent.length === 1);
    await act(async () => {
      device.say({ type: "ready", deviceId: 12 });
      await Promise.resolve();
    });
    await settleUntil(() => made(heard));
    await sockets.live.say({ type: "sending", sending: { ...sending, isAccepted: false } });
    expect(notice()).toBe("Connecting to Living room TV…");
    await sockets.live.say({ type: "sending", sending });
    expect(page().querySelector(".film-here")).toBeNull();
    expect(page().querySelector(".chromecast-connecting")).toBeNull();
    const fromThePick = seen.slice(seen.indexOf(true));
    expect(fromThePick.length).toBeGreaterThan(0);
    expect(fromThePick.every(Boolean)).toBe(true);
  });
});
