// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { SystemBackups } from "./system-backups.tsx";
import {
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const made = {
  name: "backup-20261003T120000Z.tar.gz",
  sizeBytes: 2_500_000,
  madeAt: 1_790_000_000_000,
};

it("asks for a backup now, the key only when asked, and lists the backup once made (D242 §5, D243 §11)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /backups": [
        [200, { directory: "/srv/orpheum/backups", backups: [] }],
        [200, { directory: "/srv/orpheum/backups", backups: [made] }],
      ],
      "POST /backups": [202, { includesKey: false }],
    },
    heard,
  );
  mounted = await mount(withServices(<SystemBackups />, services));
  const container = mounted.container;
  expect(container.textContent).toContain("There is no backup yet.");
  expect(container.textContent).toContain("/srv/orpheum/backups");
  await click(buttonWithText(container, "Back up now"));
  expect(heard.filter((each) => each.methodAndPath === "POST /backups")).toEqual([
    { methodAndPath: "POST /backups", body: { includesKey: false } },
  ]);
  expect(container.textContent).toContain("A backup is on its way");
  expect(container.textContent).toContain("backup-20261003T120000Z.tar.gz, 2.5 MB");
});

it("puts Back up now in the heading's row and folds the list of backups, closed (ui-audit-2026-11-04.md §3)", async () => {
  const services = servedBy({
    "GET /backups": [200, { directory: "/srv/orpheum/backups", backups: [made] }],
  });
  mounted = await mount(withServices(<SystemBackups />, services));
  const container = mounted.container;
  const row = container.querySelector("#system-backups")?.parentElement;
  expect(row?.contains(buttonWithText(container, "Back up now") ?? null)).toBe(true);
  expect(row?.querySelector("p, label")).toBeNull();
  const fold = container.querySelector("details");
  expect(fold?.open).toBe(false);
  expect(fold?.querySelector("summary")?.textContent).toBe("The backups kept (1)");
  expect(fold?.textContent).toContain("backup-20261003T120000Z.tar.gz");
});

it("warns what a backup with the key holds, and says why one was refused", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /backups": [200, { directory: "/srv/orpheum/backups", backups: [] }],
      "POST /backups": refusal(409, "conflict"),
    },
    heard,
  );
  mounted = await mount(withServices(<SystemBackups />, services));
  const container = mounted.container;
  await click(container.querySelector('input[type="checkbox"]'));
  expect(container.textContent).toContain("Whoever has a backup with the key");
  await click(buttonWithText(container, "Back up now"));
  expect(heard.filter((each) => each.methodAndPath === "POST /backups")).toEqual([
    { methodAndPath: "POST /backups", body: { includesKey: true } },
  ]);
  expect(container.querySelector('[role="alert"]')?.textContent ?? "").not.toBe("");
});
