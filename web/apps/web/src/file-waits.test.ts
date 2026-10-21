// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { preparingText } from "./film-player.tsx";
import { waitText } from "./file-waits.ts";
import { i18n } from "./i18n.ts";

describe("how long work will take, in words (D324 §6)", () => {
  const t = i18n.t.bind(i18n);

  it("says under a minute, a minute, minutes, and hours from an hour and a half", () => {
    expect(waitText(t, 20)).toBe("less than a minute");
    expect(waitText(t, 70)).toBe("about a minute");
    expect(waitText(t, 180)).toBe("about 3 minutes");
    expect(waitText(t, 89 * 60)).toBe("about 89 minutes");
    expect(waitText(t, 2 * 3600)).toBe("about 2 hours");
  });

  it("tells a viewer waiting for a prepared copy how long it will take, where the server says", () => {
    expect(preparingText(t, { isRunning: true, secondsLeft: 120 })).toBe(
      "Preparing this film for this device: about 2 minutes.",
    );
    expect(preparingText(t, { isRunning: false })).toBe("Preparing — ready in a few seconds.");
    expect(preparingText(t, undefined)).toBe("Preparing — ready in a few seconds.");
  });
});
