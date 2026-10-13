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
