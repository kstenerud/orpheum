// SPDX-License-Identifier: MPL-2.0

// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";

import { reachesUnderLoader } from "./client-reaches.ts";
import { loaderMajor, loaderMinor, type OrpheumLoader } from "./loader-interface.ts";
import { createSession } from "./session.ts";

const nothing = (): (() => void) => () => undefined;

/** A loader whose every part is a stand-in, the browser's asking among them as given. */
function loaderWith(onLocalNetworkAsking?: OrpheumLoader["onLocalNetworkAsking"]): OrpheumLoader {
  return {
    major: loaderMajor,
    minor: loaderMinor,
    core: { coreId: "k7xq2", fingerprint: "a-fingerprint", realm: "karlnas" },
    fetch: () => Promise.resolve(new Response("{}")),
    openTogetherSocket: () => {
      throw new Error("no socket in this test");
    },
    onPathMove: nothing,
    session: createSession(undefined),
    setMediaCredentials: () => undefined,
    switchHousehold: () => undefined,
    ...(onLocalNetworkAsking === undefined ? {} : { onLocalNetworkAsking }),
  };
}

describe("how the app reaches its core under a discovery's loader (D308)", () => {
  it("takes the loader's word on whether the browser asks for the local network, and none from an older loader (W2c item 8)", () => {
    const told: boolean[] = [];
    const asking: OrpheumLoader["onLocalNetworkAsking"] = (listener) => {
      listener(true);
      return () => undefined;
    };
    const reaches = reachesUnderLoader(loaderWith(asking), document);
    reaches.onLocalNetworkAsking?.((isAsking) => {
      told.push(isAsking);
    });
    expect(told).toEqual([true]);
    expect(reachesUnderLoader(loaderWith(), document).onLocalNetworkAsking).toBeUndefined();
  });

  it("sends the app's requests the loader's way, and names the realm it reached", async () => {
    const loader = loaderWith();
    const reaches = reachesUnderLoader(loader, document);
    expect(reaches.fetch).toBe(loader.fetch);
    expect(reaches.realm).toBe("karlnas");
    expect(await (await reaches.fetch(new Request("https://core.example/api/v1/me"))).text()).toBe(
      "{}",
    );
  });
});
