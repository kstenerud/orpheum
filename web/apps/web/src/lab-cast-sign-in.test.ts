// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { signInOf } from "./lab-cast.tsx";

const session = { token: () => "a-token", locator: () => "a-locator" };
const throughDiscovery = {
  clientApi: "https://api.example/client/v1",
  coreId: "k7xq2",
  fingerprint: "a-fingerprint",
};

describe("what a play sent to the lab's receiver signs in by", () => {
  it("is nothing unless the person ticked it and the page holds a session", () => {
    expect(signInOf(false, session, throughDiscovery)).toEqual({});
    expect(signInOf(true, { token: () => undefined, locator: () => undefined }, undefined)).toEqual(
      {},
    );
  });

  it("is the session alone on the core's own origin", () => {
    expect(signInOf(true, session, undefined)).toEqual({ authorization: "Bearer a-token" });
  });

  it("carries the way to the core from away under a discovery's loader (the 11-05 batch's item 10)", () => {
    expect(signInOf(true, session, throughDiscovery)).toEqual({
      authorization: "Bearer a-token",
      reach: { ...throughDiscovery, token: "a-token", locator: "a-locator" },
    });
  });
});
