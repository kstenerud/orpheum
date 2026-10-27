// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { commandOnLeavingFor } from "./together-host.tsx";

describe("the host leaving their player (D246 §5, D339 §3)", () => {
  it("stops the film for everyone when the host leaves for anything but another film's player", () => {
    expect(commandOnLeavingFor({ screen: "home" }, 7)).toEqual({ command: "stop" });
    expect(commandOnLeavingFor({ screen: "play", fileId: 7 }, 7)).toEqual({ command: "stop" });
  });

  it("sends nothing when the host moves on to the next episode's player, so the group follows", () => {
    expect(commandOnLeavingFor({ screen: "play", fileId: 8 }, 7)).toBeUndefined();
  });
});
