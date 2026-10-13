// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Account } from "./account.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { television } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The account page for a session that may, or may not, change the account's security. */
async function accountOf(canManageAccount: boolean): Promise<HTMLElement> {
  mounted = await mount(
    withServices(
      <Account />,
      servedBy({
        "GET /me/devices": [200, { devices: [television()] }],
        "GET /me": [200, { id: 1, name: "owner", role: "user", isAdmin: false, canManageAccount }],
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
});
