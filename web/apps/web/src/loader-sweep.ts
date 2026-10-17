// SPDX-License-Identifier: MPL-2.0

/**
 * What the loader clears at its start (D308 §7): every cookie it can see, since no page the loader
 * serves sets or reads one, so any there was set by another core's page on the same site; and, at the
 * loader's own origin, every kept key it does not own, which an older release left: above all the
 * sessions the whole client once kept there, which `app.` may no longer hold (D308 §3).
 */

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";

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

/** The prefix of every key a release of Orpheum keeps. */
const keptPrefix = "orpheum.";

/** Every key storage holds. */
function keysOf(storage: Storage): string[] {
  const keys: string[] = [];
  for (let index = 0; index < storage.length; index += 1) {
    const key = storage.key(index);
    if (key !== null) {
      keys.push(key);
    }
  }
  return keys;
}

/**
 * Removes every key under Orpheum's prefix that owned does not name: what an older release kept,
 * read by none now. A key of another prefix is not Orpheum's to remove.
 */
export function removeUnownedKeys(
  storage: Storage | undefined,
  owned: (key: string) => boolean,
): void {
  if (storage === undefined) {
    return;
  }
  let keys: string[];
  try {
    keys = keysOf(storage);
  } catch {
    return;
  }
  const removable: StringStorage = storageThatNeverThrows(storage);
  for (const key of keys) {
    if (key.startsWith(keptPrefix) && !owned(key)) {
      removable.removeItem(key);
    }
  }
}
