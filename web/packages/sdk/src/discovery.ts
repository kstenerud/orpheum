// SPDX-License-Identifier: MPL-2.0

/**
 * The client's side of the discovery protocol (docs/contributors/design/discovery-protocol.md §2.5,
 * §3): reading a link, the hashes a discovery finds a core by, an offer's proof, and the core's signed
 * answer checked against the link's fingerprint. Each derivation is the Go one in
 * discovery/link and discovery/proof, byte for byte; the tests hold vectors the Go code made.
 */

import type { Description, Signaller } from "./rtc-transport.ts";
import { offerThroughDiscovery } from "./relayed-offer.ts";

/** A core's link's fragment: `<coreId>.<secret>.<fingerprint>`, after `#`. */
export interface LinkFragment {
  readonly coreId: string;
  readonly secret: string;
  readonly fingerprint: string;
}

/** A fingerprint's length: SHA-256 in unpadded base64url. */
const fingerprintChars = 43;

const base64urlPart = /^[A-Za-z0-9_-]+$/u;

/** Reads a link's fragment, with or without its `#`; undefined when it is not a core's link's. */
export function readLinkFragment(text: string): LinkFragment | undefined {
  const parts = text.replace(/^#/u, "").split(".");
  if (parts.length !== 3 || parts.some((part) => !base64urlPart.test(part))) {
    return undefined;
  }
  const [coreId = "", secret = "", fingerprint = ""] = parts;
  return fingerprint.length === fingerprintChars ? { coreId, secret, fingerprint } : undefined;
}

/** Bytes as unpadded base64url. */
export function base64url(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCodePoint(byte);
  }
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/u, "");
}

/** Unpadded base64url, or the standard base64 Go's JSON gives []byte, as bytes. */
export function fromBase64(text: string): Uint8Array<ArrayBuffer> {
  const standard = text.replaceAll("-", "+").replaceAll("_", "/");
  const binary = atob(standard + "=".repeat((4 - (standard.length % 4)) % 4));
  return Uint8Array.from(binary, (char) => char.codePointAt(0) ?? 0);
}

const encoder = new TextEncoder();

async function sha256(text: string | Uint8Array<ArrayBuffer>): Promise<Uint8Array<ArrayBuffer>> {
  const bytes = typeof text === "string" ? encoder.encode(text) : text;
  return new Uint8Array(await crypto.subtle.digest("SHA-256", bytes));
}

/** The hash a discovery locates a core by for a link's secret: link.LocateHash. */
export async function locateHash(secret: string): Promise<string> {
  return base64url(await sha256(`orpheum-locate${secret}`));
}

/** The hash a discovery finds a core by for a device's locator: link.LocatorHash. */
export async function locatorHash(locator: string): Promise<string> {
  return base64url(await sha256(`orpheum-locator${locator}`));
}

/** The key an offer made by a link's bootstrap is proven under: link.OfferKey. */
export function offerKey(secret: string): Promise<Uint8Array<ArrayBuffer>> {
  return sha256(`orpheum-offer${secret}`);
}

/** The key an offer made by a signed-in client is proven under: proof.SessionOfferKey. */
export function sessionOfferKey(token: string): Promise<Uint8Array<ArrayBuffer>> {
  return sha256(token);
}

const prefix = "orpheum-discovery-v1:";

/** An offer's proof, an HMAC over its DTLS fingerprint under key: proof.Offer. */
export async function offerProof(
  key: Uint8Array<ArrayBuffer>,
  fingerprint: string,
): Promise<string> {
  const hmac = await crypto.subtle.importKey("raw", key, { name: "HMAC", hash: "SHA-256" }, false, [
    "sign",
  ]);
  const mac = await crypto.subtle.sign(
    "HMAC",
    hmac,
    encoder.encode(`${prefix}offer\n${fingerprint}`),
  );
  return base64url(new Uint8Array(mac));
}

/** The DTLS fingerprint an SDP carries, its first a=fingerprint value: rtcapi.FingerprintOf. */
export function fingerprintOf(sdp: string): string {
  for (const line of sdp.replaceAll("\r\n", "\n").split("\n")) {
    if (line.startsWith("a=fingerprint:")) {
      return line.slice("a=fingerprint:".length);
    }
  }
  return "";
}

/** An identity key's fingerprint, SHA-256 in base64url: proof.Fingerprint. */
export async function keyFingerprint(publicKey: Uint8Array<ArrayBuffer>): Promise<string> {
  return base64url(await sha256(publicKey));
}

/** A WebRTC answer as the core signs it: proof.Answer. */
export interface SignedAnswer {
  readonly coreId: string;
  readonly offerId: string;
  readonly offerFingerprint: string;
  readonly sdp: string;
}

/** What the core's signature over an answer signs: proof.Answer's Message. */
export function answerMessage(answer: SignedAnswer): string {
  return `${prefix}answer\n${answer.coreId}\n${answer.offerId}\n${answer.offerFingerprint}\n${answer.sdp}`;
}

