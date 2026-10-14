// SPDX-License-Identifier: MPL-2.0

/**
 * The built client served as a discovery's page (webrtc-transport.md W4c): the page names the
 * discovery's client API, and the client opens on the gate that finds its core rather than on the
 * core's own app. The discovery is answered here, inside the browser's routing, so nothing leaves
 * the machine; the whole path through a discovery is W4b.6's.
 */

import { expect, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import { serverUrl } from "./pages.ts";

const clientApi = "https://discovery.invalid/client/v1";
const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";

/** The core's content-security policy with the discovery's client API added to what the page may call. */
function discoveryPolicy(corePolicy: string): string {
  const origin = new URL(clientApi).origin;
  return corePolicy.includes("connect-src")
    ? corePolicy.replace(/connect-src ([^;]*)/u, `connect-src $1 ${origin}`)
    : `${corePolicy}; connect-src 'self' ${origin}`;
}

/**
 * Serves the core's page as a discovery serves it, naming its client API, and answers every call to
 * that API as a discovery that knows no such link would; returns the paths asked.
 */
async function asADiscoverysPage(page: Page): Promise<string[]> {
  const asked: string[] = [];
  await page.route(`${serverUrl()}/**`, async (route) => {
    if (route.request().resourceType() !== "document") {
      await route.fallback();
      return;
    }
    const served = await route.fetch();
    const named = `<head><meta name="orpheum-discovery-client-api" content="${clientApi}">`;
    // The discovery's own policy for its page, which lets the page call its client API; the core's
    // allows only the core.
    const headers = served.headers();
    headers["content-security-policy"] = discoveryPolicy(headers["content-security-policy"] ?? "");
    await route.fulfill({
      response: served,
      headers,
      body: (await served.text()).replace("<head>", named),
    });
  });
  await page.route(`${clientApi}/**`, async (route) => {
    asked.push(new URL(route.request().url()).pathname);
    await route.fulfill({
      status: 404,
      contentType: "application/json",
      headers: { "access-control-allow-origin": "*", "access-control-allow-headers": "*" },
      body: JSON.stringify({ code: "not_found" }),
    });
  });
  return asked;
}

test("a discovery's page with no link and no server reached before says how to get in", async ({
  page,
}) => {
  const asked = await asADiscoverysPage(page);
  await page.goto(serverUrl());
  await expect(page.getByText("Open the link you were sent")).toBeVisible();
  await expect(page.getByLabel("Password")).toBeHidden();
  expect(asked).toEqual([]);
});

test("a discovery's page spends a link only on its click, and says when it finds nothing", async ({
  page,
}) => {
  const asked = await asADiscoverysPage(page);
  await page.goto(`${serverUrl()}/register#k7xq2.a-links-secret.${fingerprint}`);
  await expect(page.getByRole("heading", { name: "Your server's link" })).toBeVisible();
  expect(asked).toEqual([]);
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(
    page.getByRole("heading", { name: "This link no longer finds its server" }),
  ).toBeVisible();
  expect(asked).toEqual(["/client/v1/locate"]);
});
