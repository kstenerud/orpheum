// SPDX-License-Identifier: MPL-2.0

import type { TestInfo } from "@playwright/test";
import { describe, expect, it } from "vitest";

import { builtScriptOf, readsStylesheets, record } from "./browser-map.ts";

const passed = (tags: string[]): TestInfo => ({ status: "passed", tags }) as unknown as TestInfo;

describe("a test's record in the browser map", () => {
  it("is what its pages held, or nothing (reaching every change) for one that reads the stylesheets", () => {
    const held = new Set(["poster", "div"]);
    expect(record(held, true, passed([]))).toEqual(["div", "poster"]);
    expect(record(held, true, passed([readsStylesheets]))).toBeNull();
    expect(record(held, false, passed([]))).toBeNull();
  });
});

describe("a script a test loaded", () => {
  it("is its file in the build, from any origin, and only a script is one", () => {
    expect(builtScriptOf("https://abc.orpheum-client.com/assets/loader-main-x.js?client")).toBe(
      "assets/loader-main-x.js",
    );
    expect(builtScriptOf("http://127.0.0.1:4000/sw.js")).toBe("sw.js");
    expect(builtScriptOf("http://127.0.0.1:4000/assets/client-main-x.css")).toBeUndefined();
    expect(builtScriptOf("http://127.0.0.1:4000/api/v1/health")).toBeUndefined();
  });
});
