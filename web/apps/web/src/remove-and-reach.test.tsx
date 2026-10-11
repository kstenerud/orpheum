// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { BrowseView } from "./browse.tsx";
import { People } from "./people.tsx";
import { PersonPage } from "./person.tsx";
import {
  buttonWithText,
  choose,
  click,
  mount,
  servedBy,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import type { Services } from "./services.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const ignore = (): void => undefined;

const roles = {
  capabilities: ["administer"],
  roles: [
    {
      id: 1,
      name: "admin",
      isBuiltin: true,
      capabilities: ["administer"],
      isEveryLibrary: true,
      libraries: [],
      holderCount: 1,
    },
    {
      id: 2,
      name: "user",
      isBuiltin: true,
      capabilities: [],
      isEveryLibrary: false,
      libraries: [],
      holderCount: 1,
    },
    {
      id: 3,
      name: "family",
      isBuiltin: false,
      capabilities: [],
      isEveryLibrary: false,
      libraries: [{ libraryId: 1 }],
      holderCount: 0,
    },
  ],
};

const noLibraryChosen = "This role has no library yet";
const noLibraryTicked = "No library is ticked for this role";

function me(id: number, hasNoLibrary = false): Answers[string] {
  return [
    200,
    {
      id,
      name: "someone",
      role: "admin",
      isAdmin: true,
      canHostWatchTogether: true,
      hasNoLibrary,
    },
  ];
}

/** A person's page, as the admin with id adminId sees it, and the services it reaches. */
async function mountPerson(
  adminId: number,
  heard: Heard[] = [],
): Promise<{ readonly page: Mounted; readonly services: Services }> {
  const services = servedBy(
    {
      "GET /me": me(adminId),
      "GET /users/5": [
        200,
        {
          account: { id: 5, name: "Dan", role: "user", hasPassword: true },
          watchStats: { watchedMs: 0, titlesWatchedCount: 0 },
        },
      ],
      "GET /users/5/devices": [200, { devices: [] }],
      "GET /roles": [200, roles],
      "DELETE /users/5": [204, null],
    },
    heard,
  );
  return { page: await mount(withServices(<PersonPage userId={5} />, services)), services };
}

it("warns when the role chosen for someone new, or ticked for nobody, reaches no library", async () => {
  mounted = await mount(
    withServices(
      <People />,
      servedBy({
        "GET /users": [200, { accounts: [] }],
        "GET /roles": [200, roles],
        "GET /libraries": [200, { libraries: [] }],
        "GET /invitations": [200, { invitations: [] }],
      }),
    ),
  );
  const invitation = buttonWithText(mounted.container, "Make their link")?.closest("form");
  expect(invitation?.textContent).toContain(noLibraryChosen);
  await choose(invitation?.querySelector("select"), "family");
  expect(invitation?.textContent).not.toContain(noLibraryChosen);
  const userRole = [...mounted.container.querySelectorAll("form")].find(
    (form) => form.querySelector("h3")?.textContent === "Can watch",
  );
  expect(userRole?.textContent).toContain(noLibraryTicked);
  const adminRole = [...mounted.container.querySelectorAll("form")].find(
    (form) => form.querySelector("h3")?.textContent === "Administrator",
  );
  expect(adminRole?.textContent).not.toContain(noLibraryTicked);
});

it("removes a person once the admin confirms, and goes back to everyone (S40)", async () => {
  const heard: Heard[] = [];
  const { page, services } = await mountPerson(1, heard);
  mounted = page;
  expect(mounted.container.textContent).toContain(noLibraryChosen);
  await click(buttonWithText(mounted.container, "Remove Dan…"));
  expect(mounted.container.textContent).toContain("Dan is signed out everywhere at once");
  expect(heard.some((each) => each.methodAndPath === "DELETE /users/5")).toBe(false);
  await click(buttonWithText(mounted.container, "Remove them"));
  expect(heard.some((each) => each.methodAndPath === "DELETE /users/5")).toBe(true);
  expect(services.navigation.route()).toEqual({ screen: "people" });
});

it("offers no removal on the admin's own page", async () => {
  mounted = (await mountPerson(5)).page;
  expect(buttonWithText(mounted.container, "Remove Dan…")).toBeUndefined();
});

it("tells someone given no library why their list is empty", async () => {
  const empty = { state: "loaded", value: { nextCursor: "", items: [] } } as const;
  const view = (
    <BrowseView kind="movie" loadedEntries={empty} isLoadingMore={false} onLoadMore={ignore} />
  );
  mounted = await mount(withServices(view, servedBy({ "GET /me": me(5, true) })));
  expect(mounted.container.textContent).toContain("You have not been given any library yet");
  mounted.unmount();
  mounted = await mount(withServices(view, servedBy({ "GET /me": me(5, false) })));
  expect(mounted.container.textContent).toContain("Once a library is scanned");
});
