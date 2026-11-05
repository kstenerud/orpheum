// SPDX-License-Identifier: MPL-2.0

import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { readLinkFragment, type Api, type components } from "@orpheum/sdk";

import { dataOrThrow, throwIfRefused } from "./problem.ts";
import { thisDevice, type NewDevice } from "./device.ts";
import type { Session } from "./session.ts";

/** One person, as the admin's list shows them. */
export type Account = components["schemas"]["UserEntryBody"];

/**
 * What signing in answers: the new session's token, the device it is on, that device's secret, the
 * proof a later sign-in resumes it by (the third audit's A8), and its locator, which a discovery finds
 * the server by (discovery-core.md K7).
 */
export type SignInResult = Readonly<
  Pick<components["schemas"]["SessionOutputBody"], "token" | "deviceSecret" | "locator">
> & {
  readonly user: { readonly device?: { readonly id: number; readonly name: string } };
};

/**
 * The link a registration invitation is shared as: the secret after #, so the browser never sends
 * it to the server's logs or anyone else (D218 §1). While a discovery has claimed the core, the
 * core names the link in the discovery's form, which reaches it from anywhere (the protocol's §3.1),
 * and that one is shared; otherwise it is made on this page's origin.
 */
export function registrationUrl(
  origin: string,
  issued: { readonly secret: string; readonly link?: string | undefined },
): string {
  return issued.link ?? `${origin}/register#${issued.secret}`;
}

/** The secret a registration invitation's link carries after #, or undefined when there is none. */
export function secretFromHash(hash: string): string | undefined {
  // A link a discovery serves carries the core and its fingerprint around the secret (the protocol's §3.1).
  const link = readLinkFragment(hash);
  if (link !== undefined) {
    return link.secret;
  }
  const secret = hash.startsWith("#") ? hash.slice(1) : hash;
  return secret === "" ? undefined : secret;
}

