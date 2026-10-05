// SPDX-License-Identifier: MPL-2.0

import { idempotent } from "./idempotent.ts";
import type { Api, components } from "@orpheum/sdk";

import type { StringStorage } from "./browser-storage.ts";
import { dataOrThrow, throwIfRefused } from "./problem.ts";
import type { NewDevice } from "./device.ts";

/** One person, as the admin's list shows them. */
export type Account = components["schemas"]["AccountBody"];

/** What signing in answers: the new session's token, and the device it is on. */
interface SignInResult {
  readonly token: string;
  readonly user: { readonly device?: { readonly id: number; readonly name: string } };
}

/**
 * The link a registration invitation is shared as: the secret after #, so the browser never sends
 * it to the server's logs or anyone else (D218 §1).
 */
export function registrationUrl(origin: string, secret: string): string {
  return `${origin}/register#${secret}`;
}

/** The secret a registration invitation's link carries after #, or undefined when there is none. */
export function secretFromHash(hash: string): string | undefined {
  const secret = hash.startsWith("#") ? hash.slice(1) : hash;
  return secret === "" ? undefined : secret;
}

/** Everyone, for the admin. */
export async function listAccounts(api: Api): Promise<Account[]> {
  return dataOrThrow(await api.GET("/users")).accounts ?? [];
}

/** An account the admin sets up in full: a name, a password, a role, and a note of who it is for. */
export interface NewAccount {
  readonly name: string;
  readonly password: string;
  readonly role: string;
  readonly adminNote: string;
}

/** Sets a person's account up in full (D246 §2). */
export async function createAccount(
  api: Api,
  account: NewAccount,
  idempotencyKey: string,
): Promise<Account> {
  const { adminNote, ...unnoted } = account;
  const body = adminNote === "" ? unnoted : { ...account };
  return dataOrThrow(await api.POST("/users", { body, ...idempotent(idempotencyKey) })).account;
}

/** An invitation not yet used: its role, and the admin's note of who it is for. */
export type Invitation = components["schemas"]["InvitationBody"];

/** Invites someone to make their own account under a role, with a note of who it is for (D246 §2). */
export async function invite(
  api: Api,
  invitation: { readonly role: string; readonly adminNote: string },
  idempotencyKey: string,
): Promise<{ invitation: Invitation; link: components["schemas"]["IssuedSecretBody"] }> {
  return dataOrThrow(
    await api.POST("/invitations", { body: { ...invitation }, ...idempotent(idempotencyKey) }),
  );
}

/** The invitations not yet used, newest first. */
export async function listInvitations(api: Api): Promise<Invitation[]> {
  return dataOrThrow(await api.GET("/invitations")).invitations ?? [];
}

/** Withdraws an invitation: its link admits no one. */
export async function withdrawInvitation(api: Api, invitationId: number): Promise<void> {
  throwIfRefused(await api.DELETE("/invitations/{id}", { params: { path: { id: invitationId } } }));
}

/** What a link is for: an invitation, or a registration link for the person it names. */
export type LinkKind = components["schemas"]["LinkKindOutputBody"];

/**
 * What a link is for, without using it: asked as its page opens, not by a click, and a question
 * that changes nothing, so each asking is its own (D244 §4).
 */
export async function linkKind(api: Api, secret: string): Promise<LinkKind> {
  return dataOrThrow(
    await api.POST("/registration/link", { body: { secret }, ...idempotent(crypto.randomUUID()) }),
  );
}

/** Changes the admin's note of who a person is for; empty removes it. */
export async function setAdminNote(api: Api, userId: number, adminNote: string): Promise<void> {
  throwIfRefused(
    await api.PUT("/users/{id}/admin-note", {
      params: { path: { id: userId } },
      body: { adminNote },
    }),
  );
}

/** A new registration invitation for a person, withdrawing the old. */
export async function issueRegistrationInvitation(
  api: Api,
  userId: number,
  idempotencyKey: string,
): Promise<components["schemas"]["IssuedSecretBody"]> {
  return dataOrThrow(
    await api.POST("/users/{id}/registration-invitation", {
      params: { path: { id: userId } },
      ...idempotent(idempotencyKey),
    }),
  );
}

/** Signs this device in with a name and password. */
export async function signIn(
  api: Api,
  credentials: { readonly name: string; readonly password: string; readonly device: NewDevice },
  idempotencyKey: string,
): Promise<SignInResult> {
  return dataOrThrow(
    await api.POST("/session", { body: { ...credentials }, ...idempotent(idempotencyKey) }),
  );
}

/**
 * Uses a link: an invitation makes the account with the name chosen; a registration link sets the
 * person's new password. Either signs this device in.
 */
export async function register(
  api: Api,
  registration: {
    readonly secret: string;
    readonly name?: string;
    readonly password: string;
    readonly device: NewDevice;
  },
  idempotencyKey: string,
): Promise<SignInResult> {
  return dataOrThrow(
    await api.POST("/registration", { body: { ...registration }, ...idempotent(idempotencyKey) }),
  );
}

/** What a device code is for: a new device, named, or a listed one to sign back in (D221). */
export type CodeTarget = { readonly deviceName: string } | { readonly deviceId: number };

/** A code that signs a device in, for oneself or, for an admin, for another person. */
export async function issueDeviceCode(
  api: Api,
  code: { readonly target: CodeTarget; readonly userId?: number },
  idempotencyKey: string,
): Promise<components["schemas"]["IssuedSecretBody"]> {
  const { target, userId } = code;
  const body = userId === undefined ? { ...target } : { ...target, userId };
  return dataOrThrow(await api.POST("/device-codes", { body, ...idempotent(idempotencyKey) }));
}

/** Whether one's device code is still waiting to be used (D221 §3). */
export async function isCodeWaiting(api: Api): Promise<boolean> {
  return dataOrThrow(await api.GET("/me/device-code")).isWaiting;
}

/** Signs this device in with a device code, resuming the device this browser was before (D221). */
export async function signInWithCode(
  api: Api,
  signIn: { readonly code: string; readonly previousDeviceId: number | undefined },
  idempotencyKey: string,
): Promise<SignInResult> {
  const { code, previousDeviceId } = signIn;
  const body = (
    previousDeviceId === undefined
      ? { code, platform: "web" }
      : { code, platform: "web", previousDeviceId }
  ) satisfies components["schemas"]["CodeSignInInputBody"];
  return dataOrThrow(await api.POST("/session/code", { body, ...idempotent(idempotencyKey) }));
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
export async function setMyDeviceAudioOutput(
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
export async function signOutElsewhere(api: Api, idempotencyKey: string): Promise<void> {
  throwIfRefused(await api.POST("/me/devices/sign-out-elsewhere", idempotent(idempotencyKey)));
}

/** Where this tab keeps a device code it showed, so a reload shows it again until it expires. */
const shownCodeKey = "orpheum.deviceCode";

/** Keeps a device code this tab showed, with its device's name. */
export function keepShownCode(
  storage: StringStorage,
  code: components["schemas"]["IssuedSecretBody"] & { readonly deviceName: string },
): void {
  storage.setItem(shownCodeKey, JSON.stringify(code));
}

/** A device code as this tab keeps it: with the name of the device it is for. */
export type ShownCode = components["schemas"]["IssuedSecretBody"] & { readonly deviceName: string };

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
export function forgetShownCode(storage: StringStorage): void {
  storage.removeItem(shownCodeKey);
}

/** The device code this tab last showed, while it still works; one that has expired is forgotten. */
export function readShownCode(storage: StringStorage, nowMs: number): ShownCode | undefined {
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
