// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { dateText, dateTimeText, timeText } from "./date-time-text.ts";

describe("dateTimeText", () => {
  const moment = Date.UTC(2026, 8, 29, 14, 5);

  it("says a moment's date and time in the viewer's language", () => {
    const expected = new Intl.DateTimeFormat("en-GB", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(moment));
    expect(dateTimeText(moment, "en-GB")).toBe(expected);
    expect(dateTimeText(moment, "en-GB")).toContain("2026");
  });

  it("says the date alone and the time alone as its two parts", () => {
    const date = dateText(moment, "en-GB");
    const time = timeText(moment, "en-GB");
    expect(date).toContain("2026");
    expect(time).not.toContain("2026");
    expect(dateTimeText(moment, "en-GB")).toContain(date);
    expect(dateTimeText(moment, "en-GB")).toContain(time);
  });
});
