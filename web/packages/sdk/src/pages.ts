// SPDX-License-Identifier: MPL-2.0

/** The most entries one page of a list holds: every paged list's limit's maximum (D416 §6). */
export const pageLimitMax = 500;

/** What a paged list is asked for one page: at most limit entries, from cursor, absent for the first. */
export interface PageQuery {
  readonly limit: number;
  readonly cursor?: string;
}

/** One page of a list as the server answers it: its entries, and the next page's cursor, absent on the last. */
export interface ListPage<T> {
  readonly entries: readonly T[];
  readonly nextCursor?: string | undefined;
}

/**
 * Every entry of a paged list, its pages read one after another by their nextCursor until a page
 * has none (D416 §6). load reads the page query asks for, pageLimitMax at a time, and its query is
 * spread into the list's own query. A cursor the server gives twice ends the walk with an error
 * rather than looping.
 */
export async function allPages<T>(load: (query: PageQuery) => Promise<ListPage<T>>): Promise<T[]> {
  const every: T[] = [];
  const seen = new Set<string>();
  let query: PageQuery = { limit: pageLimitMax };
  for (;;) {
    const page = await load(query);
    every.push(...page.entries);
    if (page.nextCursor === undefined || page.nextCursor === "") {
      return every;
    }
    if (seen.has(page.nextCursor)) {
      throw new Error(`the server gave the cursor ${page.nextCursor} twice`);
    }
    seen.add(page.nextCursor);
    query = { limit: pageLimitMax, cursor: page.nextCursor };
  }
}
