// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { pictureStreamOf } from "./step-foresight.ts";

describe("the stream a quality's playlist address names (D532 §1)", () => {
  it("is the folder its stream.m3u8 is in, past any query, and none for another address", () => {
    expect(pictureStreamOf("q3000k720p/stream.m3u8")).toBe("q3000k720p");
    expect(pictureStreamOf("/api/v1/x/picture/stream.m3u8?t=1#a")).toBe("picture");
    expect(pictureStreamOf("stream.m3u8")).toBeUndefined();
    expect(pictureStreamOf("/stream.m3u8")).toBeUndefined();
    expect(pictureStreamOf("q3000k720p/other.m3u8")).toBeUndefined();
    expect(pictureStreamOf(undefined)).toBeUndefined();
  });
});
