// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { deviceFrom, thisDevice } from "./device.ts";

describe("naming this device", () => {
  it("names the browser and the system, the more specific first", () => {
    const named = (userAgent: string): string => deviceFrom(userAgent).name;
    expect(
      named(
        "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0 Mobile Safari/537.36",
      ),
    ).toBe("Chrome on Android");
    expect(
      named(
        "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
      ),
    ).toBe("Safari on iPhone");
    expect(
      named(
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0 Safari/537.36 Edg/129.0",
      ),
    ).toBe("Edge on Windows");
    expect(named("Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0")).toBe(
      "Firefox on Linux",
    );
    expect(named("something else")).toBe("A web browser");
  });

  it("names a Fire TV's Silk, whose agent says Android and like Chrome, by its model code", () => {
    // Staging's device 33, a Fire TV Stick, was named "Chrome on Android" (the owner, 2026-10-27).
    const named = (userAgent: string): string => deviceFrom(userAgent).name;
    expect(
      named(
        "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36",
      ),
    ).toBe("Silk on Fire TV");
    expect(
      named(
        "Mozilla/5.0 (Linux; Android 7.1.2; AFTKA) AppleWebKit/537.36 (KHTML, like Gecko) Silk/131.4.1 like Chrome/131.0.6778.260 Safari/537.36",
      ),
    ).toBe("Silk on Fire TV");
  });

  it("says it is a web client", () => {
    expect(deviceFrom("").platform).toBe("web");
  });

  it("names the device it was before, with the proof it was it, which the server resumes (D221)", () => {
    expect(thisDevice({ deviceId: () => 7, deviceProof: () => "token" })).toMatchObject({
      previousDeviceId: 7,
      previousDeviceProof: "token",
    });
    expect(
      thisDevice({ deviceId: () => undefined, deviceProof: () => "token" }),
    ).not.toHaveProperty("previousDeviceId");
  });
});
