// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { People } from "./people.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const roles = {
  capabilities: ["administer"],
  roles: [
    {
      id: 2,
      name: "user",
      isBuiltin: true,
      capabilities: [],
      isEveryLibrary: true,
      libraries: [],
      holderCount: 1,
    },
  ],
};

const waiting = {
  id: 7,
  role: "user",
  adminNote: "my cousin",
  issuedAt: 1,
  liveLinks: 0,
  expiresAt: 0,
};
const used = {
  id: 6,
  role: "user",
  adminNote: "Grandma",
  issuedAt: 1,
  liveLinks: 0,
  expiresAt: 0,
  made: { userId: 4, name: "Gran", at: 1_790_000_000_000 },
};

it("sends a waiting invitation another link, and names the person a used one made (D318, D311 §4)", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <People />,
      servedBy(
        {
          "GET /users": [200, { accounts: [] }],
          "GET /roles": [200, roles],
          "GET /invitations": [200, { invitations: [waiting, used] }],
          "POST /invitations/7/links": [201, { secret: "another-secret", expiresAt: 2 }],
        },
        heard,
      ),
    ),
  );
  const page = mounted.container;
  expect(page.textContent).toContain("my cousin, as");
  expect(page.textContent).toContain("its links have ended");
  const usedList = page.querySelector('section[aria-labelledby="people-used-invitations"]');
  expect(usedList?.textContent).toContain("Grandma");
  expect(usedList?.querySelector("a")?.textContent).toBe("Gran");
  // A used invitation has nothing to send again or withdraw.
  expect(usedList?.querySelector("button")).toBeNull();
  await click(buttonWithText(page, "Send another link"));
  expect(heard.map((each) => each.methodAndPath)).toContain("POST /invitations/7/links");
  expect(page.textContent).toContain("my cousin's link: send it to them");
  expect(page.querySelector(".people__issued")?.textContent).toContain("another-secret");
});
