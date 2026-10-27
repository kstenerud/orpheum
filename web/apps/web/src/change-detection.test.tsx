// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import type { Library } from "./libraries.tsx";
import { ChangeDetectionChoice, type ChangeDetection } from "./library-manage.tsx";
import { choose, enter, mount, submit, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const films: Library = {
  id: 3,
  name: "Films",
  kind: "movies",
  metadataSource: "nfo",
  changeDetection: "manual",
  isWritable: false,
  createdAt: 1,
};

it("chooses again how a library notices changes, hours only with an interval (2026-10-09)", async () => {
  const saved: ChangeDetection[] = [];
  mounted = await mount(
    <ChangeDetectionChoice
      library={films}
      onSave={(choice) => {
        saved.push(choice);
      }}
    />,
  );
  const { container } = mounted;
  const save = (): HTMLButtonElement | null => container.querySelector("button[type=submit]");
  expect(save()?.disabled).toBe(true);
  expect(container.querySelector("input[type=number]")).toBeNull();

  await choose(container.querySelector("select"), "interval");
  await enter(container.querySelector("input[type=number]"), "6");
  expect(save()?.disabled).toBe(false);
  await submit(container.querySelector("form"));
  await choose(container.querySelector("select"), "watch");
  expect(container.querySelector("input[type=number]")).toBeNull();
  await submit(container.querySelector("form"));
  expect(saved).toEqual([
    { changeDetection: "interval", checkEveryHours: 6 },
    { changeDetection: "watch" },
  ]);
});
