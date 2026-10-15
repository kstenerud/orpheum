// SPDX-License-Identifier: MPL-2.0

/**
 * A realm sign-in (D297, D298): a person with no link finds their core at a paid discovery by its
 * realm, their name and their password, all three or nothing. The page derives a key from the
 * password as the core does (discovery/proof's realm.go, whose vectors the tests hold), proves its
 * request with it, and believes the core's identity key only once the core's tag under the same key
 * holds: no discovery can make the tag, so none can pose as the core to harvest the password the
 * sign-in sends next.
 */

import { base64url, fromBase64, keyFingerprint, locatedOf, postToDiscovery } from "./discovery.ts";
import type { DiscoveryClient, Located } from "./discovery.ts";

const prefix = "orpheum-discovery-v1:";
const encoder = new TextEncoder();

/** PBKDF2-SHA256 rounds a realm key costs: proof.RealmRounds. */
export const realmRounds = 600_000;

/** A realm or a name as a realm key takes it, trimmed and in lower case: proof.RealmName. */
export function realmName(name: string): string {
  return name.trim().toLowerCase();
}

/** The master realm a realm names before any subrealm: proof.MasterRealm. */
export function masterRealm(realm: string): string {
  return realmName(realm).split(".")[0] ?? "";
}

/** A realm as the discovery keeps one: a master of 5 to 32, and maybe a dot and a subrealm (D298). */
export function isRealm(realm: string): boolean {
  return /^[a-z0-9](?:[a-z0-9-]{3,30})[a-z0-9](?:\.(?:[a-z0-9]|[a-z0-9][a-z0-9-]{0,30}[a-z0-9]))?$/u.test(
    realm,
  );
}

/** The key a person's realm sign-in is proven by both ways: proof.RealmKey. */
export async function realmKey(
  realm: string,
  name: string,
  password: string,
): Promise<Uint8Array<ArrayBuffer>> {
  const material = await crypto.subtle.importKey("raw", encoder.encode(password), "PBKDF2", false, [
    "deriveBits",
  ]);
  const salt = encoder.encode(`${prefix}realm-key\n${masterRealm(realm)}\n${realmName(name)}`);
  const bits = await crypto.subtle.deriveBits(
    { name: "PBKDF2", hash: "SHA-256", salt, iterations: realmRounds },
    material,
    256,
  );
  return new Uint8Array(bits);
}

/** Who a realm sign-in is for, as the discovery passes it to the core: proof.RealmPerson. */
export async function realmPerson(realm: string, name: string): Promise<string> {
  const digest = await crypto.subtle.digest(
    "SHA-256",
    encoder.encode(`${prefix}realm-person\n${realmName(realm)}\n${realmName(name)}`),
  );
  return base64url(new Uint8Array(digest));
}

/** An HMAC-SHA256 under key over the prefix and text, base64url. */
async function mac(key: Uint8Array<ArrayBuffer>, text: string): Promise<string> {
  const hmac = await crypto.subtle.importKey("raw", key, { name: "HMAC", hash: "SHA-256" }, false, [
    "sign",
  ]);
  return base64url(
    new Uint8Array(await crypto.subtle.sign("HMAC", hmac, encoder.encode(prefix + text))),
  );
}

/** The page's proof that it holds the person's key: proof.RealmAsk. */
export function realmAsk(
  key: Uint8Array<ArrayBuffer>,
  realm: string,
  person: string,
): Promise<string> {
  return mac(key, `realm-ask\n${realmName(realm)}\n${person}`);
}

/** The core's tag over its id and identity key, which only it can make: proof.RealmTag. */
export function realmTag(
  key: Uint8Array<ArrayBuffer>,
  coreId: string,
  publicKey: Uint8Array<ArrayBuffer>,
): Promise<string> {
  return mac(key, `realm-core\n${coreId}\n${base64url(publicKey)}`);
}

/** What a person types to sign in by realm. */
export interface RealmTyped {
  readonly realm: string;
  readonly name: string;
  readonly password: string;
}

/** What a realm sign-in found: the core, as a link's locate finds it, its key's fingerprint, and the
 * key the person's offers are proven by. */
export interface RealmFound {
  readonly located: Located;
  readonly fingerprint: string;
  readonly key: Uint8Array<ArrayBuffer>;
}

/** The core's identity key and its tag in the discovery's answer, when both are there. */
function signedBy(
  answer: unknown,
): { publicKey: Uint8Array<ArrayBuffer>; tag: string } | undefined {
  const field = (name: string): unknown =>
    typeof answer === "object" && answer !== null ? Reflect.get(answer, name) : undefined;
  const publicKey = field("publicKey");
  const tag = field("tag");
  return typeof publicKey === "string" && typeof tag === "string"
    ? { publicKey: fromBase64(publicKey), tag }
    : undefined;
}

/**
 * Finds the core by a person's realm, name and password: NotFound when any is wrong, as the
 * discovery says no more; and an error when the answer's tag does not hold under the person's key,
 * a discovery answering in the core's place, whose key is never believed.
 */
export async function realmSignIn(client: DiscoveryClient, typed: RealmTyped): Promise<RealmFound> {
  const { realm, name, password } = typed;
  const key = await realmKey(realm, name, password);
  const person = await realmPerson(realm, name);
  const answer = await postToDiscovery({
    ...client,
    path: "/realm",
    body: { realm: realmName(realm), person, ask: await realmAsk(key, realm, person) },
  });
  const located = locatedOf(answer);
  const signed = signedBy(answer);
  const notTheServers = new Error(
    "the answer is not the server's own: its tag does not hold under the password's key",
  );
  if (signed === undefined) {
    throw notTheServers;
  }
  if (signed.tag !== (await realmTag(key, located.coreId, signed.publicKey))) {
    throw notTheServers;
  }
  return { located, fingerprint: await keyFingerprint(signed.publicKey), key };
}
