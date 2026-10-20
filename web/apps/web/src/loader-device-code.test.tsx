// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { handOffFragment } from "./discovery-start.ts";
import { DeviceCodeSignIn, type DeviceCodeParts } from "./loader-device-code.tsx";
import { mount, settleUntil, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

function fromBase64url(text: string): Uint8Array<ArrayBuffer> {
  const standard = text.replaceAll("-", "+").replaceAll("_", "/");
  return Uint8Array.from(
    atob(standard + "=".repeat((4 - (standard.length % 4)) % 4)),
    (char) => char.codePointAt(0) ?? 0,
  );
}

// The key and outcome discovery/proof/devicecode_test.go holds: what the core's seal made.
const fixture = {
  d: "xeruaKT9s4YsierbGsVdrQk8RzKuhPC2k453oBOIjJo",
  publicKey:
    "BMbJRSPCf69nfVclsx77C19vrA1JS1lRUkmpU5kSQdr53_JGKknN2W0eAMmPYZsdP_8JtRtLggy7CKKoOkhGKLg",
  sealed:
    "BBfNci1ok_9cjsv6uhX90qCwN1b0RE50DPnYo7SGlux2s80NY108R0TvHVo3KPY5KazNfFHeG4Dn2fkeKvMVP-oMf-wohuEBLdcVzhTfDBoozkA_2d4u2pjXsE4v0fovTLENqE7jS8rhl16UXX561ZmEeQvGVa-qMS2DPXOjJj2gFt2xjAiyjzdaCYcj8sMcectPCpFj7F_Ck0aPQDoQl9EP-KmgHpUkhUZQau8x1DIH97qZ4VJIY8NM3VHZFTleUkqSxfuW2XOQrWF5iw",
};

/** The fixture's key, as the page holds one. */
async function fixtureKey(): Promise<{
  privateKey: CryptoKey;
  publicKey: Uint8Array<ArrayBuffer>;
}> {
  const publicKey = fromBase64url(fixture.publicKey);
  const privateKey = await crypto.subtle.importKey(
    "jwk",
    {
      kty: "EC",
      crv: "P-256",
      d: fixture.d,
      x: btoa(String.fromCodePoint(...publicKey.slice(1, 33)))
        .replaceAll("+", "-")
        .replaceAll("/", "_")
        .replace(/=+$/u, ""),
      y: btoa(String.fromCodePoint(...publicKey.slice(33, 65)))
        .replaceAll("+", "-")
        .replaceAll("/", "_")
        .replace(/=+$/u, ""),
    },
    { name: "ECDH", namedCurve: "P-256" },
    false,
    ["deriveBits"],
  );
  return { privateKey, publicKey };
}

/**
 * The page's parts over a service that answers answers in turn, the page waiting between asks for
 * a release the test gives; what it asked, remembered and went to.
 */
function partsAnswering(answers: unknown[]): {
  parts: DeviceCodeParts;
  asked: string[];
  remembered: unknown[];
  went: string[];
  release: () => void;
} {
  const asked: string[] = [];
  const remembered: unknown[] = [];
  const went: string[] = [];
  const waiting: (() => void)[] = [];
  return {
    asked,
    remembered,
    went,
    release: () => {
      waiting.shift()?.();
    },
    parts: {
      client: {
        clientApi: "https://api.example/client/v1",
        fetch: (url: string | URL | Request): Promise<Response> => {
          const path = url instanceof Request ? url.url : url.toString();
          asked.push(path.replace("https://api.example/client/v1", ""));
          return Promise.resolve(Response.json(answers.shift()));
        },
      },
      originOf: (coreId) => (coreId === "k7xq2" ? "https://k7xq2.client.example" : undefined),
      remember: (core) => {
        remembered.push(core);
      },
      go: (address) => {
        went.push(address);
      },
      wait: () =>
        new Promise((resolve) => {
          waiting.push(resolve);
        }),
      newKey: fixtureKey,
    },
  };
}

describe("a television's sign-in by a code from the discovery's service (D293 §2)", () => {
  it("shows its code, then the check code of its key, and goes on to the core with the hand-off", async () => {
    const service = partsAnswering([
      { code: "abcd-efgh", secret: "s", expiresIn: 600 },
      { state: "waiting" },
      { state: "entered" },
      { state: "activated", sealed: fixture.sealed, bootstrapToken: "a-bootstrap" },
    ]);
    mounted = await mount(<DeviceCodeSignIn parts={service.parts} onBack={() => undefined} />);
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes("abcd-efgh"));
    expect(page.querySelector('output[aria-label="Code"]')?.textContent).toBe("abcd-efgh");
    service.release();
    await settleUntil(() => service.asked.length === 2);
    service.release();
    await settleUntil(() => page.querySelector('output[aria-label="Check code"]') !== null);
    // The core makes the same check code of this key (discovery/proof's TestDeviceCheckCode_…).
    expect(page.querySelector('output[aria-label="Check code"]')?.textContent).toBe("tasv-f9te");
    service.release();
    await settleUntil(() => service.went.length > 0);
    expect(service.asked).toEqual([
      "/device-codes",
      "/device-codes/state",
      "/device-codes/state",
      "/device-codes/state",
    ]);
    const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
    expect(service.went).toEqual([
      `https://k7xq2.client.example/${handOffFragment({
        code: "a-hand-off-code",
        bootstrapToken: "a-bootstrap",
        fingerprint,
        realm: "nas",
      })}`,
    ]);
    expect(service.remembered).toEqual([
      expect.objectContaining({ coreId: "k7xq2", fingerprint, realm: "nas" }),
    ]);
  });

  it("asks for a new code when the last one ended, and says so", async () => {
    const service = partsAnswering([
      { code: "abcd-efgh", secret: "s", expiresIn: 600 },
      { state: "ended" },
      { code: "wxyz-0123", secret: "t", expiresIn: 600 },
    ]);
    mounted = await mount(<DeviceCodeSignIn parts={service.parts} onBack={() => undefined} />);
    const page = mounted.container;
    await settleUntil(() => page.textContent.includes("abcd-efgh"));
    service.release();
    await settleUntil(() => page.textContent.includes("wxyz-0123"));
    expect(page.textContent).toContain("The last code ended, so here is a new one.");
  });

  it("says when no code could be had", async () => {
    const service = partsAnswering([]);
    const parts: DeviceCodeParts = {
      ...service.parts,
      client: {
        clientApi: "https://api.example/client/v1",
        fetch: () => Promise.resolve(new Response("", { status: 429 })),
      },
    };
    mounted = await mount(<DeviceCodeSignIn parts={parts} onBack={() => undefined} />);
    const page = mounted.container;
    await settleUntil(() => page.querySelector('[role="alert"]') !== null);
    expect(page.textContent).toContain("No code could be had just now.");
  });
});
