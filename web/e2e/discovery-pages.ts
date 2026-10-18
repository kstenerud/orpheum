// SPDX-License-Identifier: MPL-2.0

/**
 * A development discovery for the browser tests that reach the core through one
 * (through-discovery.spec.ts, through-discovery-realm.spec.ts): started, the core connected to it and
 * claimed in its operator's place, and the owner's API beside.
 */

import { spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { expect, type APIRequestContext } from "@playwright/test";

import { serverUrl } from "./pages.ts";
import { owner } from "./server.ts";

/** A development discovery: its process, the address the core calls, and where the loader is. */
export interface Discovery {
  readonly process: ChildProcess;
  readonly address: string;
  /** The loader's own origin, localhost on the discovery's port (D308). */
  readonly loaderOrigin: string;
  /** A core's client origin, <coreId>.localhost on the discovery's port. */
  readonly clientOriginOf: (coreId: string) => string;
}

/**
 * Starts `orpheum dev discovery` on a free loopback port, a paid discovery's realm when one is named,
 * and is it.
 */
export async function startDiscovery(realm?: string): Promise<Discovery> {
  const binary = process.env["ORPHEUM_TEST_BINARY"];
  if (binary === undefined) {
    throw new Error("ORPHEUM_TEST_BINARY names no binary");
  }
  const state = mkdtempSync(path.join(tmpdir(), "orpheum-discovery-"));
  const realmFlags = realm === undefined ? [] : ["--realm", realm];
  const started = spawn(
    binary,
    ["dev", "discovery", "--listen", "127.0.0.1:0", "--state", state, ...realmFlags],
    {
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
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
  const port = new URL(address).port;
  return {
    process: started,
    address,
    loaderOrigin: `http://localhost:${port}`,
    clientOriginOf: (coreId) => `http://${coreId}.localhost:${port}`,
  };
}

/** The owner's session token, signed in over the core's own API. */
export async function ownersToken(api: APIRequestContext): Promise<string> {
  const answer = await api.post(`${serverUrl()}/api/v1/session`, {
    data: {
      name: owner.name,
      password: owner.password,
      device: { name: "Discovery test", platform: "web" },
    },
  });
  expect(answer.ok(), `the sign-in: ${String(answer.status())} ${await answer.text()}`).toBe(true);
  const token = await answerField(answer, "token");
  if (typeof token !== "string") {
    throw new Error("the sign-in gave no token");
  }
  return token;
}

/** A field of a JSON answer, or of a JSON value. */
export function fieldOf(value: unknown, name: string): unknown {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  const field: unknown = Reflect.get(value, name);
  return field;
}

/** A field of a JSON answer. */
export async function answerField(
  answer: Awaited<ReturnType<APIRequestContext["get"]>>,
  name: string,
): Promise<unknown> {
  return fieldOf(await answer.json(), name);
}

/** The headers a signed-in call carries. */
export function signedIn(token: string): Record<string, string> {
  return Object.fromEntries([["Authorization", `Bearer ${token}`]]);
}

/** Connects the core to the discovery as its admin does, and claims it in the operator's place. */
export async function connectAndClaim(
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
