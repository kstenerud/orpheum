// SPDX-License-Identifier: MPL-2.0

/**
 * A browser reaching its core through a discovery (discovery-mode.md W4b.6; webrtc-transport.md W5):
 * the core enrols with a single-target discovery and is claimed; a registration link in the
 * discovery's form, opened on the discovery's page, finds the core, reaches it over WebRTC through
 * the discovery's offer (the discovery publishes no direct name, so the race has only WebRTC), checks
 * the core's answer by the link's fingerprint, registers, and plays a film. Every public part of the
 * discovery meets the core and the web client here.
 */

import { spawn, type ChildProcess } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { expect, request, type APIRequestContext } from "@playwright/test";

import { test } from "./browser-map.ts";
import { isPlaying, serverUrl } from "./pages.ts";
import { owner, warmUpFilm } from "./server.ts";

/** Starts `orpheum dev discovery` on a free loopback port, and is its process and its address. */
async function startDiscovery(): Promise<{ process: ChildProcess; address: string }> {
  const binary = process.env["ORPHEUM_TEST_BINARY"];
  if (binary === undefined) {
    throw new Error("ORPHEUM_TEST_BINARY names no binary");
  }
  const state = mkdtempSync(path.join(tmpdir(), "orpheum-discovery-"));
  const started = spawn(binary, ["dev", "discovery", "--listen", "127.0.0.1:0", "--state", state], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  let output = "";
  started.stderr.on("data", (chunk: Buffer) => {
    output += chunk.toString();
  });
  const address = await new Promise<string>((resolve, reject) => {
    started.stdout.on("data", (chunk: Buffer) => {
      const served = /serving the discovery at (\S+)/u.exec(chunk.toString());
      if (served?.[1] !== undefined) {
        resolve(served[1]);
      }
    });
    started.once("exit", (code) => {
      reject(new Error(`the discovery exited (${String(code)}):\n${output}`));
    });
  });
  return { process: started, address };
}

/** The owner's session token, signed in over the core's own API. */
async function ownersToken(api: APIRequestContext): Promise<string> {
  const answer = await api.post(`${serverUrl()}/api/v1/session`, {
    data: {
      name: owner.name,
      password: owner.password,
      device: { name: "Discovery test", platform: "web" },
    },
  });
  expect(answer.ok()).toBe(true);
  const token = await answerField(answer, "token");
  if (typeof token !== "string") {
    throw new Error("the sign-in gave no token");
  }
  return token;
}

/** A field of a JSON answer, or of a JSON value. */
function fieldOf(value: unknown, name: string): unknown {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  const field: unknown = Reflect.get(value, name);
  return field;
}

/** A field of a JSON answer. */
async function answerField(
  answer: Awaited<ReturnType<APIRequestContext["get"]>>,
  name: string,
): Promise<unknown> {
  return fieldOf(await answer.json(), name);
}

/** The headers a signed-in call carries. */
function signedIn(token: string): Record<string, string> {
  return Object.fromEntries([["Authorization", `Bearer ${token}`]]);
}

/** A link's locate hash, as the client makes it (discovery/link.LocateHash). */
function locateHash(secret: string): string {
  return createHash("sha256").update(`orpheum-locate${secret}`).digest("base64url");
}

/** Connects the core to the discovery as its admin does, and claims it in the operator's place. */
async function connectAndClaim(
  api: APIRequestContext,
  token: string,
  discovery: string,
): Promise<void> {
  const connected = await api.post(`${serverUrl()}/api/v1/system/discovery`, {
    headers: signedIn(token),
    data: { baseUrl: `${discovery}/core/v1` },
  });
  expect(connected.ok()).toBe(true);
  const claimed = await api.post(`${discovery}/dev/claim`, {
    data: { code: await answerField(connected, "claimCode") },
  });
  expect(claimed.status()).toBe(204);
  await expect
    .poll(
      async () =>
        await answerField(
          await api.get(`${serverUrl()}/api/v1/system/discovery`, { headers: signedIn(token) }),
          "state",
        ),
      { timeout: 60_000 },
    )
    .toBe("claimed");
}

/**
 * An invitation as a person is sent it, in the discovery's form: the link the core makes is on its
 * own origin, the discovery's being plain HTTP, so it is written here by the core's id and
 * fingerprint. Returned once the discovery finds the core by it, online on its signalling connection.
 */
async function invitationThrough(
  api: APIRequestContext,
  token: string,
  discovery: string,
): Promise<string> {
  const invited = await api.post(`${serverUrl()}/api/v1/invitations`, {
    headers: signedIn(token),
    data: { role: "admin", adminNote: "through the discovery" },
  });
  expect(invited.status()).toBe(201);
  const secret = fieldOf(await answerField(invited, "link"), "secret");
  const core: unknown = await (await api.get(`${discovery}/dev/core`)).json();
  const coreId = fieldOf(core, "coreId");
  const fingerprint = fieldOf(core, "fingerprint");
  if (typeof secret !== "string" || typeof coreId !== "string" || typeof fingerprint !== "string") {
    throw new Error("the invitation or the discovery's core is not as made");
  }
  await expect
    .poll(
      async () =>
        await answerField(
          await api.post(`${discovery}/client/v1/locate`, {
            headers: { "Orpheum-Discovery-Protocol": "1.0" },
            data: { hash: locateHash(secret) },
          }),
          "state",
        ),
      { timeout: 60_000 },
    )
    .toBe("online");
  return `${discovery}/register#${coreId}.${secret}.${fingerprint}`;
}

test("a browser registers and plays through a discovery, over WebRTC, its core's answer checked by the link's fingerprint", async ({
  page,
}) => {
  test.setTimeout(240_000);
  const discovery = await startDiscovery();
  const api = await request.newContext();
  const token = await ownersToken(api);
  try {
    await connectAndClaim(api, token, discovery.address);
    const link = await invitationThrough(api, token, discovery.address);
    const movedToWebrtc = page.waitForEvent("console", (message) =>
      message.text().includes("requests now go over WebRTC"),
    );
    await page.goto(link);
    await page.getByRole("button", { name: "Connect" }).click();
    await movedToWebrtc;
    // A name of each browser's own: the browsers share one core.
    await page.getByLabel("Your name").fill(`guest-${test.info().project.name}`);
    await page.getByLabel("Password", { exact: true }).fill("guest-password");
    await page.getByLabel("The same password again").fill("guest-password");
    await page.getByRole("button", { name: "Set password and sign in" }).click();
    await page.getByRole("link", { name: "Films", exact: true }).click();
    const card = page
      .locator("a.poster-grid__card")
      .filter({ hasText: new RegExp(warmUpFilm, "u") });
    await expect(card).toBeVisible({ timeout: 60_000 });
    await card.click();
    await page.getByRole("link", { name: "Play" }).click();
    await page.getByRole("button", { name: /Built-in speakers/u }).click();
    await expect.poll(() => isPlaying(page), { timeout: 60_000 }).toBe(true);
  } finally {
    // The core leaves the discovery, as the next test, or the next browser, finds it free.
    await api.delete(`${serverUrl()}/api/v1/system/discovery`, { headers: signedIn(token) });
    await api.dispose();
    discovery.process.kill();
  }
});
