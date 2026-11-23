// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { LibraryConfigure } from "./library-configure.tsx";
import { MeContext } from "./signed-in-me.ts";
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
    checkEveryHours: 6,
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

/** Who is signed in, carrying the person's choices (D480 §2). */
const signedIn = {
  id: 1,
  name: "owner",
  role: "user",
  isAdmin: false,
  canManageAccount: true,
  canHostWatchTogether: false,
  hasNoLibrary: false,
  showsDebugTools: false,
  preferences,
} as const;

/**
 * A library's Configure, opened, over a stand-in server that hears what is sent; an admin's if isAdmin.
 * Drawn alone it reads who is signed in itself; within the shell's reading (isInShell), it reads nothing.
 */
async function configured(
  of: LibraryBody,
  heard: Heard[] = [],
  {
    isAdmin = false,
    isInShell = false,
  }: { readonly isAdmin?: boolean; readonly isInShell?: boolean } = {},
): Promise<HTMLElement> {
  const configure = (
    <IsAdminContext value={isAdmin}>
      <LibraryConfigure library={of} />
    </IsAdminContext>
  );
  const me = { state: "loaded", value: signedIn } as const;
  mounted = await mount(
    withServices(
      isInShell ? (
        <MeContext value={{ me, readMeAgain: () => undefined }}>{configure}</MeContext>
      ) : (
        configure
      ),
      servedBy(
        {
          "GET /me": [200, signedIn],
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
    const page = await configured(library(1, "Films", "film"), heard);
    expect(page.querySelector("dialog")?.getAttribute("aria-label")).toBe("Films");
    expect(choices(page)).toEqual(["Show in Films:true", "Show on Home:true"]);
    const [films] = page.querySelectorAll<HTMLInputElement>('dialog input[type="checkbox"]');
    await click(films);
    await settleUntil(() => puts(heard).length > 0);
    expect(puts(heard)).toEqual([{ leftOutLibraryIds: [2, 1] }]);
  });

  it("shows a library left out of Films unchecked, and puts it back in when checked", async () => {
    const heard: Heard[] = [];
    const page = await configured(library(2, "Kids' films", "film"), heard);
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
    const page = await configured(library(1, "Films", "film"), heard);
    expect(choices(page)).toContain("Show on Home:true");
    const boxes = page.querySelectorAll<HTMLInputElement>('dialog input[type="checkbox"]');
    await click(boxes[1]);
    await settleUntil(() => puts(heard).length > 0);
    expect(puts(heard)).toEqual([{ leftOutOfHomeLibraryIds: [3, 1] }]);
  });

  it("leads an admin to the library's own page, to manage it, and offers no one else the way (the owner, 2026-11-08)", async () => {
    const admins = await configured(library(7, "Films", "film"), [], { isAdmin: true });
    const manage = admins.querySelector<HTMLAnchorElement>("dialog a");
    expect(manage?.textContent).toBe("Manage this library");
    expect(manage?.getAttribute("href")).toBe("/libraries/7");
    mounted?.unmount();
    const someones = await configured(library(7, "Films", "film"));
    expect(someones.querySelector("dialog a")).toBeNull();
    expect(someones.textContent).not.toContain("Manage this library");
  });

  it("takes the person's choices from who the shell read as signed in, asking for nothing on opening (D480 §2)", async () => {
    const heard: Heard[] = [];
    const page = await configured(library(2, "Kids' films", "film"), heard, { isInShell: true });
    expect(choices(page)).toEqual(["Show in Films:false", "Show on Home:true"]);
    expect(heard).toEqual([]);
  });

  it("drawn alone, takes them from GET /me, and reads no choices of its own", async () => {
    const heard: Heard[] = [];
    await configured(library(1, "Films", "film"), heard);
    expect(heard.map((each) => each.methodAndPath)).not.toContain("GET /me/preferences");
  });

  it("closes on a remote's Back, and as Escape closes it, the dialog having no cross (D538 §2)", async () => {
    const page = await configured(library(1, "Films", "film"));
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
    await fire(page.querySelector("dialog"), new Event("cancel", { cancelable: true }));
    expect(page.querySelector("dialog")).toBeNull();
  });
});
