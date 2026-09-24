// SPDX-License-Identifier: MPL-2.0

/**
 * The extra host names Vite's dev server accepts, from ORPHEUM_DEV_ALLOWED_HOSTS: a
 * comma-separated list, for reaching `make dev` on the LAN by a name. Unset, Vite accepts
 * only localhost and IP addresses.
 */
export function devAllowedHosts(value: string | undefined): string[] {
  return (value ?? "")
    .split(",")
    .map((host) => host.trim())
    .filter((host) => host !== "");
}
