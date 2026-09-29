// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { dateTimeText } from "./date-time-text.ts";

describe("dateTimeText", () => {
  it("says a moment's date and time in the viewer's language", () => {
    const moment = Date.UTC(2026, 8, 29, 14, 5);
    const expected = new Intl.DateTimeFormat("en-GB", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(moment));
    expect(dateTimeText(moment, "en-GB")).toBe(expected);
    expect(dateTimeText(moment, "en-GB")).toContain("2026");
  });
});
