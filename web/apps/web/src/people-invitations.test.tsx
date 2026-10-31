// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it, vi } from "vitest";

import { People } from "./people.tsx";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
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
  links: [],
};
const used = {
  id: 6,
  role: "user",
  adminNote: "Grandma",
  issuedAt: 1,
  liveLinks: 0,
  expiresAt: 0,
  links: [],
  made: { userId: 4, name: "Gran", at: 1_790_000_000_000 },
};

it("shows a waiting invitation's live links again, each with Copy (D267 §5)", async () => {
  const live = {
    ...waiting,
    liveLinks: 2,
    expiresAt: 1_790_000_000_000,
    links: [
      { secret: "first-secret", expiresAt: 1_789_000_000_000 },
      {
        secret: "second-secret",
        expiresAt: 1_790_000_000_000,
        link: "https://discovery.example/register#core.second-secret.print",
      },
    ],
  };
  const copied: string[] = [];
  const clipboard = { writeText: (text: string) => Promise.resolve(void copied.push(text)) };
  vi.stubGlobal("isSecureContext", true);
  vi.stubGlobal("navigator", { ...globalThis.navigator, clipboard });
  try {
    mounted = await mount(
      withServices(
        <People />,
        servedBy({
          "GET /users": [200, { accounts: [] }],
          "GET /roles": [200, roles],
          "GET /libraries": [200, { libraries: [] }],
          "GET /invitations": [200, { invitations: [live, used] }],
        }),
      ),
    );
    const waitingList = mounted.container.querySelector(
      'section[aria-labelledby="people-invitations"]',
    );
    const shown = [...(waitingList?.querySelectorAll(".share__link") ?? [])].map(
      (link) => link.textContent,
    );
    expect(shown).toHaveLength(2);
    expect(shown[0]).toMatch(/\/register#first-secret$/u);
    expect(shown[1]).toBe("https://discovery.example/register#core.second-secret.print");
    const copies = [...(waitingList?.querySelectorAll("button") ?? [])].filter(
      (button) => button.textContent === "Copy",
    );
    expect(copies).toHaveLength(2);
    await click(copies[1] ?? null);
    await settleUntil(() => copied.length === 1);
    expect(copied).toEqual(["https://discovery.example/register#core.second-secret.print"]);
    await settleUntil(() => waitingList?.textContent.includes("Copied.") === true);
    // A used invitation's link is its person's own now, never shown here.
    const usedList = mounted.container.querySelector(
      'section[aria-labelledby="people-used-invitations"]',
    );
    expect(usedList?.querySelector(".share__link")).toBeNull();
  } finally {
    vi.unstubAllGlobals();
  }
});

it("sends a waiting invitation another link, and names the person a used one made (D318, D311 §4)", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <People />,
      servedBy(
        {
          "GET /users": [200, { accounts: [] }],
          "GET /roles": [200, roles],
          "GET /libraries": [200, { libraries: [] }],
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