/** A signed answer to check: the core's key as the answer carried it, and the fingerprint held. */
export interface AnswerToCheck {
  readonly publicKey: Uint8Array<ArrayBuffer>;
  /** The fingerprint the client holds: its link's, or the key it remembers. */
  readonly fingerprint: string;
  readonly answer: SignedAnswer;
  readonly signature: Uint8Array<ArrayBuffer>;
}

/**
 * Whether the signature is the core's over the answer, by the key it carried, which is taken only
 * when its fingerprint is the one the client holds (the protocol's §2.5 item 4; A7, A9).
 */
export async function verifyAnswer(checked: AnswerToCheck): Promise<boolean> {
  if ((await keyFingerprint(checked.publicKey)) !== checked.fingerprint) {
    return false;
  }
  try {
    const key = await crypto.subtle.importKey(
      "raw",
      checked.publicKey,
      { name: "Ed25519" },
      false,
      ["verify"],
    );
    return await crypto.subtle.verify(
      "Ed25519",
      key,
      checked.signature,
      encoder.encode(answerMessage(checked.answer)),
    );
  } catch {
    // A key this browser cannot import, or one that is no Ed25519 key, verifies nothing.
    return false;
  }
}

/** What a device holds that a discovery finds its core by: its locator, or a bootstrap token. */
export type Held = { readonly locator: string } | { readonly bootstrapToken: string };

/** What an offer is sent and proven with: what the device holds, and the key its proof is under. */
export interface Proving {
  readonly held: Held;
  readonly key: Uint8Array<ArrayBuffer>;
}

/** The first answer the discovery's JSON carries, as far as it is an answer; undefined when not. */
interface OfferAnswer {
  readonly offerId: string;
  readonly sdp: string;
  readonly signature: string;
  readonly publicKey: string;
}

/** The first item of a discovery's offer response's answers, whatever it is. */
function firstOf(body: unknown): unknown {
  const answers: unknown =
    typeof body === "object" && body !== null ? Reflect.get(body, "answers") : undefined;
  return Array.isArray(answers) ? answers[0] : undefined;
}

/** The first answer in a discovery's offer response, or why there is none. */
function firstAnswer(body: unknown): OfferAnswer | string {
  const first = firstOf(body);
  if (typeof first !== "object" || first === null) {
    return "no answer";
  }
  const field = (name: string): string | undefined => {
    const value: unknown = Reflect.get(first, name);
    return typeof value === "string" ? value : undefined;
  };
  const [offerId, sdp, signature, publicKey] = ["offerId", "sdp", "signature", "publicKey"].map(
    field,
  );
  if (
    offerId === undefined ||
    sdp === undefined ||
    signature === undefined ||
    publicKey === undefined
  ) {
    return field("refusal") ?? "no answer";
  }
  return { offerId, sdp, signature, publicKey };
}

/** Where a signaller posts its offers, and how it proves and checks them. */
export interface DiscoverySignalling {
  /** The discovery's client API, `<base>/client/v1`. */
  readonly clientApi: string;
  readonly coreId: string;
  /** The identity key's fingerprint the client holds: its link's, or the key it remembers. */
  readonly fingerprint: string;
  readonly held: Held;
  /** The key the offers are proven under: the link's offer key, or the session's. */
  readonly key: Uint8Array<ArrayBuffer>;
  /**
   * What the device holds now, asked at each offer, when it holds more than it began with: a link's
   * key proves only the offer that spends it, so once its person joined or signed in, the
   * connections opened after, and any that replace one gone, are proven by the membership or session
   * the core gave them. Undefined, or answering undefined, keeps held and key.
   */
  readonly latest?: (() => Promise<Proving | undefined>) | undefined;
  /** How the signaller posts: the browser's fetch, unless a test gives another. */
  readonly fetch?: typeof fetch | undefined;
}

/** The protocol version every call carries (discoveryproto.ProtocolVersion). */
export const protocolVersion = "1.0";

/**
 * A signaller that passes each offer to the core through its discovery, proven under the key, and
 * takes only an answer the core signed, by the key whose fingerprint the client holds; any other is
 * refused, so a discovery that relays the offer cannot answer it itself (the protocol's §2.5).
 */
export function discoverySignaller(signalling: DiscoverySignalling): Signaller {
  const post = signalling.fetch ?? fetch;
  return async (offer: Description): Promise<Description> => {
    const { held, key } = (await signalling.latest?.()) ?? signalling;
    const fingerprint = fingerprintOf(offer.sdp);
    const proof = await offerProof(key, fingerprint);
    const response = await post(`${signalling.clientApi}/cores/${signalling.coreId}/offer`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Orpheum-Discovery-Protocol": protocolVersion,
      },
      body: JSON.stringify({
        ...held,
        offers: [{ sdp: offerThroughDiscovery(offer.sdp), proof }],
      }),
    });
    if (!response.ok) {
      throw new Error(`the discovery answered ${String(response.status)}`);
    }
    const answered = firstAnswer(await response.json());
    if (typeof answered === "string") {
      throw new Error(`the server refused the offer: ${answered}`);
    }
    const isTheCores = await verifyAnswer({
      publicKey: fromBase64(answered.publicKey),
      fingerprint: signalling.fingerprint,
      answer: {
        coreId: signalling.coreId,
        offerId: answered.offerId,
        offerFingerprint: fingerprint,
        sdp: answered.sdp,
      },
      signature: fromBase64(answered.signature),
    });
    if (!isTheCores) {
      throw new Error(
        "the answer is not the server's own: its key or signature does not match its link",
      );
    }
    return { type: "answer", sdp: answered.sdp };
  };
}

