// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { Settings } from "./settings.tsx";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const retired = {
  key: "resumeMinPercent",
  value: 12,
  retiredIn: "v0.3.0",
  retiredReason: "renamed inProgressAfterPercent",
};
const typo = { key: "reader_limt", value: 3 };

/** The index's other reads, answered with nothing to say. */
const indexAnswers: Answers = {
  "GET /config": [200, { settings: [] }],
  "GET /secrets": [200, { secrets: [] }],
  "GET /needs-attention": [200, { items: [] }],
};

it("says on Settings' index how many settings this release does not read, its line opening their fold (pages-spec-3)", async () => {
  const services = servedBy({
    ...indexAnswers,
    "GET /config/unused": [200, { unusedSettings: [retired, typo] }],
  });
  mounted = await mount(withServices(<Settings />, services));
  const container = mounted.container;
  await settleUntil(() => container.querySelector("#settings-unread") !== null);
  const line = container.querySelector<HTMLAnchorElement>('a[href="#settings-unread"]');
  expect(line?.textContent).toBe("2 settings are not read by this release.");
  const fold = container.querySelector<HTMLDetailsElement>("details#settings-unread");
  expect(fold?.open).toBe(false);
  await click(line);
  expect(fold?.open).toBe(true);
  expect(document.activeElement).toBe(fold?.querySelector("summary"));
});

it("lists the settings this release does not read with why, and deletes one only once confirmed (D202 §2)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      ...indexAnswers,
      "GET /config/unused": [
        [200, { unusedSettings: [retired, typo] }],
        [200, { unusedSettings: [typo] }],
      ],
      "DELETE /config/unused/resumeMinPercent": [204, undefined],
    },
    heard,
  );
  mounted = await mount(withServices(<Settings />, services));
  const container = mounted.container;
  await settleUntil(() => container.querySelector("#settings-unread") !== null);
  expect(container.textContent).toContain(
    "resumeMinPercent = 12: retired in v0.3.0, renamed inProgressAfterPercent",
  );
  expect(container.textContent).toContain(
    "reader_limt = 3: no release has read it, so probably a typo",
  );
  await click(container.querySelector('button[aria-label="Delete resumeMinPercent"]'));
  expect(heard.some((each) => each.methodAndPath.startsWith("DELETE"))).toBe(false);
  await click(buttonWithText(container, "Delete it"));
  expect(
    heard
      .filter((each) => each.methodAndPath.startsWith("DELETE"))
      .map((each) => each.methodAndPath),
  ).toEqual(["DELETE /config/unused/resumeMinPercent"]);
  expect(container.textContent).not.toContain("resumeMinPercent");
  expect(container.textContent).toContain("1 setting is not read by this release.");
});

it("says nothing of them while every setting is one this release reads", async () => {
  const services = servedBy({
    ...indexAnswers,
    "GET /config/unused": [200, { unusedSettings: [] }],
  });
  mounted = await mount(withServices(<Settings />, services));
  const container = mounted.container;
  await settleUntil(() => container.querySelector(".settings-index") !== null);
  expect(container.querySelector("#settings-unread")).toBeNull();
  expect(container.textContent).not.toContain("Needs you");
});
