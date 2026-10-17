// SPDX-License-Identifier: MPL-2.0

/**
 * The core's client's files as the loader hands them to its service worker (D308 §6): each one's
 * path and SHA-256 from the manifest the core signed, which the worker holds every file it answers to.
 */

import { base64url } from "@orpheum/sdk";

/** One file of the core's client, as its manifest names it (webclient.ManifestFile). */
export interface ClientFile {
  readonly path: string;
  readonly size: number;
  readonly sha256: string;
}

/** The manifest of the core's client (webclient.Manifest). */
export interface ClientManifest {
  readonly version: string;
  readonly script: string;
  readonly files: readonly ClientFile[];
}

/** The loader's word to its worker: the files of the client it is about to run. */
export interface ClientFilesMessage {
  readonly kind: "orpheum-client-files";
  readonly files: readonly ClientFile[];
}

/** Where the client's files are loaded from: Vite's assets, apart from the loader's own. */
export const clientAssetsPrefix = "/assets/";

function isClientFile(value: unknown): value is ClientFile {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  return (
    typeof Reflect.get(value, "path") === "string" &&
    typeof Reflect.get(value, "size") === "number" &&
    typeof Reflect.get(value, "sha256") === "string"
  );
}

/** The files a manifest's JSON names, or undefined for one that is not a manifest. */
export function manifestOf(text: string): ClientManifest | undefined {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return undefined;
  }
  if (typeof parsed !== "object" || parsed === null) {
    return undefined;
  }
  const version: unknown = Reflect.get(parsed, "version");
  const script: unknown = Reflect.get(parsed, "script");
  const files: unknown = Reflect.get(parsed, "files");
  if (
    typeof version !== "string" ||
    typeof script !== "string" ||
    !Array.isArray(files) ||
    !files.every((file) => isClientFile(file))
  ) {
    return undefined;
  }
  return { version, script, files };
}

/** Whether message is the loader's word on the client's files. */
export function isClientFilesMessage(message: unknown): message is ClientFilesMessage {
  if (typeof message !== "object" || message === null) {
    return false;
  }
  const files: unknown = Reflect.get(message, "files");
  return (
    Reflect.get(message, "kind") === "orpheum-client-files" &&
    Array.isArray(files) &&
    files.every((file) => isClientFile(file))
  );
}

/** The SHA-256 of bytes in unpadded base64url, as a manifest names a file's. */
export async function sha256Of(bytes: ArrayBuffer): Promise<string> {
  return base64url(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes)));
}
