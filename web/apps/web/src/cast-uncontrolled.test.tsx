// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { tabStorage } from "./browser-storage.ts";
import { castReceivingKey } from "./cast-receiving-mark.ts";
import { PlayOrSendOn } from "./cast-to.tsx";
import { LiveProvider, useLive } from "./live.tsx";
import { createNavigation, type Navigation } from "./navigation.ts";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Answers,
  type Mounted,
} from "./test-dom.tsx";
import { fakePlace, withServices, type FakePlace } from "./test-services.tsx";
import { liveAndSession, type FakeTogetherServer } from "./test-together.ts";

/** Whether the page receives a cast, as a child of the provider. */
function Receiving(): string {
  const { receiving } = useLive();
  return receiving === undefined ? "" : `receiving cast ${String(receiving.castId)}`;
}

/** The cast the TV received before its page loaded again: film 7's, from the Laptop. */
const handed = {
  castId: 5,
  membershipCredential: "cast",
  fileId: 7,
  fromDeviceId: 2,
  fromDeviceName: "Laptop",
};

// The owner's Fire TV received a cast as staging was deployed; its page loaded again as a plain Play
// page of the cast's film, playing with no controller (staging, 2026-10-30).
describe("a page loaded again while its tab received a cast (D383 §2, D384 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    tabStorage().removeItem(castReceivingKey);
  });

  /**
   * The TV's tab loaded again on the cast's Play page, film 7's, which noted its title, 10, on its
   * entry before; its live connection open, the server not yet heard.
   */
  async function loadedAgain(answers: Answers): Promise<{
    readonly place: FakePlace;
    readonly navigation: Navigation;
    readonly live: FakeTogetherServer;
    readonly shown: Mounted;
  }> {
    tabStorage().setItem(castReceivingKey, "5");
    const place = fakePlace("/titles/3");
    place.history.pushState({ appDepth: 1, inPlace: "/titles/10" }, "", "/play/7");
    const navigation = createNavigation(place, false, false);
    const services = { ...servedBy(answers, [], "token"), navigation };
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Receiving />
          <PlayOrSendOn fileId={7} shouldPlayHere={false}>
            <p>the film plays</p>
          </PlayOrSendOn>
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "sending" });
    await sockets.live.say({ type: "devices", deviceIds: [1, 2], inFrontDeviceIds: [1] });
    return { place, navigation, live: sockets.live, shown: mounted };
  }

  /** The server's answer to the ping the page sent once told the devices: nothing was handed. */
  async function handedNothing(live: FakeTogetherServer): Promise<void> {
    const ping = live.heard.find((each) => each["type"] === "ping");
    expect(ping).toBeDefined();
    await live.say({ type: "pong", tMono: ping?.["tMono"] });
  }

  it("holds the film, then goes home in its place once the server hands it nothing", async () => {
    const { place, navigation, live, shown } = await loadedAgain({});
    expect(shown.container.textContent).not.toContain("the film plays");
    await handedNothing(live);
    await settleUntil(() => navigation.route().screen !== "play");
    expect(navigation.route()).toEqual({ screen: "home" });
    expect(place.addresses).toEqual(["/titles/3", "/"]);
    expect(tabStorage().getItem(castReceivingKey)).toBeNull();
  });

  it("receives its cast again where the server hands it, and plays the film", async () => {
    const { navigation, live, shown } = await loadedAgain({
      "POST /me/casts/5/accept": [200, { castId: 5 }],
    });
    await live.say({ type: "receive", receive: handed });
    await click(buttonWithText(shown.container, "Use"));
    await handedNothing(live);
    expect(shown.container.textContent).toContain("receiving cast 5");
    expect(shown.container.textContent).toContain("the film plays");
    expect(navigation.route()).toEqual({ screen: "play", fileId: 7 });
  });
});
