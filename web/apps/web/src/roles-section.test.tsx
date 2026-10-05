// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { People } from "./people.tsx";
import { PersonPage } from "./person.tsx";
import {
  buttonWithText,
  choose,
  click,
  enter,
  mount,
  servedBy,
  submit,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const roles = {
  capabilities: ["administer", "edit_metadata", "host_watch_together"],
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
      capabilities: ["host_watch_together"],
      isEveryLibrary: false,
      libraries: [{ libraryId: 1 }, { libraryId: 2 }],
      holderCount: 1,
    },
    {
      id: 3,
      name: "friends",
      isBuiltin: false,
      capabilities: [],
      isEveryLibrary: false,
      libraries: [],
      holderCount: 0,
    },
  ],
};
const libraries = {
  libraries: [
    {
      id: 1,
      name: "Films",
      kind: "movies",
      metadataSource: "nfo",
      changeDetection: "manual",
      isWritable: false,
      createdAt: 1,
    },
    {
      id: 2,
      name: "Private",
      kind: "movies",
      metadataSource: "nfo",
      changeDetection: "manual",
      isWritable: false,
      createdAt: 1,
    },
  ],
};

/** The People page, answered with the roles and libraries above. */
async function mountPeople(heard: Heard[]): Promise<Mounted> {
  return await mount(
    withServices(
      <People />,
      servedBy(
        {
          "GET /users": [200, { accounts: [] }],
          "GET /roles": [200, roles],
          "GET /libraries": [200, libraries],
          "PUT /roles/2": [204, null],
          "POST /roles": [201, { id: 4 }],
          "GET /invitations": [200, { invitations: [] }],
          "POST /invitations": [
            201,
            {
              invitation: {
                id: 3,
                role: "friends",
                adminNote: "my cousin",
                issuedAt: 1,
                expiresAt: 2,
              },
              link: { secret: "s", expiresAt: 2 },
            },
          ],
        },
        heard,
      ),
    ),
  );
}

it("takes a library back from a role, and makes a role, as the admin asks (D243 §4)", async () => {
  const heard: Heard[] = [];
  mounted = await mountPeople(heard);
  const userRole = [...mounted.container.querySelectorAll("form")].find(
    (form) => form.querySelector("h3")?.textContent === "Can watch",
  );
  const privateBox = [...(userRole?.querySelectorAll("label") ?? [])].find((label) =>
    label.textContent.includes("Private"),
  );
  await click(privateBox?.querySelector("input"));
  await submit(userRole);
  expect(heard.find((each) => each.methodAndPath === "PUT /roles/2")?.body).toEqual({
    name: "user",
    capabilities: ["host_watch_together"],
    libraries: [{ libraryId: 1 }],
  });
  expect(mounted.container.textContent).toContain("Every library: administrators see everything.");

  const newRole = buttonWithText(mounted.container, "Add the role")?.closest("form");
  await enter(newRole?.querySelector("input"), "cousins");
  await submit(newRole);
  expect(heard.find((each) => each.methodAndPath === "POST /roles")?.body).toEqual({
    name: "cousins",
    capabilities: [],
    libraries: [],
  });
});

it("invites someone into a role with a note of who it is for, and shows the link to send (D246 §2)", async () => {
  const heard: Heard[] = [];
  mounted = await mountPeople(heard);
  const invitation = buttonWithText(mounted.container, "Make their link")?.closest("form");
  await enter(invitation?.querySelector("input"), "my cousin");
  await choose(invitation?.querySelector("select"), "friends");
  await submit(invitation);
  expect(heard.find((each) => each.methodAndPath === "POST /invitations")?.body).toEqual({
    role: "friends",
    adminNote: "my cousin",
  });
  expect(mounted.container.textContent).toContain("my cousin's link: send it to them");
});

it("gives a person another role from their page", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <PersonPage userId={5} />,
      servedBy(
        {
          "GET /users/5": [
            200,
            {
              account: { id: 5, name: "cousin", role: "user", hasPassword: true },
              watchStats: { watchedMs: 0, titlesWatchedCount: 0 },
            },
          ],
          "GET /users/5/devices": [200, { devices: [] }],
          "GET /roles": [200, roles],
          "PUT /users/5/role": [204, null],
        },
        heard,
      ),
    ),
  );
  const choice = buttonWithText(mounted.container, "Give this role")?.closest("form");
  await choose(choice?.querySelector("select"), "friends");
  await submit(choice);
  expect(heard.find((each) => each.methodAndPath === "PUT /users/5/role")?.body).toEqual({
    role: "friends",
  });
});
