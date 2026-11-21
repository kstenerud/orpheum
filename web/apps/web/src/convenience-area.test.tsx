// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { ConvenienceArea } from "./convenience-area.tsx";
import { i18n } from "./i18n.ts";
import { mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(async () => {
  mounted?.unmount();
  mounted = undefined;
  await i18n.changeLanguage("en");
});

describe("the overlay's convenience area (D407 §1)", () => {
  it("says when the film ends in the person's chosen language, not the browser's (the code audit of 2026-11-21, finding 9)", async () => {
    await i18n.changeLanguage("ar-EG");
    mounted = await mount(
      <ConvenienceArea
        film={{ videoRef: createRef(), readLinkKbps: () => undefined, failure: undefined }}
        offer={undefined}
        hasOffer={false}
        isOverlayFull={true}
        onOffered={() => undefined}
        leftMs={60_000}
        areaRef={createRef()}
      />,
    );
    const endsAt = mounted.container.querySelector(".convenience-area__ends")?.textContent;
    expect(endsAt).toContain("Ends at");
    expect(endsAt).toMatch(/[٠-٩]/u); // the chosen language's digits, where the browser's are 0-9
  });
});
