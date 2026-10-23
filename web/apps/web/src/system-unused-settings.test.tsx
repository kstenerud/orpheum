// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { SystemUnusedSettings } from "./system-unused-settings.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const retired = {
  key: "resume_min_percent",
  value: 12,
  retiredIn: "v0.3.0",
  retiredReason: "renamed in_progress_after_percent",
};
const typo = { key: "reader_limt", value: 3 };

it("lists the settings this release does not read with why, and deletes one only once confirmed (D202 §2)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /config/unused": [
        [200, { unusedSettings: [retired, typo] }],
        [200, { unusedSettings: [typo] }],
      ],
      "DELETE /config/unused/resume_min_percent": [204, undefined],
    },
    heard,
  );
  mounted = await mount(withServices(<SystemUnusedSettings />, services));
  const container = mounted.container;
  expect(container.textContent).toContain(
    "resume_min_percent = 12: retired in v0.3.0, renamed in_progress_after_percent",
  );
  expect(container.textContent).toContain(
    "reader_limt = 3: no release has read it, so probably a typo",
  );
  await click(container.querySelector('button[aria-label="Delete resume_min_percent"]'));
  expect(heard.some((each) => each.methodAndPath.startsWith("DELETE"))).toBe(false);
  await click(buttonWithText(container, "Delete it"));
  expect(
    heard
      .filter((each) => each.methodAndPath.startsWith("DELETE"))
      .map((each) => each.methodAndPath),
  ).toEqual(["DELETE /config/unused/resume_min_percent"]);
  expect(container.textContent).not.toContain("resume_min_percent");
  expect(container.textContent).toContain("reader_limt");
});

it("shows nothing while every setting is one this release reads", async () => {
  const services = servedBy({ "GET /config/unused": [200, { unusedSettings: [] }] }, []);
  mounted = await mount(withServices(<SystemUnusedSettings />, services));
  expect(mounted.container.textContent).toBe("");
});
