// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import type { Library } from "./libraries.tsx";
import { ChangeDetectionChoice, type ChangeDetection } from "./library-manage.tsx";
import { chooseIn, enter, fire, mount, settleAnswers, submit, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const films: Library = {
  id: 3,
  name: "Films",
  kind: "film",
  metadataSource: "nfo",
  changeDetection: "manual",
  checkEveryHours: 6,
  isWritable: false,
  createdAt: 1,
};

// The owner, 2026-11-04, Q5: every setting saves at once, so no Save.
it("saves how a library notices changes as it is chosen, always with its hours, from 1 to a week, and says Saved (2026-10-09, D541)", async () => {
  const saved: ChangeDetection[] = [];
  mounted = await mount(
    <ChangeDetectionChoice
      library={films}
      onSave={async (choice) => {
        saved.push(choice);
        await Promise.resolve();
      }}
    />,
  );
  const { container } = mounted;
  expect(container.querySelector("button")).toBeNull();
  expect(container.querySelector("select")).toBeNull();
  expect(container.querySelector("input[type=number]")).toBeNull();
  expect(container.textContent).not.toContain("Saved");

  // An interval is sent at once with the hours the library kept; new hours on leaving their field.
  await chooseIn(container, "How changes are noticed", "Check every so many hours");
  await settleAnswers();
  expect(container.querySelector("output.status-line--success")?.textContent).toBe("Saved");
  const hours = container.querySelector<HTMLInputElement>("input[type=number]");
  expect(hours?.max).toBe("168");
  await enter(hours, "200");
  expect(saved).toHaveLength(1);
  expect(hours?.value).toBe("168");
  // An emptied field keeps the hours it held.
  await enter(hours, "");
  expect(hours?.value).toBe("168");
  await fire(hours, new FocusEvent("focusout", { bubbles: true }));
  await settleAnswers();
  // Enter on the same hours sends nothing again.
  await submit(container.querySelector("form"));
  await chooseIn(container, "How changes are noticed", "Watch the folders");
  expect(container.querySelector("input[type=number]")).toBeNull();
  expect(saved).toEqual([
    { changeDetection: "interval", checkEveryHours: 6 },
    { changeDetection: "interval", checkEveryHours: 168 },
    { changeDetection: "watch", checkEveryHours: 168 },
  ]);
});

it("says in place why a way of noticing changes was refused", async () => {
  mounted = await mount(
    <ChangeDetectionChoice
      library={films}
      onSave={async () => {
        await Promise.reject(new Error("The node cannot watch."));
      }}
    />,
  );
  await chooseIn(mounted.container, "How changes are noticed", "Watch the folders");
  await settleAnswers();
  expect(mounted.container.querySelector('[role="alert"]')?.textContent).not.toBe("");
  expect(mounted.container.textContent).not.toContain("Saved");
});
