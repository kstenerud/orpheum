// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";

import { dataOrThrow, throwIfRefused } from "./problem.ts";
import type { NewDevice } from "./device.ts";

/** One person, as the admin's list shows them. */
export type Account = components["schemas"]["AccountBody"];

/** A registration link's secret or a device code, and when it stops working. */
export type Invitation = components["schemas"]["InvitationBody"];

/** A new session: its token, and the device it is on. */
interface Session {
  readonly token: string;
  readonly user: { readonly device?: { readonly id: number; readonly name: string } };
}

/**
 * The address a registration link is shared as: the secret after #, so the browser never sends it
 * to the server's logs or anyone else (D218 §1).
 */
export function registrationUrl(origin: string, secret: string): string {
  return `${origin}/register#${secret}`;
}

/** The secret a registration link carries after #, or undefined when there is none. */
export function secretFromHash(hash: string): string | undefined {
  const secret = hash.startsWith("#") ? hash.slice(1) : hash;
  return secret === "" ? undefined : secret;
}

/** Everyone, for the admin. */
export async function listAccounts(api: Api): Promise<Account[]> {
  return dataOrThrow(await api.GET("/users")).accounts ?? [];
}

/** Makes a person, and their registration link. */
export async function createAccount(
  api: Api,
  name: string,
  role: "admin" | "user",
): Promise<{ account: Account; registration: Invitation }> {
  return dataOrThrow(await api.POST("/users", { body: { name, role } }));
}

/** A new registration link for a person, withdrawing the old. */
export async function issueRegistrationLink(api: Api, userId: number): Promise<Invitation> {
  return dataOrThrow(
    await api.POST("/users/{id}/registration-link", { params: { path: { id: userId } } }),
  );
}

/** Uses a registration link: sets the password and signs this device in. */
export async function register(
  api: Api,
  registration: { readonly secret: string; readonly password: string; readonly device: NewDevice },
): Promise<Session> {
  return dataOrThrow(await api.POST("/registration", { body: { ...registration } }));
}

/** What a device code is for: a new device, named, or a listed one to sign back in (D221). */
export type CodeTarget = { readonly deviceName: string } | { readonly deviceId: number };

/** A code that signs a device in, for oneself or, for an admin, for another person. */
export async function issueDeviceCode(
  api: Api,
  target: CodeTarget,
  userId?: number,
): Promise<Invitation> {
  const body = userId === undefined ? { ...target } : { ...target, userId };
  return dataOrThrow(await api.POST("/device-codes", { body }));
}

/** Whether one's device code is still waiting to be used (D221 §3). */
export async function isCodeWaiting(api: Api): Promise<boolean> {
  return dataOrThrow(await api.GET("/me/device-code")).isWaiting;
}

/** Signs this device in with a device code, resuming the device this browser was before (D221). */
export async function signInWithCode(
  api: Api,
  code: string,
  resumeId: number | undefined,
): Promise<Session> {
  const body =
    resumeId === undefined ? { code, platform: "web" } : { code, platform: "web", resumeId };
  return dataOrThrow(await api.POST("/session/code", { body }));
}

/** Changes one's own password. */
export async function changePassword(
  api: Api,
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  throwIfRefused(await api.PUT("/me/password", { body: { currentPassword, newPassword } }));
}

/** One of a person's devices. */
export type Device = components["schemas"]["DeviceBody"];

/** One's own devices, most recently used first. */
export async function listMyDevices(api: Api): Promise<Device[]> {
  return dataOrThrow(await api.GET("/me/devices")).devices ?? [];
}

/** Renames one of one's devices. */
export async function renameMyDevice(api: Api, deviceId: number, name: string): Promise<void> {
  throwIfRefused(
    await api.PUT("/me/devices/{id}", {
      params: { path: { id: deviceId } },
      body: { name },
    }),
  );
}

/** How a device makes sound (D222). */
export type AudioOutput = NonNullable<Device["audioOutput"]>;

/** Keeps how one of one's devices makes sound (D222). */
export async function setMyDeviceSound(
  api: Api,
  deviceId: number,
  audioOutput: AudioOutput,
): Promise<void> {
  throwIfRefused(
    await api.PUT("/me/devices/{id}", {
      params: { path: { id: deviceId } },
      body: { audioOutput },
    }),
  );
}

/** Removes one of one's devices: it is signed out at its next request (D218 §4). */
export async function removeMyDevice(api: Api, deviceId: number): Promise<void> {
  throwIfRefused(await api.DELETE("/me/devices/{id}", { params: { path: { id: deviceId } } }));
}

/** Signs every other device out; they stay listed. */
export async function signOutElsewhere(api: Api): Promise<void> {
  throwIfRefused(await api.POST("/me/devices/sign-out-elsewhere"));
}

/** Where this tab keeps a device code it showed, so a reload shows it again until it expires. */
const shownCodeKey = "orpheum.deviceCode";

/** Keeps a device code this tab showed, with its device's name. */
export function keepShownCode(
  storage: Storage,
  code: Invitation & { readonly deviceName: string },
): void {
  storage.setItem(shownCodeKey, JSON.stringify(code));
}

/** A device code as this tab keeps it: with the name of the device it is for. */
export type ShownCode = Invitation & { readonly deviceName: string };

/** What was kept, when it is a code this tab wrote. */
function asShownCode(kept: unknown): ShownCode | undefined {
  if (
    typeof kept !== "object" ||
    kept === null ||
    !("secret" in kept && "expiresAt" in kept && "deviceName" in kept)
  ) {
    return undefined;
  }
  const { secret, expiresAt, deviceName } = kept;
  if (
    typeof secret !== "string" ||
    typeof expiresAt !== "number" ||
    typeof deviceName !== "string"
  ) {
    return undefined;
  }
  return { secret, expiresAt, deviceName };
}

/** Forgets the device code this tab showed: it was used, or replaced. */
export function forgetShownCode(storage: Storage): void {
  storage.removeItem(shownCodeKey);
}

/** The device code this tab last showed, while it still works; one that has expired is forgotten. */
export function shownCode(storage: Storage, nowMs: number): ShownCode | undefined {
  const kept = storage.getItem(shownCodeKey);
  let code: ShownCode | undefined;
  try {
    code = kept === null ? undefined : asShownCode(JSON.parse(kept));
  } catch {
    code = undefined; // not what this tab wrote
  }
  if (code === undefined || code.expiresAt <= nowMs) {
    storage.removeItem(shownCodeKey);
    return undefined;
  }
  return code;
}

/** A person's page, for the admin: who they are and what they have watched. */
export type Person = components["schemas"]["PersonOutputBody"];

/** A person's page. */
export async function getPerson(api: Api, userId: number): Promise<Person> {
  return dataOrThrow(await api.GET("/users/{id}", { params: { path: { id: userId } } }));
}

/** Renames a person. */
export async function renamePerson(api: Api, userId: number, name: string): Promise<void> {
  throwIfRefused(
    await api.PUT("/users/{id}", { params: { path: { id: userId } }, body: { name } }),
  );
}

/** A person's devices, for the admin. */
export async function listPersonDevices(api: Api, userId: number): Promise<Device[]> {
  return (
    dataOrThrow(await api.GET("/users/{id}/devices", { params: { path: { id: userId } } }))
      .devices ?? []
  );
}

/** Removes a person's device, for the admin: a lost or given-away one. */
export async function removePersonDevice(
  api: Api,
  userId: number,
  deviceId: number,
): Promise<void> {
  throwIfRefused(
    await api.DELETE("/users/{id}/devices/{deviceId}", {
      params: { path: { id: userId, deviceId } },
    }),
  );
}
