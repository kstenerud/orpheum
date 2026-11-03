// SPDX-License-Identifier: MPL-2.0

/**
 * The built client served as a discovery's page (webrtc-transport.md W4c; D308): the page names the
 * loader's configuration, and opens as the loader, on the gate that finds its core, rather than on
 * the core's own app; a link opens at its core's client origin. The discovery is answered here,
 * inside the browser's routing, so nothing leaves the machine; the whole path through a discovery is
 * W4b.6's.
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

/** A core's client origin, at <coreId>.localhost on the core's port, which a browser takes as secure. */
function clientOriginOf(coreId: string): string {
  return `http://${coreId}.localhost:${new URL(serverUrl()).port}`;
}

/**
 * Serves the core's page as a discovery serves it, at the core's own origin, which is the loader's,
 * and at a core's client origin, naming the loader's configuration, and answers every call to the
 * discovery's API as a discovery that knows no such link would; returns the paths asked.
 */
async function asADiscoverysPage(page: Page): Promise<string[]> {
  const asked: string[] = [];
  const pattern = clientOriginOf("{core}");
  for (const origin of [serverUrl(), clientOriginOf("k7xq2")]) {
    await page.route(`${origin}/**`, async (route) => {
      await route.fulfill(await asTheDiscoveryServes(route, origin, pattern));
    });
  }
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

/**
 * Ends the test's routing of page, its last step: a load the page still makes as the test ends (a
 * script of the client it goes on loading), its handler still waiting on the core, otherwise calls
 * route.fulfill after the context closed, which has dropped what route.fetch read: "route.fulfill:
 * Fetch response has been disposed", or "route.fetch: Test ended", failing a test that had passed, or
 * the next one its worker runs. Unrouted with "ignoreErrors" in the test's own body, a handler in
 * flight fails nothing (idiosyncrasies.md, "Playwright fails a test whose route handler is still
 * running as it ends").
 */
async function stopRouting(page: Page): Promise<void> {
  await page.unrouteAll({ behavior: "ignoreErrors" });
}

/**
 * The core's own answer to a load of origin, as the discovery serves it there: its page naming the
 * loader's configuration, under a policy that lets it call the discovery's API.
 */
async function asTheDiscoveryServes(
  route: Parameters<Parameters<Page["route"]>[1]>[0],
  origin: string,
  pattern: string,
): Promise<Parameters<typeof route.fulfill>[0]> {
  // The core's listener knows its own name alone: the load goes to it there.
  const headers = Object.fromEntries(
    Object.entries(route.request().headers()).filter(([name]) => name !== "host"),
  );
  const served = await route.fetch({
    url: route.request().url().replace(origin, serverUrl()),
    headers,
  });
  if (route.request().resourceType() !== "document") {
    return { response: served };
  }
  const named =
    `<head><meta name="orpheum-discovery-client-api" content="${clientApi}">` +
    `<meta name="orpheum-client-origins" content="${pattern}">` +
    `<meta name="orpheum-loader-origin" content="${serverUrl()}">`;
  // The discovery's own policy for its page, which lets the page call its client API; the core's
  // allows only the core.
  const answered = served.headers();
  answered["content-security-policy"] = discoveryPolicy(answered["content-security-policy"] ?? "");
  return {
    response: served,
    headers: answered,
    body: (await served.text()).replace("<head>", named),
  };
}

test("a discovery's page with no link and no server reached before says how to get in", async ({
  page,
}) => {
  const asked = await asADiscoverysPage(page);
  await page.goto(serverUrl());
  await expect(page.getByText("Open the link you were sent")).toBeVisible();
  await expect(page.getByLabel("Password")).toBeHidden();
  expect(asked).toEqual([]);
  await stopRouting(page);
});

test("a discovery's page spends a link only on its click, and says when it finds nothing", async ({
  page,
}) => {
  const asked = await asADiscoverysPage(page);
  // A link leads to its core's own client origin (D308).
  await page.goto(`${clientOriginOf("k7xq2")}/register#k7xq2.a-links-secret.${fingerprint}`);
  await expect(page.getByRole("heading", { name: "Your server's link" })).toBeVisible();
  expect(asked).toEqual([]);
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(
    page.getByRole("heading", { name: "This link no longer finds its server" }),
  ).toBeVisible();
  expect(asked).toEqual(["/client/v1/locate"]);
  await stopRouting(page);
});
