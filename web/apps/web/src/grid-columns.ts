// SPDX-License-Identifier: MPL-2.0

import { type RefObject, useLayoutEffect, useState } from "react";

/** How many posters a row of the grid holds, by its laid-out columns; one where it has none. */
export function columnsOf(grid: HTMLElement): number {
  const columns = getComputedStyle(grid).gridTemplateColumns.split(" ").filter(Boolean).length;
  return Math.max(1, columns);
}

/** The grid's columns, read as it is laid out and again each time it changes size; one before. */
export function useGridColumns(gridRef: RefObject<HTMLElement | null>): number {
  const [columns, setColumns] = useState(1);
  useLayoutEffect(() => {
    const grid = gridRef.current;
    if (grid === null) {
      return undefined;
    }
    const read = (): void => {
      setColumns(columnsOf(grid));
    };
    read();
    if (typeof ResizeObserver === "undefined") {
      return undefined;
    }
    const resizes = new ResizeObserver(read);
    resizes.observe(grid);
    return () => {
      resizes.disconnect();
    };
  }, [gridRef]);
  return columns;
}

/**
 * For each card, by its place, whether its row holds an upright poster, so a 16:9 still in it
 * stands centred in a poster's height (D495 §4): a row of stills alone is as tall as its stills.
 * isUpright says, by place, which cards are upright; columns is how many a row holds.
 */
export function rowsWithPosters(
  isUpright: readonly boolean[],
  columns: number,
): readonly boolean[] {
  const perRow = Math.max(1, Math.floor(columns));
  const rows: boolean[] = [];
  for (let start = 0; start < isUpright.length; start += perRow) {
    const row = isUpright.slice(start, start + perRow);
    const hasPoster = row.includes(true);
    rows.push(...row.map(() => hasPoster));
  }
  return rows;
}