/** The discovery finds no core by what the device holds: never registered, used up or ended. */
export class NotFound extends Error {
  constructor() {
    super("the discovery finds no server by this");
    this.name = "NotFound";
  }
}

/** A call to a discovery's client API: where, what, and how to post. */
export interface ClientCall {
  readonly clientApi: string;
  readonly path: string;
  readonly body: unknown;
  readonly fetch?: typeof fetch | undefined;
}

/** Posts a call's body and returns its answer's JSON; NotFound when the discovery finds no core. */
export async function postToDiscovery(call: ClientCall): Promise<unknown> {
  const send = call.fetch ?? fetch;
  const response = await send(`${call.clientApi}${call.path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Orpheum-Discovery-Protocol": protocolVersion },
    body: JSON.stringify(call.body),
  });
  if (response.status === 404) {
    throw new NotFound();
  }
  if (!response.ok) {
    throw new Error(`the discovery answered ${String(response.status)}`);
  }
  if (response.status === 204) {
    return undefined;
  }
  const answer: unknown = await response.json();
  return answer;
}

/** Where the core is, as a discovery's locate or paths answers. */
export interface Located {
  readonly coreId: string;
  readonly iceServers: RTCIceServer[];
  /** The core's published names, LAN, IPv6 then IPv4, as HTTPS URLs: tried before WebRTC (§3.3). */
  readonly directUrls: readonly string[];
  /** Locate's alone: good for paths and offers for a few minutes, never renewed. */
  readonly bootstrapToken?: string;
}

/** An ICE server as the discovery names it, or none when it is not one. */
function iceServerOf(named: unknown): RTCIceServer[] {
  if (typeof named !== "object" || named === null) {
    return [];
  }
  const urls: unknown = Reflect.get(named, "urls");
  const username: unknown = Reflect.get(named, "username");
  const credential: unknown = Reflect.get(named, "credential");
  if (!Array.isArray(urls) || !urls.every((url): url is string => typeof url === "string")) {
    return [];
  }
  return [
    {
      urls,
      ...(typeof username === "string" ? { username } : {}),
      ...(typeof credential === "string" ? { credential } : {}),
    },
  ];
}

/** The kinds of path a direct URL names (the protocol's §3.2). */
const directKinds = new Set(["lan", "ipv6", "ipv4"]);

/** The direct paths' URLs a discovery's answer names, in its order; anything else is left out. */
function directUrlsOf(paths: unknown): string[] {
  if (!Array.isArray(paths)) {
    return [];
  }
  return paths.flatMap((path: unknown) => {
    if (typeof path !== "object" || path === null) {
      return [];
    }
    const kind: unknown = Reflect.get(path, "kind");
    const url: unknown = Reflect.get(path, "url");
    return typeof kind === "string" &&
      directKinds.has(kind) &&
      typeof url === "string" &&
      url.startsWith("https://")
      ? [url]
      : [];
  });
}

export function locatedOf(answer: unknown): Located {
  const field = (name: string): unknown =>
    typeof answer === "object" && answer !== null ? Reflect.get(answer, name) : undefined;
  const coreId = field("coreId");
  const iceServers = field("iceServers");
  const bootstrapToken = field("bootstrapToken");
  if (typeof coreId !== "string") {
    throw new Error("the discovery's answer names no server");
  }
  return {
    coreId,
    iceServers: Array.isArray(iceServers) ? iceServers.flatMap(iceServerOf) : [],
    directUrls: directUrlsOf(field("paths")),
    ...(typeof bootstrapToken === "string" ? { bootstrapToken } : {}),
  };
}

/** Where to ask, and how: a discovery's client API, and the fetch to post with (the browser's). */
export interface DiscoveryClient {
  readonly clientApi: string;
  readonly fetch?: typeof fetch | undefined;
}

/** Locates the core by a link's secret, spending a use of the link (the protocol's §3.1). */
export async function locate(client: DiscoveryClient, secret: string): Promise<Located> {
  return locatedOf(
    await postToDiscovery({ ...client, path: "/locate", body: { hash: await locateHash(secret) } }),
  );
}

/** The core's paths, by what the device holds. */
export async function pathsOf(client: DiscoveryClient, held: Held): Promise<Located> {
  return locatedOf(await postToDiscovery({ ...client, path: "/paths", body: held }));
}

/** Tells the discovery nothing reached the core, so its admin hears of it (D293 §7); best effort. */
export async function reportDeadEnd(client: DiscoveryClient, held: Held): Promise<void> {
  try {
    await postToDiscovery({ ...client, path: "/dead-end", body: held });
  } catch {
    // A report that fails is no worse than none.
  }
}
