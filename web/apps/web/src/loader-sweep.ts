// SPDX-License-Identifier: MPL-2.0

/**
 * What the loader clears at its start (D308 §7): every cookie it can see, since no page the loader
 * serves sets or reads one, so any there was set by another core's page on the same site; and, at the
 * loader's own origin, every kept key it does not own, which an older release left: above all the
 * sessions the whole client once kept there, which `app.` may no longer hold (D308 §3).
 */

/**
 * Clears every cookie page can see: for its own host and for each domain above it, since another
 * core's page may have set one for the whole site, and at the root path, where one would be sent
 * with every load.
 */
export function clearCookies(
  cookies: { readonly read: () => string; readonly write: (cookie: string) => void },
  hostname: string,
): void {
  const names = cookies
    .read()
    .split(";")
    .map((part) => part.split("=")[0]?.trim() ?? "")
    .filter((name) => name !== "");
  const labels = hostname.split(".");
  const domains = labels.slice(0, -1).map((_, index) => labels.slice(index).join("."));
  for (const name of names) {
    const expired = `${name}=; Max-Age=0; Path=/`;
    cookies.write(expired);
    for (const domain of domains) {
      cookies.write(`${expired}; Domain=${domain}`);
    }
  }
}
