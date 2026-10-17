// SPDX-License-Identifier: MPL-2.0

import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";

import { StillReadingIcon } from "./still-reading.tsx";

it("draws an hourglass with its caps, its glass and sand run into the lower bulb (the owner, 2026-10-17)", () => {
  // The thin outline alone was not recognised as an hourglass at a poster's corner.
  for (const isRinged of [true, false]) {
    const markup = renderToStaticMarkup(<StillReadingIcon isRinged={isRinged} />);
    for (const part of ["caps", "glass", "sand"]) {
      expect(markup).toContain(`still-reading-icon__${part}`);
    }
    expect(markup.includes("still-reading-icon__ring")).toBe(isRinged);
  }
});
