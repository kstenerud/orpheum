// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { useCallback, useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { focusAndShow } from "./focus-navigation.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import type { Pages } from "./use-pages.ts";
import { useResource } from "./use-resource.ts";

/** A letter of the alphabet rail pressed, counted, so the same letter pressed again jumps again. */
export interface LetterJump {
  readonly letter: string;
  readonly count: number;
}

/** The letters of the alphabet rail, a list's names' first characters: digits and marks, then A to Z. */
export const railLetters = [
  "#",
  ...Array.from({ length: 26 }, (_, index) => String.fromCodePoint(0x41 + index)),
];

/**
 * A list's alphabet beside its grid, by name only, to jump to a letter (the owner, 2026-10-25): a
 * letter moves the list to its first title, # to the first of all, those beginning with a digit or
 * a mark, a focus container of its own. Given letters (D349 §9.1), only those shown: the ones some
 * title in the list begins with, as the server names them (GET /titles/letters); every letter,
 * undefined, for a list that has not asked.
 */
export function AlphabetRail(props: {
  readonly onLetter: (letter: string) => void;
  readonly letters?: readonly string[] | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const shown =
    props.letters === undefined
      ? railLetters
      : railLetters.filter((letter) => props.letters?.includes(letter) === true);
  return (
    <ol className="alphabet-rail" aria-label={t("browse.alphabet")} data-focus-container="">
      {shown.map((letter) => (
        <li key={letter}>
          <button
            type="button"
            className="alphabet-rail__letter"
            onClick={() => {
              props.onLetter(letter);
            }}
          >
            {letter}
          </button>
        </li>
      ))}
    </ol>
  );
}

/**
 * The alphabet rail's letters for a list, by its kind, library and sort: only those some title in
 * it begins with (D349 §9), as the server names them (`GET /titles/letters`), asked for again
 * with the list's own background poll (D237); undefined while that first answer is loading, for
 * AlphabetRail's own letters prop. Films, Series and the Libraries page all ask this one way.
 */
export function useAlphabetRailLetters(
  kind: "movie" | "series" | "other" | "any",
  library: number | undefined,
  sort: "name" | "file_name",
): readonly string[] | undefined {
  const { api } = useServices();
  const [letters] = useResource(
    `alphabet-rail-letters/${kind}/${String(library)}/${sort}`,
    async () => {
      const body = dataOrThrow(
        await api.GET("/titles/letters", {
          params: { query: { kind, sort, ...(library === undefined ? {} : { library }) } },
        }),
      );
      return body.letters ?? [];
    },
    useBackgroundChangesPoller(),
  );
  return letters.state === "loaded" ? letters.value : undefined;
}

/** Focuses a poster jumped to, its row scrolled to the view's top, clear of the banner. */
export function showAtTop(poster: HTMLElement): void {
  // A test's DOM may have no scrolling at all.
  if (typeof poster.scrollIntoView === "function") {
    poster.scrollIntoView({ block: "start" });
  }
  // Focused as a key would, a remote's press being one; scrolled on, gently, clear of the banner.
  focusAndShow(poster, true);
}

/**
 * Moves a list by name or file name to the first title at or after the letter jump names, as the
 * server sorts them (`from`), # to the first of all: it loads the list's pages up to it, as
 * scrolling would, and focuses its poster at the top of the view, so a remote goes on from there;
 * where no title comes at or after the letter, the last. The newest jump wins over one still
 * loading.
 */
export function useLetterJump<T extends { readonly id: number }>(props: {
  readonly kind: "movie" | "series" | "other" | "any";
  readonly sort: "name" | "file_name";
  readonly library: number | undefined;
  readonly jump: LetterJump | undefined;
  readonly pages: Pages<T>;
  readonly listRef: RefObject<HTMLElement | null>;
}): void {
  const { api } = useServices();
  const { kind, sort, library, jump, listRef } = props;
  const { loadUntil, loadedSoFar } = props.pages;
  // The place of the poster to focus once it is drawn, the last's for -1, undefined for none; the
  // jump found it, the drawing of the pages after it focuses it.
  const toFocusRef = useRef<number | undefined>(undefined);
  const [found, setFound] = useState(0);
  useEffect(() => {
    if (jump === undefined) {
      return undefined;
    }
    let isCurrent = true;
    const from = jump.letter === "#" ? {} : { from: jump.letter };
    const query = { kind, sort, limit: 1, ...(library === undefined ? {} : { library }), ...from };
    api
      .GET("/titles", { params: { query } })
      .then(async (answer) => {
        const first = dataOrThrow(answer).titles?.[0];
        const place = await loadUntil((item) => item.id === first?.id);
        if (isCurrent) {
          toFocusRef.current = place ?? -1;
          setFound((count) => count + 1);
        }
      })
      .catch(reportFailure);
    return () => {
      isCurrent = false;
    };
  }, [api, kind, sort, library, jump, loadUntil]);
  useEffect(() => {
    const place = toFocusRef.current;
    if (place === undefined) {
      return;
    }
    const posters = listRef.current?.querySelectorAll<HTMLElement>(".poster-grid > li > a");
    const poster = place < 0 ? posters?.[posters.length - 1] : posters?.[place];
    if (poster !== undefined) {
      toFocusRef.current = undefined;
      showAtTop(poster);
    }
  }, [found, loadedSoFar, listRef]);
}

/**
 * A letter jump counted again, for the same letter pressed twice in a row to jump again, and a way
 * to clear it: a list remounted by a sort change starts with none, not the last letter pressed in
 * another order.
 */
export function useLetterJumpState(): [
  LetterJump | undefined,
  (letter: string) => void,
  () => void,
] {
  const [jump, setJump] = useState<LetterJump | undefined>(undefined);
  const onLetter = useCallback((letter: string) => {
    setJump((last) => ({ letter, count: (last?.count ?? 0) + 1 }));
  }, []);
  const clear = useCallback(() => {
    setJump(undefined);
  }, []);
  return [jump, onLetter, clear];
}
