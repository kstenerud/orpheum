// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";

import type { SignInResult } from "./accounts.ts";
import type { StringStorage } from "./browser-storage.ts";
import { dataOrThrow } from "./problem.ts";

/*
 * A device not signed in, activated by the code it shows (D288): what it asks the server on its own,
 * no click sending it, and how its tab keeps its code across a reload.
 */

/** A code this device shows to be activated, and the secret by which it alone asks after it (D288). */
export type DevicePairing = components["schemas"]["PairingOutputBody"];

/** How this device's pairing stands, and the check code to show while it is checked. */
export type PairingState = components["schemas"]["PairingStateOutputBody"];

/**
 * A code for this device to show (D288 §1): the one it has while secret names a pairing no one has
 * entered, or a new one; the device it was before is resumed when it is the same person's (D221).
 */
export async function requestPairing(
  api: Api,
  pairing: { readonly secret: string | undefined; readonly previousDeviceId: number | undefined },
): Promise<DevicePairing> {
  const { secret, previousDeviceId } = pairing;
  const body = {
    platform: "web",
    ...(secret === undefined ? {} : { secret }),
    ...(previousDeviceId === undefined ? {} : { previousDeviceId }),
  } satisfies components["schemas"]["PairingRequestInputBody"];
  return dataOrThrow(await api.POST("/pairings", { body }));
}

/** How the pairing secret names stands. */
export async function pairingStateOf(api: Api, secret: string): Promise<PairingState> {
  return dataOrThrow(await api.POST("/pairings/state", { body: { secret } }));
}

/** The session a confirmed pairing made, taken once. */
export async function takePairedSession(api: Api, secret: string): Promise<SignInResult> {
  return dataOrThrow(await api.POST("/pairings/session", { body: { secret } }));
}

/** Where this tab keeps its pairing, so a reload shows the same code until it ends. */
const pairingKey = "orpheum.pairing";

/** Keeps this tab's pairing. */
export function keepPairing(storage: StringStorage, pairing: DevicePairing): void {
  storage.setItem(pairingKey, JSON.stringify(pairing));
}

/** What was kept, when it is a pairing this tab wrote. */
function asPairing(kept: unknown): DevicePairing | undefined {
  if (
    typeof kept !== "object" ||
    kept === null ||
    !("secret" in kept && "code" in kept && "expiresAt" in kept)
  ) {
    return undefined;
  }
  const { secret, code, expiresAt } = kept;
  if (typeof secret !== "string" || typeof code !== "string" || typeof expiresAt !== "number") {
    return undefined;
  }
  return { secret, code, expiresAt };
}

/** The pairing this tab keeps, while its code works; one that has expired is forgotten. */
export function readPairing(storage: StringStorage, nowMs: number): DevicePairing | undefined {
  const kept = storage.getItem(pairingKey);
  let pairing: DevicePairing | undefined;
  try {
    pairing = kept === null ? undefined : asPairing(JSON.parse(kept));
  } catch {
    pairing = undefined; // not what this tab wrote
  }
  if (pairing === undefined || pairing.expiresAt <= nowMs) {
    storage.removeItem(pairingKey);
    return undefined;
  }
  return pairing;
}

/** Forgets this tab's pairing: it ended, or signed the device in. */
export function forgetPairing(storage: StringStorage): void {
  storage.removeItem(pairingKey);
}
