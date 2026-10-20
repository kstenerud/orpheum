// SPDX-License-Identifier: MPL-2.0

/**
 * A device code through the paid service (D293 §2; discovery/proof's devicecode.go): a device that
 * knows no core makes a P-256 key, asks the service for a code with its public half, shows the code,
 * then the check code made of its key once a person entered the code, and opens what the core sealed
 * to its key once they said the two match. HPKE's base mode (RFC 9180) over DHKEM(P-256,
 * HKDF-SHA256), HKDF-SHA256 and AES-128-GCM is built on Web Crypto alone, so no library is needed and
 * a television's old engine serves; its tests hold it to RFC 9180's vectors and to a blob the core's
 * own seal made.
 */

import { base64url, fromBase64, postToDiscovery, TooMany } from "./discovery.ts";
import type { DiscoveryClient } from "./discovery.ts";

const prefix = "orpheum-discovery-v1:";
const encoder = new TextEncoder();

/** A device's key: the private half, which never leaves the page, and the public half as sent. */
export interface DeviceKey {
  readonly privateKey: CryptoKey;
  /** The uncompressed P-256 point, 65 bytes. */
  readonly publicKey: Uint8Array<ArrayBuffer>;
}

/** A new device key, whose private half cannot be exported. */
export async function newDeviceKey(): Promise<DeviceKey> {
  const pair = await crypto.subtle.generateKey({ name: "ECDH", namedCurve: "P-256" }, false, [
    "deriveBits",
  ]);
  return {
    privateKey: pair.privateKey,
    publicKey: new Uint8Array(await crypto.subtle.exportKey("raw", pair.publicKey)),
  };
}

/** The safe32 alphabet: discovery/safe32code's. */
const safe32 = "0123456789abcdefghjkmnpqrstvwxyz";

/**
 * The check code both screens show for a device asking with publicKey: proof.DeviceCheckCode, 40
 * bits of the key's SHA-256 under its purpose, as a device code is written, "xxxx-xxxx".
 */
export async function deviceCheckCode(publicKey: Uint8Array<ArrayBuffer>): Promise<string> {
  const purpose = encoder.encode(`${prefix}device-check-code\n`);
  const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", concat(purpose, publicKey)));
  let value = 0n;
  for (const byte of digest.subarray(0, 5)) {
    value = (value << 8n) | BigInt(byte);
  }
  let written = "";
  for (let index = 0; index < 8; index++) {
    written = (safe32[Number(value & 31n)] ?? "") + written;
    value >>= 5n;
  }
  return `${written.slice(0, 4)}-${written.slice(4)}`;
}

/** A device's code from the service, the secret it asks how the code stands by, and its life. */
export interface DeviceCode {
  readonly code: string;
  readonly secret: string;
  readonly expiresInSeconds: number;
}

/** Asks the service for a code for key, the device naming its platform. */
export async function requestDeviceCode(
  client: DiscoveryClient,
  key: DeviceKey,
  platform: string,
): Promise<DeviceCode> {
  const answer = await postToDiscovery({
    ...client,
    path: "/device-codes",
    body: { publicKey: base64url(key.publicKey), platform },
  });
  const code = field(answer, "code");
  const secret = field(answer, "secret");
  const expiresIn = field(answer, "expiresIn");
  if (typeof code !== "string" || typeof secret !== "string" || typeof expiresIn !== "number") {
    throw new Error("the discovery's device code is not what it should be");
  }
  return { code, secret, expiresInSeconds: expiresIn };
}

/**
 * The core that claimed a code, as the service names it: its id and its realm ("" for none), which
 * the device shows its person and checks the sealed outcome against (the device code audit's M1).
 */
export interface ClaimedBy {
  readonly coreId: string;
  readonly realm: string;
}

/**
 * How a device's code stands: entered, by the core the service names; activated, with the sealed
 * outcome and a bootstrap token besides; or, while the service asks the device to slow down, as it
 * stood.
 */
export type DeviceCodeState =
  | { readonly state: "waiting" | "ended" | "slowDown" }
  | { readonly state: "entered"; readonly claimedBy: ClaimedBy }
  | {
      readonly state: "activated";
      readonly claimedBy: ClaimedBy;
      readonly sealed: string;
      readonly bootstrapToken: string;
    };

