// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { LibraryConfigure } from "./library-configure.tsx";
import {
  click,
  fire,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

type LibraryBody = components["schemas"]["LibraryBody"];

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A library of kind, its other fields as any. */
function library(id: number, name: string, kind: LibraryBody["kind"]): LibraryBody {
  return {
    id,
    name,
    kind,
    metadataSource: "tmdb",
    changeDetection: "watch",
    isWritable: true,
    createdAt: 0,
  };
}

/** The person's choices: Kids' films (2) left out of Films, Home movies (3) left out of Home. */
const preferences = {
  isDialogueBoostOn: false,
  isLevellingOn: true,
  isAutoplayOn: true,
  leftOutLibraryIds: [2],
  leftOutOfHomeLibraryIds: [3],
};

/** A library's Configure, opened, over a stand-in server that hears what is sent; an admin's if isAdmin. */
async function configured(
  of: LibraryBody,
  heard: Heard[] = [],
  isAdmin = false,
): Promise<HTMLElement> {
  mounted = await mount(
    withServices(
      <IsAdminContext value={isAdmin}>
        <LibraryConfigure library={of} />
      </IsAdminContext>,
      servedBy(
        {
          "GET /me/preferences": [200, preferences],
          "PUT /me/preferences": [204, {}],
        },
        heard,
      ),
    ),
  );
  await click(mounted.container.querySelector<HTMLButtonElement>('button[aria-label="Configure"]'));
  await settleUntil(
    () => mounted?.container.querySelector('dialog input[type="checkbox"]') !== null,
  );
  return mounted.container;
}

/** Each of the dialog's checkboxes, by its label, and whether it is checked. */
function choices(container: HTMLElement): string[] {
  return [...container.querySelectorAll<HTMLInputElement>('dialog input[type="checkbox"]')].map(
    (box) => `${box.closest("label")?.textContent ?? ""}:${String(box.checked)}`,
  );
}

/** The bodies of what was put of the person's choices. */
function puts(heard: readonly Heard[]): unknown[] {
  return heard
    .filter((request) => request.methodAndPath === "PUT /me/preferences")
    .map((request) => request.body);
}

describe("a library's Configure (the owner, 2026-10-27; D349 §8, D366 §5)", () => {
  it("offers a films library to Films, checked until left out, and leaves one out when unchecked", async () => {
    const heard: Heard[] = [];
    const page = await configured(library(1, "Films", "movie"), heard);
    expect(page.querySelector("dialog")?.getAttribute("aria-label")).toBe("Films");
    expect(choices(page)).toEqual(["Show in Films:true", "Show on Home:true"]);
    const [films] = page.querySelectorAll<HTMLInputElement>('dialog input[type="checkbox"]');
    await click(films);
    await settleUntil(() => puts(heard).length > 0);
    expect(puts(heard)).toEqual([{ leftOutLibraryIds: [2, 1] }]);
  });

  it("shows a library left out of Films unchecked, and puts it back in when checked", async () => {
    const heard: Heard[] = [];
    const page = await configured(library(2, "Kids' films", "movie"), heard);
    expect(choices(page)).toEqual(["Show in Films:false", "Show on Home:true"]);
    const [films] = page.querySelectorAll<HTMLInputElement>('dialog input[type="checkbox"]');
    await click(films);
    await settleUntil(() => puts(heard).length > 0);
    expect(puts(heard)).toEqual([{ leftOutLibraryIds: [] }]);
  });

  it("offers a series library to Series", async () => {
    const page = await configured(library(4, "Shows", "series"));
    expect(choices(page)).toEqual(["Show in Series:true", "Show on Home:true"]);
  });

  it("offers an other library to Home alone, which neither Films nor Series gathers", async () => {
    const heard: Heard[] = [];
    const page = await configured(library(3, "Home movies", "other"), heard);
    expect(choices(page)).toEqual(["Show on Home:false"]);
    expect(page.textContent).not.toContain("Show in");
    const [home] = page.querySelectorAll<HTMLInputElement>('dialog input[type="checkbox"]');
    await click(home);
    await settleUntil(() => puts(heard).length > 0);
    expect(puts(heard)).toEqual([{ leftOutOfHomeLibraryIds: [] }]);
  });

  it("leaves a library out of Home when its Show on Home is unchecked", async () => {
    const heard: Heard[] = [];
    const page = await configured(library(1, "Films", "movie"), heard);
    expect(choices(page)).toContain("Show on Home:true");
    const boxes = page.querySelectorAll<HTMLInputElement>('dialog input[type="checkbox"]');
    await click(boxes[1]);
    await settleUntil(() => puts(heard).length > 0);
    expect(puts(heard)).toEqual([{ leftOutOfHomeLibraryIds: [3, 1] }]);
  });

  it("leads an admin to the library's own page, to manage it, and offers no one else the way (the owner, 2026-11-08)", async () => {
    const admins = await configured(library(7, "Films", "movie"), [], true);
    const manage = admins.querySelector<HTMLAnchorElement>("dialog a");
    expect(manage?.textContent).toBe("Manage this library");
    expect(manage?.getAttribute("href")).toBe("/libraries/7");
    mounted?.unmount();
    const someones = await configured(library(7, "Films", "movie"));
    expect(someones.querySelector("dialog a")).toBeNull();
    expect(someones.textContent).not.toContain("Manage this library");
  });

  it("closes on a remote's Back, and by its Close, the cross every dialog has", async () => {
    const page = await configured(library(1, "Films", "movie"));
    const box = page.querySelector<HTMLInputElement>('dialog input[type="checkbox"]');
    box?.focus();
    const back = new KeyboardEvent("keydown", {
      key: "XF86Back",
      keyCode: 10_009,
      bubbles: true,
      cancelable: true,
    });
    await fire(box, back);
    expect(page.querySelector("dialog")).toBeNull();
    await click(page.querySelector<HTMLButtonElement>('button[aria-label="Configure"]'));
    await settleUntil(() => page.querySelector("dialog") !== null);
    await click(page.querySelector<HTMLButtonElement>("dialog .modal-dialog__close button"));
    expect(page.querySelector("dialog")).toBeNull();
  });
});
