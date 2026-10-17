// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { chunkModules } from "./chunk-modules.ts";

describe("chunkModules", () => {
  it("lists each chunk's own files from the repository's root, without a query, and no asset", () => {
    expect(
      chunkModules(
        {
          "assets/loader-main-x.js": {
            type: "chunk",
            moduleIds: [
              "/repo/web/apps/web/src/loader-main.tsx",
              "/repo/web/apps/web/src/client-main.tsx?client",
              "/repo/web/node_modules/react/index.js",
              "\0vite/preload-helper.js",
              "/repo/web/packages/sdk/src/discovery.ts",
              "/repo/web/apps/web/src/loader-main.tsx",
            ],
          },
          "assets/logo-y.svg": { type: "asset" },
        },
        "/repo",
      ),
    ).toEqual({
      "assets/loader-main-x.js": [
        "web/apps/web/src/client-main.tsx",
        "web/apps/web/src/loader-main.tsx",
        "web/packages/sdk/src/discovery.ts",
      ],
    });
  });
});