/** How the code its secret names stands. Anything the service says that is none is ended. */
export async function deviceCodeState(
  client: DiscoveryClient,
  secret: string,
): Promise<DeviceCodeState> {
  let answer: unknown;
  try {
    answer = await postToDiscovery({ ...client, path: "/device-codes/state", body: { secret } });
  } catch (error) {
    if (error instanceof TooMany && error.code === "slow_down") {
      return { state: "slowDown" };
    }
    throw error;
  }
  return stateOf(answer);
}

/** The state the service's answer says; anything it does not say right is ended. */
function stateOf(answer: unknown): DeviceCodeState {
  const state = field(answer, "state");
  const claimedBy = claimedByOf(answer);
  if (claimedBy === undefined) {
    return state === "waiting" ? { state } : { state: "ended" };
  }
  const sealed = field(answer, "sealed");
  const bootstrapToken = field(answer, "bootstrapToken");
  if (state === "activated" && typeof sealed === "string" && typeof bootstrapToken === "string") {
    return { state, claimedBy, sealed, bootstrapToken };
  }
  return state === "entered" ? { state, claimedBy } : { state: "ended" };
}

/** The core the answer names as having claimed the code, or undefined when it names none. */
function claimedByOf(answer: unknown): ClaimedBy | undefined {
  const coreId = field(answer, "coreId");
  const realm = field(answer, "realm");
  if (typeof coreId !== "string" || coreId === "") {
    return undefined;
  }
  return { coreId, realm: typeof realm === "string" ? realm : "" };
}

/** What an activated device needs: proof.DeviceOutcome. */
export interface DeviceOutcome {
  readonly coreId: string;
  readonly fingerprint: string;
  readonly realm: string;
  readonly handOffCode: string;
  /** The name of the person it signs in as; "" from a core that names none. */
  readonly person: string;
}

/** HPKE's info for a device's outcome: proof's deviceOutcomeInfo. */
const outcomeInfo = encoder.encode(`${prefix}device-outcome`);

/** The length of an uncompressed P-256 point, HPKE's enc for DHKEM(P-256). */
const encLength = 65;

/** Opens what the core sealed to key: proof.SealDeviceOutcome's encapsulated key, then ciphertext. */
export async function openDeviceOutcome(key: DeviceKey, sealed: string): Promise<DeviceOutcome> {
  const raw = fromBase64(sealed);
  const opened = await openBase(key, {
    enc: raw.slice(0, encLength),
    ciphertext: raw.slice(encLength),
    info: outcomeInfo,
    aad: new Uint8Array(),
  });
  const outcome: unknown = JSON.parse(new TextDecoder().decode(opened));
  const coreId = field(outcome, "coreId");
  const fingerprint = field(outcome, "fingerprint");
  const realm = field(outcome, "realm");
  const handOffCode = field(outcome, "handOffCode");
  if (
    typeof coreId !== "string" ||
    typeof fingerprint !== "string" ||
    typeof realm !== "string" ||
    typeof handOffCode !== "string"
  ) {
    throw new Error("the device's outcome is not what it should be");
  }
  const person = field(outcome, "person");
  return {
    coreId,
    fingerprint,
    realm,
    handOffCode,
    person: typeof person === "string" ? person : "",
  };
}

/** RFC 9180's suite ids: DHKEM(P-256, HKDF-SHA256) 0x0010, HKDF-SHA256 0x0001, AES-128-GCM 0x0001. */
const kemSuite = concat(encoder.encode("KEM"), new Uint8Array([0x00, 0x10]));
const hpkeSuite = concat(
  encoder.encode("HPKE"),
  new Uint8Array([0x00, 0x10, 0x00, 0x01, 0x00, 0x01]),
);

/** A message sealed by HPKE: the encapsulated key, the ciphertext, and the info and aad it binds. */
export interface SealedMessage {
  readonly enc: Uint8Array<ArrayBuffer>;
  readonly ciphertext: Uint8Array<ArrayBuffer>;
  readonly info: Uint8Array<ArrayBuffer>;
  readonly aad: Uint8Array<ArrayBuffer>;
}

