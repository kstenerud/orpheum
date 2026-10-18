// SPDX-License-Identifier: MPL-2.0

/**
 * The paid discovery's realm sign-in (D297, D298), in a spec of its own: its discovery has a realm,
 * and the core holds one discovery, so it cannot share through-discovery.spec.ts's.
 */

import { expect, request } from "@playwright/test";

import { test } from "./browser-map.ts";
import {
  connectAndClaim,
  fieldOf,
  ownersToken,
  signedIn,
  startDiscovery,
} from "./discovery-pages.ts";
import { serverUrl } from "./pages.ts";
import { owner } from "./server.ts";

/**
 * A person with no link at a paid discovery's page (D297, D298): the discovery has the realm
 * karlnas, and the owner, having signed in once with their password since the core was given it, has
 * a realm key. On the page they type the realm, their name and their password, the discovery passes
 * the sign-in to the core, the core's tag proves the answer its own, and the app opens signed in, the
 * realm in its banner.
 */
test("a person with no link signs in at a paid discovery's page by realm, name and password", async ({
  page,
}) => {
  test.setTimeout(180_000);
  const discovery = await startDiscovery("karlnas");
  const api = await request.newContext();
  const token = await ownersToken(api);
  try {
    await connectAndClaim(api, token, discovery.address);
    // A sign-in with the password since the core has its realm makes the owner's realm key, and the
    // sign-in waits for the core's signalling connection, as one long after its claim finds it.
    await ownersToken(api);
    await expect
      .poll(
        async () =>
          fieldOf(await (await api.get(`${discovery.address}/dev/core`)).json(), "isOnline"),
        {
          timeout: 60_000,
        },
      )
      .toBe(true);
    // The realm sign-in is at the loader's own origin, which hands the browser on to the core's.
    await page.goto(discovery.loaderOrigin);
    await page.getByLabel("Realm", { exact: true }).fill("KarlNAS");
    await page.getByLabel("Name", { exact: true }).fill(owner.name);
    await page.getByLabel("Password", { exact: true }).fill(owner.password);
    // The box sits in one row beside its words.
    await expect(page.locator(".credentials-form__check")).toHaveCSS("display", "flex");
    await page.getByLabel("This is my own device: remember my realm").check();
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.locator(".app-shell__realm")).toHaveText("karlnas", { timeout: 60_000 });
    expect(new URL(page.url()).hostname.endsWith(".localhost")).toBe(true);
    // Quiet beside the tray, on one line however narrow the banner.
    await expect(page.locator(".app-shell__realm")).toHaveCSS("white-space", "nowrap");
    await expect(page.getByRole("link", { name: "Films" })).toBeVisible();
    // Back at the loader, the household is listed with a small red Forget at its row's end, not a
    // button the form's width beside a small name (the owner, 2026-10-17).
    await page.goto(`${discovery.loaderOrigin}/?households=`);
    const forget = page.getByRole("button", { name: "Forget karlnas" });
    const name = await page.getByRole("link", { name: "karlnas" }).boundingBox();
    const drawn = await forget.boundingBox();
    const list = await page.locator(".loader-households").boundingBox();
    if (name === null || drawn === null || list === null) {
      throw new Error("the household list is drawn nowhere");
    }
    expect(drawn.width).toBeLessThan(list.width / 2);
    expect(Math.abs(drawn.y + drawn.height / 2 - (name.y + name.height / 2))).toBeLessThan(3);
    await forget.click();
    await page.getByRole("button", { name: "Forget it" }).click();
    await expect(page.locator(".loader-households li")).toHaveCount(0);
  } finally {
    await api.delete(`${serverUrl()}/api/v1/system/discovery`, { headers: signedIn(token) });
    await api.dispose();
    discovery.process.kill();
  }
});
