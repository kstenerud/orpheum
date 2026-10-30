// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { MoreActions } from "./more-actions.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("a page's menu of rarely used actions", () => {
  it("shows nothing where the page has no rarely used action", async () => {
    mounted = await mount(withServices(<MoreActions actions={[]} />));
    expect(mounted.container.innerHTML).toBe("");
  });
});
