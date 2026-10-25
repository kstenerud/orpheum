// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { LabInput } from "./lab-input.tsx";
import { buttonWithText, mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

describe("the input probe", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("moves into its tiles by our navigation on the first arrow, and starts its tone with it", async () => {
    mounted = await mount(withServices(<LabInput />));
    const tone = buttonWithText(mounted.container, "Play a quiet tone");
    if (tone === undefined) {
      throw new Error("the probe has no tone button");
    }
    expect(tone.getAttribute("aria-pressed")).toBe("false");
    act(() => {
      document.body.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true, cancelable: true }),
      );
    });
    expect(document.activeElement?.classList.contains("lab__tile")).toBe(true);
    expect(tone.getAttribute("aria-pressed")).toBe("true");
  });
});
