// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";

import type { NewDevice } from "./device.ts";

/** One person, as the admin's list shows them. */
export type Account = components["schemas"]["AccountBody"];

/** A registration link's secret or a device code, and when it stops working. */
export type Invitation = components["schemas"]["InvitationBody"];

/** A new session: its token. */
interface Session {
  readonly token: string;
}

/** Throws the server's message, or returns the data, of an SDK call. */
function dataOrThrow<T>(result: { data?: T; error?: { error: { message: string } } }): T {
  if (result.data === undefined) {
    throw new Error(result.error?.error.message ?? "the server did not answer");
  }
  return result.data;
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

/** A code that signs a device in, for oneself or, for an admin, for another person. */
export async function issueDeviceCode(
  api: Api,
  deviceName: string,
  userId?: number,
): Promise<Invitation> {
  const body = userId === undefined ? { deviceName } : { deviceName, userId };
  return dataOrThrow(await api.POST("/device-codes", { body }));
}

/** Signs this device in with a device code. */
export async function signInWithCode(api: Api, code: string): Promise<Session> {
  return dataOrThrow(await api.POST("/session/code", { body: { code, platform: "web" } }));
}

/** Changes one's own password. */
export async function changePassword(
  api: Api,
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  const { error } = await api.PUT("/me/password", { body: { currentPassword, newPassword } });
  if (error !== undefined) {
    throw new Error(error.error.message);
  }
}
