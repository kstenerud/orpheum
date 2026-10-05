// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import { idempotent } from "./idempotent.ts";
import { dataOrThrow, throwIfRefused } from "./problem.ts";

/** A role as the admin sees it: a person's whole access (D243 §4). */
export type Role = components["schemas"]["RoleBody"];

/** A role as the admin describes it: its libraries are taken as given. */
export type RoleChange = components["schemas"]["RoleChangeBody"];

/** Every role, and the capabilities a role may have. */
export async function listRoles(
  api: Api,
): Promise<{ readonly roles: Role[]; readonly capabilities: string[] }> {
  const listed = dataOrThrow(await api.GET("/roles"));
  return { roles: listed.roles ?? [], capabilities: listed.capabilities ?? [] };
}

/** Makes a role. */
export async function createRole(
  api: Api,
  change: RoleChange,
  idempotencyKey: string,
): Promise<number> {
  return dataOrThrow(await api.POST("/roles", { body: change, ...idempotent(idempotencyKey) })).id;
}

/** Changes a role: a library left out is gone for its members at their next request. */
export async function updateRole(api: Api, roleId: number, change: RoleChange): Promise<void> {
  throwIfRefused(await api.PUT("/roles/{id}", { params: { path: { id: roleId } }, body: change }));
}

/** Deletes a role the admin made that no one holds. */
export async function deleteRole(api: Api, roleId: number): Promise<void> {
  throwIfRefused(await api.DELETE("/roles/{id}", { params: { path: { id: roleId } } }));
}

/** Gives a person a role, from their next request. */
export async function setPersonsRole(api: Api, userId: number, role: string): Promise<void> {
  throwIfRefused(
    await api.PUT("/users/{id}/role", { params: { path: { id: userId } }, body: { role } }),
  );
}

/** A role's name as a person reads it: the built-in ones in words, the admin's own as named. */
export function roleLabel(t: TFunction, name: string): string {
  if (name === "admin") {
    return t("people.roleAdmin");
  }
  return name === "user" ? t("people.roleUser") : name;
}

/** The catalogue's key for a capability's words: its wire name in camelCase. */
export function capabilityKey(capability: string): string {
  return `roles.capability.${capability.replace(/_([a-z])/gu, (_, letter: string) => letter.toUpperCase())}`;
}
