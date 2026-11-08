// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { endCast, type CastEnd } from "./cast.ts";
import { servedBy, type Heard } from "./test-dom.tsx";

describe("ending a cast (D446)", () => {
  it("sends the reason the server knows, and no receiver's take-over, which it refuses (D446 §4)", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy({ "POST /me/casts/5/end": [200, { positionMs: 0 }] }, heard, "token");
    // @ts-expect-error a receiver takes no cast over, so movedHere is no reason to end one (D446 §4)
    const takenOver: CastEnd = "movedHere";
    const reasons: CastEnd[] = ["takenBack", "stopped"];
    for (const reason of reasons) {
      await endCast(api, 5, reason);
    }
    expect(heard.map((each) => each.body)).toEqual([
      { reason: "takenBack" },
      { reason: "stopped" },
    ]);
    expect(takenOver).toBe("movedHere");
  });
});
