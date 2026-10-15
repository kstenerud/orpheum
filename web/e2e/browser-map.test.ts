// SPDX-License-Identifier: MPL-2.0

import type { TestInfo } from "@playwright/test";
import { describe, expect, it } from "vitest";

import { readsStylesheets, record } from "./browser-map.ts";

const passed = (tags: string[]): TestInfo => ({ status: "passed", tags }) as unknown as TestInfo;

describe("a test's record in the browser map", () => {
  it("is what its pages held, or nothing (reaching every change) for one that reads the stylesheets", () => {
    const held = new Set(["poster", "div"]);
    expect(record(held, true, passed([]))).toEqual(["div", "poster"]);
    expect(record(held, true, passed([readsStylesheets]))).toBeNull();
    expect(record(held, false, passed([]))).toBeNull();
  });
});
