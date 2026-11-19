// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { base64url, fromBase64, NotFound } from "./discovery.ts";
import {
  isRealm,
  masterRealm,
  personKey,
  realmAsk,
  realmKey,
  realmPerson,
  realmSignIn,
  realmTag,
} from "./realm.ts";

// Vectors the Go code made (discovery/proof/realm_test.go), the key checked against Python's hashlib,
// over the Ed25519 key of the seed 0x00…0x1f, so the page and the core derive each value alike.
const vectors = {
  key: "2mjAagkzCF_kAZaUObGZixfFrcDG-39cUXwxtK2hxj0",
  person: "UpaJxwoFfxYyPW1HZo-g-HyN54gftFFBxpa2o5u28UQ",
  ask: "KxsLH9QDpNeduWRb9ufhBI1iEY3jLd8igLHF5TZ6nRk",
  tag: "oelueZOrkKIG510vkkAhNZ7K7M10Fj1Le9sO6mIs3J4",
  publicKey: "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg",
  // The version-2 key over the key salt 0x00…0x0f, and the ask and the tag under it (D527 §2.2).
  keySalt: "AAECAwQFBgcICQoLDA0ODw",
  personKey: "uv43RZGP775xAJg7IrZFqKEqHkdSN1lvwTPy4CUOXZY",
  personAsk: "4XnMVtARnI--WRxjOJD3mkqo9STHA91iFFCq4tCP0LA",
  personTag: "M30ogOTBStG74ad_v5tyPL9CiJkMGQFpB9iisKZEWSQ",
};

describe("a realm sign-in's values, as the Go code makes them", () => {
  it("derives the key, the person, the ask and the tag byte for byte, whatever the case typed", async () => {
    const key = await realmKey(" NAS ", "Karl", "a password");
    expect(base64url(key)).toBe(vectors.key);
    expect(await realmPerson("nas", "karl")).toBe(vectors.person);
    expect(await realmAsk(key, "nas", vectors.person)).toBe(vectors.ask);
    expect(await realmTag(key, "k7xq2", fromBase64(vectors.publicKey))).toBe(vectors.tag);
  });

  it("derives the version-2 key by the key salt and the name, and makes the ask and tag under it", async () => {
    const key = await personKey(fromBase64(vectors.keySalt), " Karl ", "a password");
    expect(base64url(key)).toBe(vectors.personKey);
    expect(await realmAsk(key, "nas", vectors.person)).toBe(vectors.personAsk);
    expect(await realmTag(key, "k7xq2", fromBase64(vectors.publicKey))).toBe(vectors.personTag);
  });

  it("salts the key by the master realm alone, and keeps a realm's rule", async () => {
    expect(base64url(await realmKey("nas.live", "karl", "a password"))).toBe(vectors.key);
    expect(masterRealm("KarlNAS.Live")).toBe("karlnas");
    for (const realm of ["karlnas", "karlnas.test", "karlnas.t", "the-smiths-2.living-room"]) {
      expect(isRealm(realm)).toBe(true);
    }
    for (const realm of [
      "nas",
      "nas.test",
      "karlnas.",
      "karlnas.a.b",
      "Karlnas",
      "-karlnas",
      "karl nas",
    ]) {
      expect(isRealm(realm)).toBe(false);
    }
  });
});

