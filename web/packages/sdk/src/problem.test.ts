// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { reportFailure } from "./problem.ts";

describe("reportFailure", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("writes the failure's cause itself to the console, so what the words left out can be read", () => {
    const written = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const cause = new SyntaxError("JSON.parse: unexpected end of data");
    reportFailure(cause);
    expect(written).toHaveBeenCalledOnce();
    expect(written.mock.calls[0]).toContain(cause);
  });
});