/** HPKE's base-mode single-shot open (RFC 9180 §5.1, §6.1) of sealed, the first message, to key. */
export async function openBase(
  key: DeviceKey,
  sealed: SealedMessage,
): Promise<Uint8Array<ArrayBuffer>> {
  const sender = await crypto.subtle.importKey(
    "raw",
    sealed.enc,
    { name: "ECDH", namedCurve: "P-256" },
    false,
    [],
  );
  const dh = new Uint8Array(
    await crypto.subtle.deriveBits({ name: "ECDH", public: sender }, key.privateKey, 256),
  );
  // DHKEM's Decap: ExtractAndExpand over the DH and the KEM context, enc then the recipient's key.
  const eaePrk = await labeledExtract({ suite: kemSuite, label: "eae_prk", ikm: dh });
  const sharedSecret = await labeledExpand({
    suite: kemSuite,
    prk: eaePrk,
    label: "shared_secret",
    info: concat(sealed.enc, key.publicKey),
    length: 32,
  });
  // KeySchedule in mode_base: no PSK.
  const pskIdHash = await labeledExtract({ suite: hpkeSuite, label: "psk_id_hash", ikm: empty });
  const infoHash = await labeledExtract({ suite: hpkeSuite, label: "info_hash", ikm: sealed.info });
  const context = concat(new Uint8Array([0x00]), pskIdHash, infoHash);
  const secret = await labeledExtract({
    suite: hpkeSuite,
    salt: sharedSecret,
    label: "secret",
    ikm: empty,
  });
  const expand = { suite: hpkeSuite, prk: secret, info: context };
  const aeadKey = await labeledExpand({ ...expand, label: "key", length: 16 });
  // The first message's nonce is the base nonce, its sequence number 0.
  const nonce = await labeledExpand({ ...expand, label: "base_nonce", length: 12 });
  const gcm = await crypto.subtle.importKey("raw", aeadKey, "AES-GCM", false, ["decrypt"]);
  return new Uint8Array(
    await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: nonce, additionalData: sealed.aad },
      gcm,
      sealed.ciphertext,
    ),
  );
}

const empty = new Uint8Array();

/** RFC 9180's LabeledExtract under suite: HKDF-Extract over "HPKE-v1", suite, label and ikm. */
async function labeledExtract(extract: {
  readonly suite: Uint8Array<ArrayBuffer>;
  readonly salt?: Uint8Array<ArrayBuffer>;
  readonly label: string;
  readonly ikm: Uint8Array<ArrayBuffer>;
}): Promise<Uint8Array<ArrayBuffer>> {
  // An empty salt is HashLen zeros (RFC 5869 §2.2), which HMAC reads as the empty key it pads to.
  return await hmac(
    extract.salt ?? new Uint8Array(32),
    concat(encoder.encode("HPKE-v1"), extract.suite, encoder.encode(extract.label), extract.ikm),
  );
}

/** RFC 9180's LabeledExpand under suite to length bytes, at most one SHA-256 block here. */
async function labeledExpand(expand: {
  readonly suite: Uint8Array<ArrayBuffer>;
  readonly prk: Uint8Array<ArrayBuffer>;
  readonly label: string;
  readonly info: Uint8Array<ArrayBuffer>;
  readonly length: number;
}): Promise<Uint8Array<ArrayBuffer>> {
  const labeledInfo = concat(
    new Uint8Array([expand.length >> 8, expand.length & 0xff]),
    encoder.encode("HPKE-v1"),
    expand.suite,
    encoder.encode(expand.label),
    expand.info,
  );
  // HKDF-Expand's T(1) = HMAC(PRK, info || 0x01), which is all of it for 32 bytes or fewer.
  const block = await hmac(expand.prk, concat(labeledInfo, new Uint8Array([0x01])));
  return block.slice(0, expand.length);
}

/** HMAC-SHA256 of data under key. */
async function hmac(
  key: Uint8Array<ArrayBuffer>,
  data: Uint8Array<ArrayBuffer>,
): Promise<Uint8Array<ArrayBuffer>> {
  const imported = await crypto.subtle.importKey(
    "raw",
    key,
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  return new Uint8Array(await crypto.subtle.sign("HMAC", imported, data));
}

/** The parts, one after another. */
function concat(...parts: readonly Uint8Array<ArrayBuffer>[]): Uint8Array<ArrayBuffer> {
  const joined = new Uint8Array(parts.reduce((total, part) => total + part.length, 0));
  let at = 0;
  for (const part of parts) {
    joined.set(part, at);
    at += part.length;
  }
  return joined;
}

/** The answer's field name, when the answer is an object. */
function field(answer: unknown, name: string): unknown {
  return typeof answer === "object" && answer !== null ? Reflect.get(answer, name) : undefined;
}
