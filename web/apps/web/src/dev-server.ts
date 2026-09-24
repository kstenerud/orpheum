// SPDX-License-Identifier: MPL-2.0

/**
 * The Go server Vite's dev server proxies to, from ORPHEUM_DEV_SERVER, which `make dev` sets.
 * Serving without it is refused rather than guessed (D204): a guessed address proxies to
 * whatever else is listening there. A production build proxies nothing and needs no address.
 */
export function devServer(
  value: string | undefined,
  command: "build" | "serve",
): string | undefined {
  if (command === "build") {
    return undefined;
  }
  if (value === undefined || value.trim() === "") {
    throw new Error(
      "ORPHEUM_DEV_SERVER is not set: start the web client with `make dev`, which sets it to " +
        "the Go server it started (D204)",
    );
  }
  return value.trim();
}
