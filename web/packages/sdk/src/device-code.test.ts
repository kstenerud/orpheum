// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { base64url, fromBase64 } from "./discovery.ts";
import {
  deviceCheckCode,
  deviceCodeState,
  deviceCommitment,
  deviceKeyHash,
  newDeviceKey,
  newDeviceNonce,
  openBase,
  openDeviceOutcome,
  requestDeviceCode,
  revealDeviceNonce,
  type DeviceKey,
} from "./device-code.ts";

function hex(text: string): Uint8Array<ArrayBuffer> {
  return Uint8Array.from(text.match(/../gu) ?? [], (pair) => Number.parseInt(pair, 16));
}

/** A device key of the P-256 scalar d whose public point is publicKey. */
async function keyOf(
  d: Uint8Array<ArrayBuffer>,
  publicKey: Uint8Array<ArrayBuffer>,
): Promise<DeviceKey> {
  const privateKey = await crypto.subtle.importKey(
    "jwk",
    {
      kty: "EC",
      crv: "P-256",
      d: base64url(d),
      x: base64url(publicKey.slice(1, 33)),
      y: base64url(publicKey.slice(33, 65)),
    },
    { name: "ECDH", namedCurve: "P-256" },
    false,
    ["deriveBits"],
  );
  return { privateKey, publicKey };
}

// RFC 9180 A.3.1: DHKEM(P-256, HKDF-SHA256), HKDF-SHA256, AES-128-GCM, base mode, sequence 0.
const rfc = {
  info: "4f6465206f6e2061204772656369616e2055726e",
  pkRm: "04fe8c19ce0905191ebc298a9245792531f26f0cece2460639e8bc39cb7f706a826a779b4cf969b8a0e539c7f62fb3d30ad6aa8f80e30f1d128aafd68a2ce72ea0",
  skRm: "f3ce7fdae57e1a310d87f1ebbde6f328be0a99cdbcadf4d6589cf29de4b8ffd2",
  enc: "04a92719c6195d5085104f469a8b9814d5838ff72b60501e2c4466e5e67b325ac98536d7b61a1af4b78e5b7f951c0900be863c403ce65c9bfcb9382657222d18c4",
  pt: "4265617574792069732074727574682c20747275746820626561757479",
  aad: "436f756e742d30",
  ct: "5ad590bb8baa577f8619db35a36311226a896e7342a6d836d8b7bcd2f20b6c7f9076ac232e3ab2523f39513434",
};

// The outcome discovery/proof/devicecode_test.go opens too: sealed by the core's own seal.
const fixture = {
  privateKey: "xeruaKT9s4YsierbGsVdrQk8RzKuhPC2k453oBOIjJo",
  publicKey:
    "BMbJRSPCf69nfVclsx77C19vrA1JS1lRUkmpU5kSQdr53_JGKknN2W0eAMmPYZsdP_8JtRtLggy7CKKoOkhGKLg",
  sealed:
    "BBfNci1ok_9cjsv6uhX90qCwN1b0RE50DPnYo7SGlux2s80NY108R0TvHVo3KPY5KazNfFHeG4Dn2fkeKvMVP-oMf-wohuEBLdcVzhTfDBoozkA_2d4u2pjXsE4v0fovTLENqE7jS8rhl16UXX561ZmEeQvGVa-qMS2DPXOjJj2gFt2xjAiyjzdaCYcj8sMcectPCpFj7F_Ck0aPQDoQl9EP-KmgHpUkhUZQau8x1DIH97qZ4VJIY8NM3VHZFTleUkqSxfuW2XOQrWF5iw",
  // The nonces, check code, commitment and key hash discovery/proof/devicecode_test.go holds too (D321).
  deviceNonce: "AAECAwQFBgcICQoLDA0ODw",
  coreNonce: "EBESExQVFhcYGRobHB0eHw",
  checkCode: "dx9y-jewz",
  commitment: "T9zJEIeNk5Oc91YyHh_bg_HeQpq9RZ9huSJhR3SrysU",
  keyHash: "rTBVo-NI60s-RZ3ENfKqkw",
};

