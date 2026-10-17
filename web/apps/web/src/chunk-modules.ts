// SPDX-License-Identifier: MPL-2.0

/**
 * The browser map's record of what each built script holds (scripts/browser_map.py): every file of
 * the repository's own that went into each chunk of a build, by the chunk's file name. With the
 * scripts each browser test was seen to load (web/e2e/browser-map.ts), revert-red runs a source
 * file's revert in only the tests that loaded a chunk holding it. Written by the build only when
 * ORPHEUM_BROWSER_MAP_DIR is set (vite.config.ts), as check-pr's full browser run sets it.
 */

/** One file of a build's output, as Rollup hands it to a plugin: a chunk lists its modules. */
export type BuiltFile =
  { readonly type: "chunk"; readonly moduleIds: readonly string[] } | { readonly type: "asset" };

/**
 * Each chunk's own files: the repository's, relative to its root, without a query (`?client`,
 * `?worker`). A dependency's (node_modules) and Vite's virtual modules ("\0…") are no file a
 * branch changes, and are left out.
 */
export function chunkModules(
  bundle: Readonly<Record<string, BuiltFile>>,
  repoRoot: string,
): Record<string, string[]> {
  const prefix = repoRoot.endsWith("/") ? repoRoot : `${repoRoot}/`;
  const chunks: Record<string, string[]> = {};
  for (const [fileName, file] of Object.entries(bundle)) {
    if (file.type !== "chunk") {
      continue;
    }
    const own = file.moduleIds
      .map((id) => id.split("?")[0] ?? id)
      .filter((id) => id.startsWith(prefix) && !id.includes("/node_modules/"))
      .map((id) => id.slice(prefix.length));
    chunks[fileName] = [...new Set(own)].toSorted();
  }
  return chunks;
}
