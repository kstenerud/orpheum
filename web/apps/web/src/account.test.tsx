// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Account } from "./account.tsx";
import { mount, refusal, servedBy, type Answer, type Mounted } from "./test-dom.tsx";
import { television } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The account page for a session that may, or may not, change the account's security. */
async function accountOf(
  canManageAccount: boolean,
  ownLink: Answer = refusal(404, "not_found"),
): Promise<HTMLElement> {
  mounted = await mount(
    withServices(
      <Account />,
      servedBy({
        "GET /me/devices": [200, { devices: [television()] }],
        "GET /me": [200, { id: 1, name: "owner", role: "user", isAdmin: false, canManageAccount }],
        "GET /me/own-link": ownLink,
      }),
    ),
  );
  return mounted.container;
}

describe("your account", () => {
  it("offers a session that may not change the account's security only what it may do, and says where to do the rest (D292 §4)", async () => {
    const limited = await accountOf(false);
    expect(limited.textContent).toContain("Living room TV");
    expect(limited.textContent).not.toContain("Change password");
    expect(limited.textContent).not.toContain("Activate a device");
    expect(limited.textContent).not.toContain("Remove");
    expect(limited.textContent).not.toContain("Sign out everywhere");
    expect(limited.textContent).toContain("not change your password");
    mounted?.unmount();
    const full = await accountOf(true);
    for (const offered of ["Change password", "Activate a device", "Remove"]) {
      expect(full.textContent).toContain(offered);
    }
    expect(full.textContent).not.toContain("not change your password");
  });

  it("shows the link the person registered by, to open or scan on another device, and nothing without one (D292 §4)", async () => {
    const link =
      "https://app.example/register#k7xq2.a-links-secret.Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
    const page = await accountOf(true, [200, { secret: "a-links-secret", link, expiresAt: 2 }]);
    expect(page.querySelector("output")?.textContent).toBe(link);
    expect(page.querySelector('img[alt="Your link, as a QR code"]')).not.toBeNull();
    mounted?.unmount();
    // On the server's own origin, before a discovery claims it, the link is the origin's.
    const home = await accountOf(true, [200, { secret: "a-links-secret", expiresAt: 2 }]);
    expect(home.querySelector("output")?.textContent).toBe(
      "http://orpheum.test/register#a-links-secret",
    );
    mounted?.unmount();
    const none = await accountOf(true);
    expect(none.textContent).not.toContain("Your link");
  });
});