describe("a device code's HPKE (D293 §2)", () => {
  it("opens RFC 9180's base-mode vector for DHKEM(P-256), HKDF-SHA256 and AES-128-GCM", async () => {
    const key = await keyOf(hex(rfc.skRm), hex(rfc.pkRm));
    const opened = await openBase(key, {
      enc: hex(rfc.enc),
      ciphertext: hex(rfc.ct),
      info: hex(rfc.info),
      aad: hex(rfc.aad),
    });
    expect([...opened]).toEqual([...hex(rfc.pt)]);
  });

  it("opens what the core sealed, and nothing with another key", async () => {
    const key = await keyOf(fromBase64(fixture.privateKey), fromBase64(fixture.publicKey));
    expect(await openDeviceOutcome(key, fixture.sealed)).toEqual({
      coreId: "k7xq2",
      fingerprint: "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw",
      realm: "nas",
      handOffCode: "a-hand-off-code",
      person: "",
    });
    await expect(openDeviceOutcome(await newDeviceKey(), fixture.sealed)).rejects.toThrow();
  });

  it("makes the check code the core makes of the same key and nonces, and another of another (D321)", async () => {
    const publicKey = fromBase64(fixture.publicKey);
    expect(await deviceCheckCode(publicKey, fixture.deviceNonce, fixture.coreNonce)).toBe(
      fixture.checkCode,
    );
    const other = await newDeviceKey();
    expect(other.publicKey).toHaveLength(65);
    expect(await deviceCheckCode(other.publicKey, fixture.deviceNonce, fixture.coreNonce)).not.toBe(
      fixture.checkCode,
    );
    expect(await deviceCheckCode(publicKey, fixture.deviceNonce, fixture.deviceNonce)).not.toBe(
      fixture.checkCode,
    );
    await expect(deviceCheckCode(publicKey, "short", fixture.coreNonce)).rejects.toThrow();
  });

  it("commits to its nonce and hashes its key for the QR code as the core does (D321)", async () => {
    const publicKey = fromBase64(fixture.publicKey);
    expect(await deviceCommitment(publicKey, fixture.deviceNonce)).toBe(fixture.commitment);
    expect(await deviceKeyHash(publicKey)).toBe(fixture.keyHash);
    const nonce = newDeviceNonce();
    expect(fromBase64(nonce)).toHaveLength(16);
    expect(newDeviceNonce()).not.toBe(nonce);
  });
});

describe("a device code at the service, asked too fast", () => {
  it("stands as it stood when the service says slow down, and fails on any other refusal", async () => {
    const refusals = ["slowDown", "tooManyCodes"];
    const client = {
      clientApi: "https://api.example/client/v1",
      fetch: (): Promise<Response> =>
        Promise.resolve(Response.json({ code: refusals.shift(), message: "m" }, { status: 429 })),
    };
    expect(await deviceCodeState(client, "s")).toEqual({ state: "slowDown" });
    await expect(deviceCodeState(client, "s")).rejects.toThrow("tooManyCodes");
  });
});

describe("a device code at the service", () => {
  it("is asked for with the key's public half, and stands as the service says, anything else ended", async () => {
    const asked: { path: string; body: unknown }[] = [];
    const answers: unknown[] = [
      { code: "abcd-efgh", secret: "s", expiresInSeconds: 600 },
      { state: "entered", coreId: "k7xq2", realm: "nas" },
      { state: "entered", coreId: "k7xq2", realm: "nas", coreNonce: "EBESExQVFhcYGRobHB0eHw" },
      { state: "activated", sealed: "x", bootstrapToken: "b", coreId: "k7xq2", realm: "nas" },
      { state: "activated", coreId: "k7xq2" },
      { state: "entered" },
    ];
    const client = {
      clientApi: "https://api.example/client/v1",
      fetch: (url: string | URL | Request, init?: RequestInit): Promise<Response> => {
        const path = url instanceof Request ? url.url : url.toString();
        asked.push({ path, body: JSON.parse(typeof init?.body === "string" ? init.body : "null") });
        return Promise.resolve(Response.json(answers.shift()));
      },
    };
    const key = await newDeviceKey();
    const nonce = newDeviceNonce();
    expect(await requestDeviceCode(client, { key, nonce, platform: "Tizen" })).toEqual({
      code: "abcd-efgh",
      secret: "s",
      expiresInSeconds: 600,
    });
    // It commits to its nonce, never sending it with the ask (D321).
    expect(asked[0]).toEqual({
      path: "https://api.example/client/v1/device-codes",
      body: {
        publicKey: base64url(key.publicKey),
        platform: "Tizen",
        commitment: await deviceCommitment(key.publicKey, nonce),
      },
    });
    const claimedBy = { coreId: "k7xq2", realm: "nas" };
    // Scanned, it is entered with no core nonce; typed, with the core's.
    expect(await deviceCodeState(client, "s")).toEqual({
      state: "entered",
      claimedBy,
      coreNonce: "",
    });
    expect(await deviceCodeState(client, "s")).toEqual({
      state: "entered",
      claimedBy,
      coreNonce: "EBESExQVFhcYGRobHB0eHw",
    });
    expect(await deviceCodeState(client, "s")).toEqual({
      state: "activated",
      claimedBy,
      sealed: "x",
      bootstrapToken: "b",
    });
    // An activation without its outcome is none, and an entry naming no core is none either.
    expect(await deviceCodeState(client, "s")).toEqual({ state: "ended" });
    expect(await deviceCodeState(client, "s")).toEqual({ state: "ended" });
    expect(asked[1]?.path).toBe("https://api.example/client/v1/device-codes/state");
  });
});

describe("a device revealing its nonce (D321)", () => {
  it("posts it with its code's secret, and fails when the service refuses it", async () => {
    const asked: unknown[] = [];
    const statuses = [204, 400];
    const client = {
      clientApi: "https://api.example/client/v1",
      fetch: (_url: string | URL | Request, init?: RequestInit): Promise<Response> => {
        asked.push(JSON.parse(typeof init?.body === "string" ? init.body : "null"));
        return Promise.resolve(new Response(null, { status: statuses.shift() ?? 500 }));
      },
    };
    await revealDeviceNonce(client, "s", fixture.deviceNonce);
    expect(asked[0]).toEqual({ secret: "s", nonce: fixture.deviceNonce });
    await expect(revealDeviceNonce(client, "s", fixture.deviceNonce)).rejects.toThrow();
  });
});
