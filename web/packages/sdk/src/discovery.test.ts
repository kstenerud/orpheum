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
} from "./discovery.ts";

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
});
