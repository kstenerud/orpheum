// SPDX-License-Identifier: MPL-2.0

/**
 * A device code through the paid service (D293 §2; discovery/proof's devicecode.go): a device that
 * knows no core makes a P-256 key and a nonce, asks the service for a code with its public half and a
 * commitment to the nonce, and shows the code with a QR code carrying its key's hash. Entered by its
 * typed code, it is told the core's nonce, reveals its own, and shows the check code made of the key
 * and both; scanned, it shows none (D321). It opens what the core sealed to its key once its person
 * said yes. HPKE's base mode (RFC 9180) over DHKEM(P-256,
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

/** A device's or a core's nonce for the check code, before encoding: proof.DeviceNonceBytes. */
const nonceBytes = 16;

/** A new nonce for the check code, base64url, which the device commits to before revealing (D321). */
export function newDeviceNonce(): string {
  return base64url(crypto.getRandomValues(new Uint8Array(nonceBytes)));
}

/** The SHA-256 of parts after a purpose of the discovery's own. */
async function purposed(
  purpose: string,
  ...parts: Uint8Array<ArrayBuffer>[]
): Promise<Uint8Array<ArrayBuffer>> {
  const label = encoder.encode(`${prefix}${purpose}\n`);
  return new Uint8Array(await crypto.subtle.digest("SHA-256", concat(label, ...parts)));
}

/** A nonce as bytes, refused when it is not nonceBytes of base64url. */
function nonceOf(nonce: string): Uint8Array<ArrayBuffer> {
  const raw = fromBase64(nonce);
  if (raw.length !== nonceBytes) {
    throw new Error("not a device code's nonce");
  }
  return raw;
}

/** The commitment a device asks with: proof.DeviceCommitment of its key and its nonce, base64url. */
export async function deviceCommitment(
  publicKey: Uint8Array<ArrayBuffer>,
  nonce: string,
): Promise<string> {
  return base64url(await purposed("device-commitment", publicKey, nonceOf(nonce)));
}

/**
 * What the device's QR code carries of its key: proof.DeviceKeyHash, 128 bits of its SHA-256,
 * base64url, which the core holds the key it activates to (D321).
 */
export async function deviceKeyHash(publicKey: Uint8Array<ArrayBuffer>): Promise<string> {
  return base64url((await purposed("device-key-hash", publicKey)).subarray(0, 16));
}

/**
 * The check code both screens show for a device asking with publicKey, which committed to
 * deviceNonce, once the core gave coreNonce: proof.DeviceCheckCode, 40 bits of the SHA-256 of all
 * three under its purpose, as a device code is written, "xxxx-xxxx" (D321).
 */
export async function deviceCheckCode(
  publicKey: Uint8Array<ArrayBuffer>,
  deviceNonce: string,
  coreNonce: string,
): Promise<string> {
  const digest = await purposed(
    "device-check-code",
    publicKey,
    nonceOf(deviceNonce),
    nonceOf(coreNonce),
  );
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

/** What a device asks for a code with: its key, the nonce it commits to, and its platform. */
export interface DeviceAsking {
  readonly key: DeviceKey;
  readonly nonce: string;
  readonly platform: string;
}

/** Asks the service for a code for the asking device's key, committed to its nonce (D321). */
export async function requestDeviceCode(
  client: DiscoveryClient,
  { key, nonce, platform }: DeviceAsking,
): Promise<DeviceCode> {
  const answer = await postToDiscovery({
    ...client,
    path: "/device-codes",
    body: {
      publicKey: base64url(key.publicKey),
      platform,
      commitment: await deviceCommitment(key.publicKey, nonce),
    },
  });
  const code = field(answer, "code");
  const secret = field(answer, "secret");
  const expiresInSeconds = field(answer, "expiresInSeconds");
  if (
    typeof code !== "string" ||
    typeof secret !== "string" ||
    typeof expiresInSeconds !== "number"
  ) {
    throw new Error("the discovery's device code is not what it should be");
  }
  return { code, secret, expiresInSeconds };
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
 * How a device's code stands: entered, by the core the service names, with that core's nonce when
 * its code was typed and "" when it was scanned (D321); activated, with the sealed outcome and a
 * bootstrap token besides; or, while the service asks the device to slow down, as it stood.
 */
export type DeviceCodeState =
  | { readonly state: "waiting" | "ended" | "slowDown" }
  | { readonly state: "entered"; readonly claimedBy: ClaimedBy; readonly coreNonce: string }
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
  const coreNonce = field(answer, "coreNonce");
  return state === "entered"
    ? { state, claimedBy, coreNonce: typeof coreNonce === "string" ? coreNonce : "" }
    : { state: "ended" };
}

/**
 * Reveals the nonce the device committed to, once its code stands entered with the core's (D321).
 * The service refuses one that does not open the commitment.
 */
export async function revealDeviceNonce(
  client: DiscoveryClient,
  secret: string,
  nonce: string,
): Promise<void> {
  await postToDiscovery({ ...client, path: "/device-codes/reveal", body: { secret, nonce } });
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
