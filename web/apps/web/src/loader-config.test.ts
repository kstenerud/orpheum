// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { clientOriginOf, coreIdOfOrigin, loaderConfigOf } from "./loader-config.ts";

/** A page whose head holds meta elements of these names and contents. */
function pageWith(metas: Record<string, string>): Document {
  const page = document.implementation.createHTMLDocument("loader");
  for (const [name, content] of Object.entries(metas)) {
    const meta = page.createElement("meta");
    meta.name = name;
    meta.content = content;
    page.head.append(meta);
  }
  return page;
}

const discoveryPage = {
  "orpheum-discovery-client-api": "https://api.example/client/v1",
  "orpheum-client-origins": "https://{core}.client.example",
  "orpheum-loader-origin": "https://app.example",
};

describe("what a discovery's page tells its loader (D308)", () => {
  it("is the client API, the client origins' pattern and the loader's origin, all three or nothing", () => {
    expect(loaderConfigOf(pageWith(discoveryPage))).toEqual({
      clientApi: "https://api.example/client/v1",
      hasRealms: false,
      clientOrigins: "https://{core}.client.example",
      loaderOrigin: "https://app.example",
    });
    for (const missing of Object.keys(discoveryPage)) {
      const metas: Record<string, string> = { ...discoveryPage };
      Reflect.deleteProperty(metas, missing);
      expect(loaderConfigOf(pageWith(metas))).toBeUndefined();
    }
    expect(
      loaderConfigOf(
        pageWith({ ...discoveryPage, "orpheum-client-origins": "https://client.example" }),
      ),
    ).toBeUndefined();
    // The core's own page names no discovery: no loader runs there.
    expect(loaderConfigOf(pageWith({}))).toBeUndefined();
  });

  it("reads a core's id from its client origin alone, and builds that origin from the id alone", () => {
    const pattern = "https://{core}-staging.client.example";
    expect(coreIdOfOrigin("https://k7xq2-staging.client.example", pattern)).toBe("k7xq2");
    for (const origin of [
      "https://app.example",
      "https://k7xq2.client.example",
      "https://evil.example/k7xq2-staging.client.example",
      "https://a.b-staging.client.example",
      "https://-staging.client.example",
    ]) {
      expect(coreIdOfOrigin(origin, pattern)).toBeUndefined();
    }
    expect(clientOriginOf("k7xq2", pattern)).toBe("https://k7xq2-staging.client.example");
    // Never a URL a link carries: an id that is not one sends the browser nowhere.
    for (const coreId of ["evil.example/", "a.b", "", "K7XQ2"]) {
      expect(clientOriginOf(coreId, pattern)).toBeUndefined();
    }
  });
});