describe("a realm sign-in through a discovery", () => {
  // A discovery answering a sign-in by answer and the salt asked by salt: by default a 404, as a
  // discovery that gives none does.
  const discoveryAnswering =
    (
      answer: (asked: unknown) => [number, unknown],
      heard: unknown[] = [],
      salt: () => [number, unknown] = () => [404, {}],
    ): typeof fetch =>
    (url, init) => {
      const asked: unknown = JSON.parse(typeof init?.body === "string" ? init.body : "null");
      heard.push(asked);
      const address = url instanceof Request ? url.url : url.toString();
      const [status, body] = address.endsWith("/realm-salt") ? salt() : answer(asked);
      return Promise.resolve(new Response(JSON.stringify(body), { status }));
    };
  const coreAnswer = (tag: string): unknown => ({
    coreId: "k7xq2",
    bootstrapToken: "a-token",
    iceServers: [],
    paths: [],
    publicKey: vectors.publicKey,
    tag,
  });
  const askOf = (asked: unknown): unknown =>
    typeof asked === "object" && asked !== null ? Reflect.get(asked, "ask") : undefined;

  it("asks with the person and the ask, and takes the core whose tag holds under the key", async () => {
    const heard: unknown[] = [];
    const key = await realmKey("nas", "karl", "a password");
    const tag = await realmTag(key, "k7xq2", fromBase64(vectors.publicKey));
    const found = await realmSignIn(
      {
        clientApi: "https://discovery.example/client/v1",
        fetch: discoveryAnswering(
          () => [
            200,
            {
              coreId: "k7xq2",
              bootstrapToken: "a-token",
              iceServers: [],
              paths: [],
              publicKey: vectors.publicKey,
              tag,
            },
          ],
          heard,
        ),
      },
      { realm: " NAS ", name: "Karl", password: "a password" },
    );
    expect(heard).toEqual([
      { realm: "nas" },
      { realm: "nas", person: vectors.person, ask: vectors.ask },
    ]);
    expect(found.located).toMatchObject({ coreId: "k7xq2", bootstrapToken: "a-token" });
    expect(found.fingerprint).toBe("Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw");
    expect(base64url(found.key)).toBe(vectors.key);
  });

  it("believes no key whose tag does not hold, and says not found when the discovery does", async () => {
    const posing = {
      clientApi: "https://discovery.example/client/v1",
      fetch: discoveryAnswering(() => [
        200,
        {
          coreId: "k7xq2",
          bootstrapToken: "a-token",
          iceServers: [],
          paths: [],
          publicKey: vectors.publicKey,
          tag: "made up",
        },
      ]),
    };
    await expect(
      realmSignIn(posing, { realm: "nas", name: "karl", password: "a password" }),
    ).rejects.toThrow("not the server's own");
    const refusing = {
      clientApi: "https://discovery.example/client/v1",
      fetch: discoveryAnswering(() => [404, {}]),
    };
    await expect(
      realmSignIn(refusing, { realm: "nas", name: "karl", password: "a guess" }),
    ).rejects.toBeInstanceOf(NotFound);
  });

  it("signs in under the version-2 key first, salted as the discovery says, and derives no other", async () => {
    const heard: unknown[] = [];
    const v2 = await personKey(fromBase64(vectors.keySalt), "karl", "a password");
    const tag = await realmTag(v2, "k7xq2", fromBase64(vectors.publicKey));
    const found = await realmSignIn(
      {
        clientApi: "https://discovery.example/client/v1",
        fetch: discoveryAnswering(
          (asked) => (askOf(asked) === vectors.personAsk ? [200, coreAnswer(tag)] : [404, {}]),
          heard,
          () => [200, { salt: vectors.keySalt }],
        ),
      },
      { realm: "nas", name: "Karl", password: "a password" },
    );
    expect(heard).toEqual([
      { realm: "nas" },
      { realm: "nas", person: vectors.person, ask: vectors.personAsk },
    ]);
    expect(base64url(found.key)).toBe(vectors.personKey);
  });

  it("tries the version-1 key once the version-2 key is refused, and is not found when both are", async () => {
    const heard: unknown[] = [];
    const v1 = await realmKey("nas", "karl", "a password");
    const tag = await realmTag(v1, "k7xq2", fromBase64(vectors.publicKey));
    const salted = (): [number, unknown] => [200, { salt: vectors.keySalt }];
    const found = await realmSignIn(
      {
        clientApi: "https://discovery.example/client/v1",
        fetch: discoveryAnswering(
          (asked) => (askOf(asked) === vectors.ask ? [200, coreAnswer(tag)] : [404, {}]),
          heard,
          salted,
        ),
      },
      { realm: "nas", name: "karl", password: "a password" },
    );
    expect(heard.map(askOf)).toEqual([undefined, vectors.personAsk, vectors.ask]);
    expect(base64url(found.key)).toBe(vectors.key);
    const refusingBoth: unknown[] = [];
    await expect(
      realmSignIn(
        {
          clientApi: "https://discovery.example/client/v1",
          fetch: discoveryAnswering(() => [404, {}], refusingBoth, salted),
        },
        { realm: "nas", name: "karl", password: "a guess" },
      ),
    ).rejects.toBeInstanceOf(NotFound);
    expect(refusingBoth).toHaveLength(3);
  });
});
