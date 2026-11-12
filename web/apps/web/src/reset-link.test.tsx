// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { dateTimeText } from "./date-time-text.ts";
import { UserPage } from "./user.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/*
 * A registered person's reset password link, on their page (D466): New reset password link while
 * none is outstanding; while one is, until when, with Cancel and Renew, never the link itself.
 */

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const expiresAt = Date.UTC(2026, 10, 13, 9, 30);

function page(hasPassword: boolean, resetLinkExpiresAt?: number): unknown {
  return {
    user: { id: 5, name: "Dan", role: "user", hasPassword },
    watchStats: { watchedMs: 0, titlesWatchedCount: 0 },
    ...(resetLinkExpiresAt === undefined ? {} : { resetLinkExpiresAt }),
  };
}

const around = {
  "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
  "GET /users/5/devices": [200, { devices: [] }],
  "GET /roles": [200, { capabilities: [], roles: [] }],
} as const;

function buttonTexts(container: HTMLElement): (string | null)[] {
  return [...container.querySelectorAll("button")].map((button) => button.textContent);
}

it("offers a registered person with no reset link outstanding a New reset password link", async () => {
  mounted = await mount(
    withServices(
      <UserPage userId={5} />,
      servedBy({ ...around, "GET /users/5": [200, page(true)] }),
    ),
  );
  const buttons = buttonTexts(mounted.container);
  expect(buttons).toContain("New reset password link");
  expect(buttons).not.toContain("New link");
  expect(buttons).not.toContain("Renew");
  expect(mounted.container.textContent).not.toContain("A reset password link is live");
});

it("keeps New link for a person not registered yet", async () => {
  mounted = await mount(
    withServices(
      <UserPage userId={5} />,
      servedBy({ ...around, "GET /users/5": [200, page(false)] }),
    ),
  );
  const buttons = buttonTexts(mounted.container);
  expect(buttons).toContain("New link");
  expect(buttons).not.toContain("New reset password link");
});

it("says until when an outstanding reset link is live, with Cancel and Renew, and never the link", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <UserPage userId={5} />,
      servedBy(
        {
          ...around,
          "GET /users/5": [
            [200, page(true, expiresAt)],
            [200, page(true)],
          ],
          "DELETE /users/5/registration-invitation": [204, null],
        },
        heard,
      ),
    ),
  );
  const container = mounted.container;
  expect(container.textContent).toContain(
    `A reset password link is live until ${dateTimeText(expiresAt, "en")}.`,
  );
  const buttons = buttonTexts(container);
  expect(buttons).toEqual(expect.arrayContaining(["Cancel", "Renew"]));
  expect(buttons).not.toContain("New reset password link");
  expect(container.querySelector(".people__issued")).toBeNull();
  await click(buttonWithText(container, "Cancel"));
  await click(buttonWithText(container, "Cancel the link"));
  expect(heard.map((each) => each.methodAndPath)).toContain(
    "DELETE /users/5/registration-invitation",
  );
  expect(container.textContent).not.toContain("A reset password link is live");
  expect(buttonTexts(container)).toContain("New reset password link");
});

it("renews an outstanding reset link, showing the new link and its QR code", async () => {
  const heard: Heard[] = [];
  const renewedAt = expiresAt + 600_000;
  mounted = await mount(
    withServices(
      <UserPage userId={5} />,
      servedBy(
        {
          ...around,
          "GET /users/5": [
            [200, page(true, expiresAt)],
            [200, page(true, renewedAt)],
          ],
          "POST /users/5/registration-invitation": [
            201,
            { secret: "the-new-secret", expiresAt: renewedAt },
          ],
        },
        heard,
      ),
    ),
  );
  const container = mounted.container;
  expect(container.innerHTML).not.toContain("the-new-secret");
  await click(buttonWithText(container, "Renew"));
  await click(buttonWithText(container, "Make a new link"));
  expect(heard.map((each) => each.methodAndPath)).toContain(
    "POST /users/5/registration-invitation",
  );
  const issued = container.querySelector(".people__issued");
  expect(issued?.querySelector("h2")?.textContent).toBe(
    "Dan's reset password link: send it to them",
  );
  expect(issued?.innerHTML).toContain("the-new-secret");
  expect(issued?.querySelector('img[alt="Dan\'s link as a QR code"]')).not.toBeNull();
  expect(container.textContent).toContain(
    `A reset password link is live until ${dateTimeText(renewedAt, "en")}.`,
  );
});
