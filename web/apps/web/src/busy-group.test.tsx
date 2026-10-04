// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, expect, it } from "vitest";

import { BusyGroup } from "./busy-group.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/**
 * While a request runs, the controls it holds sit in a disabled fieldset, which a browser disables,
 * every one, so a second click sends nothing (D244 §4). happy-dom does not carry a fieldset's
 * disabling to what it holds, so the test reads the fieldset's own state.
 */
it("holds its controls in a fieldset disabled while a request runs (D244 §4)", async () => {
  const controls = <button type="button">Cancel</button>;
  mounted = await mount(<BusyGroup isBusy>{controls}</BusyGroup>);
  const busy = mounted.container.querySelector("fieldset");
  expect(busy?.disabled).toBe(true);
  expect(busy?.querySelector("button")).not.toBeNull();
  mounted.unmount();
  mounted = await mount(<BusyGroup isBusy={false}>{controls}</BusyGroup>);
  expect(mounted.container.querySelector("fieldset")?.disabled).toBe(false);
});
