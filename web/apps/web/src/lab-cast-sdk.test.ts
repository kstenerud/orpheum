// SPDX-License-Identifier: MPL-2.0

import { failureText } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { castTo } from "./lab-cast-sdk.ts";

/** Puts a Cast context whose requestSession answers as requested on the page's globals. */
function givenCastContext(requestSession: () => Promise<unknown>): void {
  const castContext = { setOptions: () => undefined, requestSession };
  Reflect.set(globalThis, "cast", {
    // eslint-disable-next-line @typescript-eslint/naming-convention -- the Cast SDK's own name
    framework: { CastContext: { getInstance: () => castContext } },
  });
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, "cast");
});

describe("the lab's cast to a receiver", () => {
  it("says which error code the SDK refused the session with, not only that it failed (2026-11-11)", async () => {
    // As Google's SDK does: the promise is rejected with the bare code.
    givenCastContext(
      // eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors -- the SDK's bare code is what is tested
      async () => await Promise.reject("receiver_unavailable"),
    );
    const failure: unknown = await castTo("ABCD1234").catch((cause: unknown) => cause);
    expect(failureText(failure)).toContain("receiver_unavailable");
  });
});