/** Everyone, for the admin. */
export async function listAccounts(api: Api): Promise<Account[]> {
  return dataOrThrow(await api.GET("/users")).users ?? [];
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
  return dataOrThrow(await api.POST("/users", { body, ...idempotent(idempotencyKey) })).user;
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

/** The invitations still waiting, newest first; a used one is not listed (D403). */
export async function listInvitations(api: Api): Promise<Invitation[]> {
  return dataOrThrow(await api.GET("/invitations")).invitations ?? [];
}

/** Withdraws an invitation still waiting: none of its links admits anyone. */
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
    await api.POST("/registration/link", { body: { secret }, ...idempotent(newIdempotencyKey()) }),
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

/** Signs this device in as name with password, keeping the session the server answers with. */
export async function signInThisDevice(
  services: { readonly api: Api; readonly session: Session },
  credentials: { readonly name: string; readonly password: string },
  idempotencyKey: string,
): Promise<void> {
  const { api, session } = services;
  const { token, deviceSecret, locator, user } = await signIn(
    api,
    { ...credentials, device: thisDevice(session) },
    idempotencyKey,
  );
  session.signIn(token, { deviceId: user.device?.id, deviceSecret, locator });
}

/** Whether the server still needs its first admin, asked before anyone signs in. */
export async function isSetupRequired(api: Api): Promise<boolean> {
  return dataOrThrow(await api.GET("/setup")).isRequired;
}

/**
 * Makes a server's first admin with the one-time setup code it printed (cut-a §5.2's POST /setup),
 * under the click's key, which the server keeps apart from the same key's sign-in by the path.
 */
export async function completeSetup(
  api: Api,
  first: { readonly setupCode: string; readonly name: string; readonly password: string },
  idempotencyKey: string,
): Promise<void> {
  dataOrThrow(await api.POST("/setup", { body: { ...first }, ...idempotent(idempotencyKey) }));
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

/** What entering a device's code gives: the check code it now shows too, and the name it is listed by. */
export type Activation = components["schemas"]["ActivationOutputBody"];

/**
 * Enters the code a device shows (D288 §2): for oneself, or, an admin, for the person userId, under
 * the name the device will be listed by; with keyHash, the code was scanned from the device's QR code,
 * and the answer has no check code (D321).
 */
export async function activateDevice(
  api: Api,
  activation: {
    readonly code: string;
    readonly deviceName: string;
    readonly userId?: number;
    readonly keyHash?: string;
  },
  idempotencyKey: string,
): Promise<Activation> {
  // Named on the server's schema, so a field the schema has not is a type error here, not a silent
  // drop: a scanned code's keyHash among them (D321).
  const body: components["schemas"]["ActivationInputBody"] = {
    code: activation.code,
    deviceName: activation.deviceName,
  };
  if (activation.userId !== undefined) {
    body.userId = activation.userId;
  }
  if (activation.keyHash !== undefined) {
    body.keyHash = activation.keyHash;
  }
  return dataOrThrow(await api.POST("/activations", { body, ...idempotent(idempotencyKey) }));
}

/** Says whether the two check codes match: only a match activates the device (D288 §3). */
export async function answerActivation(
  api: Api,
  answer: { readonly code: string; readonly isMatch: boolean },
  idempotencyKey: string,
): Promise<void> {
  throwIfRefused(
    await api.POST("/activations/answer", { body: { ...answer }, ...idempotent(idempotencyKey) }),
  );
}

/** Changes one's own password. */
export async function changePassword(
  api: Api,
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  throwIfRefused(await api.PUT("/me/password", { body: { currentPassword, newPassword } }));
}

/** Whether an admin administers from away from home too: their own setting, set from home (D302). */
export async function setAdminAway(api: Api, adminAway: "never" | "always"): Promise<void> {
  throwIfRefused(await api.PUT("/me/admin-away", { body: { adminAway } }));
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

/** Keeps how far one of the person's devices moves its sound from its picture, in ms: + later. */
export async function setMyDeviceSoundOffset(
  api: Api,
  deviceId: number,
  soundOffsetMs: number,
): Promise<void> {
  throwIfRefused(
    await api.PUT("/me/devices/{id}", {
      params: { path: { id: deviceId } },
      body: { soundOffsetMs },
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

/** A person's page, for the admin: who they are and what they have watched. */
export type UserPageBody = components["schemas"]["UserPageOutputBody"];

/** A person's page. */
export async function getUser(api: Api, userId: number): Promise<UserPageBody> {
  return dataOrThrow(await api.GET("/users/{id}", { params: { path: { id: userId } } }));
}

/** Renames a person. */
export async function renameUser(api: Api, userId: number, name: string): Promise<void> {
  throwIfRefused(
    await api.PUT("/users/{id}", { params: { path: { id: userId } }, body: { name } }),
  );
}

/** Removes a person: they are signed out everywhere, and their own watching is deleted (S40). */
export async function removeUser(api: Api, userId: number): Promise<void> {
  throwIfRefused(await api.DELETE("/users/{id}", { params: { path: { id: userId } } }));
}

/** The signed-in person's own link, the one they registered by. */
export type OwnLink = components["schemas"]["OwnLinkOutputBody"];

/** The signed-in person's own link, or undefined when they have none (D292 §4). */
export async function getOwnLink(api: Api): Promise<OwnLink | undefined> {
  const answer = await api.GET("/me/own-link");
  return answer.response.status === 404 ? undefined : dataOrThrow(answer);
}

/** Ends a person's own link, which leads them back through a discovery; they stay signed in. */
export async function revokeOwnLink(api: Api, userId: number): Promise<void> {
  throwIfRefused(await api.DELETE("/users/{id}/own-link", { params: { path: { id: userId } } }));
}

/** A person's devices, for the admin. */
export async function listUserDevices(api: Api, userId: number): Promise<Device[]> {
  return (
    dataOrThrow(await api.GET("/users/{id}/devices", { params: { path: { id: userId } } }))
      .devices ?? []
  );
}

/** Removes a person's device, for the admin: a lost or given-away one. */
export async function removeUserDevice(api: Api, userId: number, deviceId: number): Promise<void> {
  throwIfRefused(
    await api.DELETE("/users/{id}/devices/{deviceId}", {
      params: { path: { id: userId, deviceId } },
    }),
  );
}
