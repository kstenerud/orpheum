// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { Libraries } from "./libraries.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

it("the libraries screen says whether each machine runs FFmpeg in its sandbox (D112)", async () => {
  const services = servedBy({
    "GET /libraries": [200, { libraries: [] }],
    "GET /nodes": [
      200,
      {
        nodes: [
          {
            name: "local",
            isConnected: true,
            roots: ["/media"],
            confinement: [{ mechanism: "seccomp", isInForce: false, cause: "kernel_without" }],
          },
        ],
      },
    ],
  });
  mounted = await mount(withServices(<Libraries />, services));
  expect(mounted.container.textContent).toContain("local: FFmpeg runs without part of its sandbox");
  expect(mounted.container.textContent).toContain(
    "seccomp (which system calls FFmpeg may make) is not in this machine's kernel",
  );
});
