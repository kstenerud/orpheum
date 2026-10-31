// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  answerMessage,
  base64url,
  discoverySignaller,
  fingerprintOf,
  fromBase64,
  keyFingerprint,
  locateHash,
  locatorHash,
  offerKey,
  offerProof,
  readLinkFragment,
  sessionOfferKey,
  verifyAnswer,
  verifyClientManifest,
  handOffOfferKey,
  isCoreOffline,
  isKeyRefused,
  KeyRefused,
  locatedOf,
} from "./discovery.ts";
import { isUnreached } from "./rtc-reopen.ts";

// Vectors the Go code made: discovery/link and discovery/proof, with an Ed25519 key from the seed
// 0x00…0x1f, so the client and the core derive each value alike.
const vectors = {
  locateHash: "78WLMh8Uh58_XQXyTJxtoB97Y60Hqi9RyMof0zZCOl8",
  offerKey: "q-y6-NDmglfskpF1-CWJh_z-6IJCiusrrl4Bq0Ir0js",
  locatorHash: "3kubJAv9R5BgDSLi_nCmmy9qLM4u4LxEfI5sYSu9CU0",
  sessionOfferKey: "4zFuK6_rthA6NBjod4wK3JYNQCE6A6fn9PzTQm7YB78",
  offerProof: "y69uw60UMMyrcCgc4DuIB5TgNJTYeQhFvq_p67KD6s8",
  publicKey: "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg",
  fingerprint: "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw",
  answerSignature:
    "zPjLjCmzDvVqHdAZSpOC8WniJI6O_7YspYaS3m4jADbYVFqR7XuBosDQ7V5iD9XEtR-e9abWmBjGPJqid-ofAQ",
  // proof's clientManifestVector, over the manifest {"files":[]}, and handOffOfferKeyVector.
  clientManifestSignature:
    "UoKlrdFrAoHSyIoY1izKxSjrqGmsqj7KVvSJQWAvuhtvCYzkVwJaI_cag2Lrv9Q3h7gt7rF87XmmPLcmSaPeDQ",
  handOffOfferKey: "4kr2Om59JFSPErbW1mQ4dnJuRTQiDyeFHb7U_kPBfxI",
};
const answer = {
  coreId: "k7xq2",
  offerId: "1",
  offerFingerprint: "sha-256 AB:CD",
  sdp: "v=0\r\na=fingerprint:sha-256 99:88\r\n",
};

describe("the discovery's derivations, as the Go code makes them", () => {
  it("hashes a link's secret, a locator and a token, and proves an offer, byte for byte", async () => {
    expect(await locateHash("abc_DEF-123")).toBe(vectors.locateHash);
    expect(await locatorHash("a-locator")).toBe(vectors.locatorHash);
    expect(base64url(await offerKey("abc_DEF-123"))).toBe(vectors.offerKey);
    expect(base64url(await sessionOfferKey("a token"))).toBe(vectors.sessionOfferKey);
    expect(await offerProof(await offerKey("abc_DEF-123"), "sha-256 AB:CD")).toBe(
      vectors.offerProof,
    );
  });

  it("checks the core's answer by a key whose fingerprint the client holds, and nothing else", async () => {
    const publicKey = fromBase64(vectors.publicKey);
    const signature = fromBase64(vectors.answerSignature);
    expect(await keyFingerprint(publicKey)).toBe(vectors.fingerprint);
    expect(answerMessage(answer)).toBe(
      "orpheum-discovery-v1:answer\nk7xq2\n1\nsha-256 AB:CD\nv=0\r\na=fingerprint:sha-256 99:88\r\n",
    );
    const checked = { publicKey, fingerprint: vectors.fingerprint, answer, signature };
    expect(await verifyAnswer(checked)).toBe(true);
    expect(await verifyAnswer({ ...checked, answer: { ...answer, sdp: "v=1" } })).toBe(false);
    expect(await verifyAnswer({ ...checked, fingerprint: "another-fingerprint" })).toBe(false);
  });

  it("checks the core's client manifest by a key whose fingerprint the client holds, and derives a hand-off's offer key (D308)", async () => {
    const publicKey = fromBase64(vectors.publicKey);
    const manifest = new TextEncoder().encode('{"files":[]}');
    const checked = {
      publicKey,
      fingerprint: vectors.fingerprint,
      manifest,
      signature: fromBase64(vectors.clientManifestSignature),
    };
    expect(await verifyClientManifest(checked)).toBe(true);
    expect(
      await verifyClientManifest({
        ...checked,
        manifest: new TextEncoder().encode('{"files":[1]}'),
      }),
    ).toBe(false);
    expect(await verifyClientManifest({ ...checked, fingerprint: "another-fingerprint" })).toBe(
      false,
    );
    expect(base64url(await handOffOfferKey("a hand-off code"))).toBe(vectors.handOffOfferKey);
  });

  it("reads a link's fragment, and nothing that is not one", () => {
    expect(readLinkFragment(`#k7xq2.s3cret.${vectors.fingerprint}`)).toEqual({
      coreId: "k7xq2",
      secret: "s3cret",
      fingerprint: vectors.fingerprint,
    });
    for (const text of [
      "",
      "#secret",
      "#core.secret",
      `#core.sec ret.${vectors.fingerprint}`,
      "#a.b.short",
    ]) {
      expect(readLinkFragment(text)).toBeUndefined();
    }
    expect(fingerprintOf("v=0\r\na=fingerprint:sha-256 AB:CD\r\n")).toBe("sha-256 AB:CD");
  });
});

