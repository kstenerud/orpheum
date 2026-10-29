// SPDX-License-Identifier: MPL-2.0

// @vitest-environment happy-dom
import { mediaCarrierOf } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { reachesUnderLoader, takeMediaCarriers } from "./client-reaches.ts";
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

  it("takes the loader's word on whether its media go over WebRTC, and HTTP from an older loader (the efficiency audit, finding 1)", () => {
    let inUse: "direct" | "webrtc" = "webrtc";
    const reaches = reachesUnderLoader({ ...loaderWith(), pathInUse: () => inUse }, document);
    expect(reaches.pathMoves.isOverWebrtc()).toBe(true);
    inUse = "direct";
    expect(reaches.pathMoves.isOverWebrtc()).toBe(false);
    expect(reachesUnderLoader(loaderWith(), document).pathMoves.isOverWebrtc()).toBe(false);
  });

  it("names what carries a media load by the loader's word, and its own from an older loader (the Fire TV on staging, 2026-10-29)", () => {
    const chunk = "https://client.example/api/v1/bytes/media/chunks/t/00001.m4s";
    takeMediaCarriers(loaderWith());
    expect(mediaCarrierOf(chunk)).toBe("http");
    takeMediaCarriers({
      ...loaderWith(),
      mediaCarrierOf: (url) =>
        url === chunk ? "WebRTC connection 2, up 41 s, 3 under way, last bytes 5512 ms ago" : "",
    });
    expect(mediaCarrierOf(chunk)).toBe(
      "WebRTC connection 2, up 41 s, 3 under way, last bytes 5512 ms ago",
    );
  });

  it("hears the way to the core back by the loader's word, and the browser back online from an older loader (VQ130)", () => {
    let told = 0;
    const hear = (): void => {
      told += 1;
    };
    const backListeners: (() => void)[] = [];
    const reaches = reachesUnderLoader(
      {
        ...loaderWith(),
        onPathBack: (listener) => {
          backListeners.push(listener);
          return () => undefined;
        },
      },
      document,
    );
    reaches.onPathBack(hear);
    for (const listener of backListeners) {
      listener();
    }
    expect(told).toBe(1);
    const stop = reachesUnderLoader(loaderWith(), document).onPathBack(hear);
    window.dispatchEvent(new Event("online"));
    expect(told).toBe(2);
    stop();
    window.dispatchEvent(new Event("online"));
    expect(told).toBe(2);
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
