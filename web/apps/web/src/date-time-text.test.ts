// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { clockText, dateText, dateTimeText, timeText } from "./date-time-text.ts";

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

describe("clockText, a film's clock (the code audit of 2026-11-21, finding 9)", () => {
  it("says m:ss under an hour and h:mm:ss from one", () => {
    expect(clockText(2_472_000, "en")).toBe("41:12");
    expect(clockText(3_723_000, "en")).toBe("1:02:03");
    expect(clockText(-5, "en")).toBe("0:00");
  });

  it("says it in the locale's digits", () => {
    expect(clockText(3_723_000, "ar-EG")).toBe("١:٠٢:٠٣");
    expect(clockText(2_472_000, "ar-EG")).toBe("٤١:١٢");
  });
});