describe("a signaller through a discovery", () => {
  const offer = { type: "offer" as const, sdp: "v=0\r\na=fingerprint:sha-256 AB:CD\r\n" };
  const answering =
    (answered: Record<string, unknown>, heard: unknown[] = []): typeof fetch =>
    (_url, init) => {
      heard.push(JSON.parse(typeof init?.body === "string" ? init.body : "null"));
      return Promise.resolve(
        new Response(JSON.stringify({ answers: [answered] }), { status: 200 }),
      );
    };
  const signalling = async (post: typeof fetch) =>
    discoverySignaller({
      clientApi: "https://discovery.example/client/v1",
      coreId: "k7xq2",
      fingerprint: vectors.fingerprint,
      held: { bootstrapToken: "a-token" },
      key: await offerKey("abc_DEF-123"),
      fetch: post,
    });

  it("sends the offer with its proof and takes the core's signed answer", async () => {
    const heard: unknown[] = [];
    const signaller = await signalling(
      answering(
        {
          offerId: "1",
          sdp: answer.sdp,
          signature: vectors.answerSignature,
          publicKey: vectors.publicKey,
        },
        heard,
      ),
    );
    await expect(signaller(offer)).resolves.toEqual({ type: "answer", sdp: answer.sdp });
    expect(heard).toEqual([
      { bootstrapToken: "a-token", offers: [{ sdp: offer.sdp, proof: vectors.offerProof }] },
    ]);
  });

  it("sends only the candidates a core outside the client's network could use", async () => {
    const heard: unknown[] = [];
    const signaller = await signalling(answering({ offerId: "1", refusal: "a test's" }, heard));
    const withLocal = {
      type: "offer" as const,
      sdp: `${offer.sdp}a=candidate:1 1 udp 2122260223 192.168.1.20 50000 typ host\r\n`,
    };
    await expect(signaller(withLocal)).rejects.toThrow("a test's");
    expect(heard).toEqual([
      { bootstrapToken: "a-token", offers: [{ sdp: offer.sdp, proof: vectors.offerProof }] },
    ]);
  });

  it("proves a later offer with what the device holds by then, not the spent link", async () => {
    const heard: unknown[] = [];
    const post = answering({ offerId: "1", refusal: "a test's" }, heard);
    const later = await sessionOfferKey("a membership credential");
    const signaller = discoverySignaller({
      clientApi: "https://discovery.example/client/v1",
      coreId: "k7xq2",
      fingerprint: vectors.fingerprint,
      held: { bootstrapToken: "a-token" },
      key: await offerKey("abc_DEF-123"),
      latest: () => Promise.resolve({ held: { locator: "a-locator" }, key: later }),
      fetch: post,
    });
    await expect(signaller(offer)).rejects.toThrow("a test's");
    expect(heard).toEqual([
      {
        locator: "a-locator",
        offers: [{ sdp: offer.sdp, proof: await offerProof(later, "sha-256 AB:CD") }],
      },
    ]);
  });

  // The owner's Firefox, signed in and then a Watch Together guest by the host's link, through
  // staging's discovery: every offer after the join was refused under the spent link's key, for good
  // (2026-10-29).
  describe("an offer the core refuses as unproven", () => {
    const refusal = "the core refused the offer: the offer's proof does not hold";
    const signed = {
      offerId: "1",
      sdp: answer.sdp,
      signature: vectors.answerSignature,
      publicKey: vectors.publicKey,
    };
    /** A discovery that refuses every offer proven by refused, and answers the rest. */
    const refusingUnder =
      (refused: string, heard: unknown[]): typeof fetch =>
      (_url, init) => {
        const body: unknown = JSON.parse(typeof init?.body === "string" ? init.body : "null");
        heard.push(body);
        const offers: unknown = Reflect.get(body as object, "offers");
        const proof: unknown = Array.isArray(offers)
          ? Reflect.get(offers[0] as object, "proof")
          : "";
        const answered = proof === refused ? { offerId: "1", refusal } : signed;
        return Promise.resolve(new Response(JSON.stringify({ answers: [answered] })));
      };

    it("is sent once more under what the device holds now, which proves the offers after", async () => {
      const linkKey = await offerKey("abc_DEF-123");
      const session = await sessionOfferKey("a session token");
      const heard: unknown[] = [];
      let fallbacks = 0;
      const signaller = discoverySignaller({
        clientApi: "https://discovery.example/client/v1",
        coreId: "k7xq2",
        fingerprint: vectors.fingerprint,
        held: { bootstrapToken: "a-token" },
        key: linkKey,
        fallback: () => {
          fallbacks += 1;
          return Promise.resolve({ held: { locator: "a-locator" }, key: session });
        },
        fetch: refusingUnder(await offerProof(linkKey, "sha-256 AB:CD"), heard),
      });
      await expect(signaller(offer)).resolves.toEqual({ type: "answer", sdp: answer.sdp });
      await expect(signaller(offer)).resolves.toEqual({ type: "answer", sdp: answer.sdp });
      const sessionProof = await offerProof(session, "sha-256 AB:CD");
      expect(heard).toEqual([
        { bootstrapToken: "a-token", offers: [{ sdp: offer.sdp, proof: vectors.offerProof }] },
        { locator: "a-locator", offers: [{ sdp: offer.sdp, proof: sessionProof }] },
        // The next offer goes under what was answered, the spent link's key tried no more.
        { locator: "a-locator", offers: [{ sdp: offer.sdp, proof: sessionProof }] },
      ]);
      expect(fallbacks).toBe(1);
    });

    it("is the device's key refused once what it holds now is refused too, asked for once", async () => {
      const linkKey = await offerKey("abc_DEF-123");
      const heard: unknown[] = [];
      let fallbacks = 0;
      const signaller = discoverySignaller({
        clientApi: "https://discovery.example/client/v1",
        coreId: "k7xq2",
        fingerprint: vectors.fingerprint,
        held: { bootstrapToken: "a-token" },
        key: linkKey,
        fallback: async () => {
          fallbacks += 1;
          return { held: { locator: "a-locator" }, key: await sessionOfferKey("a revoked one") };
        },
        // Every offer refused, whatever proves it.
        fetch: (_url, init) => {
          heard.push(init?.body);
          return Promise.resolve(
            new Response(JSON.stringify({ answers: [{ offerId: "1", refusal }] })),
          );
        },
      });
      const problem: unknown = await signaller(offer).catch((caught: unknown) => caught);
      expect(problem).toBeInstanceOf(KeyRefused);
      expect(heard).toHaveLength(2);
      expect(fallbacks).toBe(1);
      // Nothing else held: the one refusal is the answer.
      const alone = discoverySignaller({
        clientApi: "https://discovery.example/client/v1",
        coreId: "k7xq2",
        fingerprint: vectors.fingerprint,
        held: { bootstrapToken: "a-token" },
        key: linkKey,
        fallback: () => Promise.resolve({ held: { bootstrapToken: "a-token" }, key: linkKey }),
        fetch: (_url, init) => {
          heard.push(init?.body);
          return Promise.resolve(
            new Response(JSON.stringify({ answers: [{ offerId: "1", refusal }] })),
          );
        },
      });
      await expect(alone(offer)).rejects.toBeInstanceOf(KeyRefused);
      expect(heard).toHaveLength(3);
    });
  });

  it("refuses an answer the core did not sign, or signed with a key its link does not name", async () => {
    const forged = await signalling(
      answering({
        offerId: "1",
        sdp: "v=0 the discovery's own",
        signature: vectors.answerSignature,
        publicKey: vectors.publicKey,
      }),
    );
    await expect(forged(offer)).rejects.toThrow("not the server's own");
    const refused = await signalling(
      answering({ offerId: "1", refusal: "the offer's proof does not hold" }),
    );
    await expect(refused(offer)).rejects.toThrow("proof does not hold");
  });

  it("tells a core that refused the device's key from a network that failed, its reason kept (the owner's Fire TV, 2026-10-28)", async () => {
    // As connectd and discovery/single pass the core's refusal on: ErrRefused, then the core's reason.
    const refusal = "the core refused the offer: the offer's proof does not hold";
    const refused = await signalling(answering({ offerId: "1", refusal }));
    const problem: unknown = await refused(offer).catch((caught: unknown) => caught);
    expect(problem).toBeInstanceOf(KeyRefused);
    expect(isKeyRefused(new AggregateError([new Error("a name", { cause: problem })]))).toBe(true);
    expect(isCoreOffline(problem)).toBe(false);
    expect(problem instanceof Error ? problem.message : "").toContain(refusal);
    // Any other refusal is no key refused.
    const busy = await signalling(
      answering({
        offerId: "1",
        refusal: "the core refused the offer: the core refused the offer",
      }),
    );
    expect(isKeyRefused(await busy(offer).catch((caught: unknown) => caught))).toBe(false);
  });

  it("fails an offer whose post reached no server as Unreached, and an aborted one as its abort", async () => {
    const noNetwork = await signalling(() =>
      Promise.reject(new TypeError("NetworkError when attempting to fetch resource.")),
    );
    const problem: unknown = await noNetwork(offer).catch((caught: unknown) => caught);
    expect(isUnreached(problem)).toBe(true);
    expect(isKeyRefused(problem)).toBe(false);
    const aborted = await signalling(() =>
      Promise.reject(new DOMException("The operation was aborted.", "AbortError")),
    );
    expect(isUnreached(await aborted(offer).catch((caught: unknown) => caught))).toBe(false);
    // A post that reached the discovery, which answered badly, reached a server.
    const bad = await signalling(() => Promise.resolve(new Response("", { status: 502 })));
    expect(isUnreached(await bad(offer).catch((caught: unknown) => caught))).toBe(false);
  });
});

describe("the paths a discovery's answer names", () => {
  it("are its direct names in its order, the LAN's told apart, and nothing it does not know or another core's", () => {
    const located = locatedOf({
      coreId: "k7xq2",
      paths: [
        { kind: "lan", url: "https://192-168-1-2.k7xq2.zone.example:8443" },
        { kind: "ipv6", url: "https://2a00-1.k7xq2.zone.example" },
        { kind: "webrtc", offer: "/cores/k7xq2/offer" },
        { kind: "lan", url: "http://plain.example" },
        // Another core's name, and a host under no core's: never a path to this one (H1).
        { kind: "ipv4", url: "https://v4.m3dz9.zone.example" },
        { kind: "ipv4", url: "https://k7xq2.example" },
      ],
    });
    expect(located.directUrls).toEqual([
      "https://192-168-1-2.k7xq2.zone.example:8443",
      "https://2a00-1.k7xq2.zone.example",
    ]);
    expect(located.lanUrls).toEqual(["https://192-168-1-2.k7xq2.zone.example:8443"]);
  });
});
