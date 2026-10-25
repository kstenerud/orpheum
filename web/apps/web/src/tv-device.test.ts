// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { isTelevision } from "./tv-device.ts";

describe("a television's browser, by its user agent", () => {
  it("knows the owner's Fire TV stick in Silk, and televisions of other makes", () => {
    expect(
      isTelevision(
        "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36",
      ),
    ).toBe(true);
    expect(
      isTelevision(
        "Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/5.0 Chrome/94.0.4606.31 TV Safari/537.36",
      ),
    ).toBe(true);
    expect(
      isTelevision(
        "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36 WebAppManager",
      ),
    ).toBe(true);
  });

  it("takes a computer's and a phone's browser for no television", () => {
    expect(
      isTelevision("Mozilla/5.0 (X11; Linux x86_64; rv:158.0) Gecko/20100101 Firefox/158.0"),
    ).toBe(false);
    expect(
      isTelevision(
        "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
      ),
    ).toBe(false);
    expect(
      isTelevision(
        "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36",
      ),
    ).toBe(false);
  });
});
