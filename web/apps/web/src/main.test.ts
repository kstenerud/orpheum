// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("./client-main.tsx", () => {
  throw new Error("Unable to preload CSS for /assets/busy-group-CtCniuzz.css");
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("main.tsx", () => {
  it("reloads the page once when the client's code does not come, asked twice, never leaving it black", async () => {
    vi.useFakeTimers();
    const reload = vi.fn();
    vi.spyOn(window, "location", "get").mockReturnValue({ ...window.location, reload });

    await import("./main.tsx");
    await vi.advanceTimersByTimeAsync(5000);

    expect(reload).toHaveBeenCalledTimes(1);
  });
});
