// SPDX-License-Identifier: MPL-2.0

import { allPages, type components, reportFailure } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { chipsOf, decades } from "./list-filters.ts";
import { dataOrThrow } from "./problem.ts";
import { ActionChooser } from "./route-chooser.tsx";
import type { ListFilters } from "./router.ts";
import { useServices } from "./services.tsx";

/** A genre the chip offers, as GET /genres names it. */
export type Genre = components["schemas"]["GenreBody"];

/** The kinds of list that have genres to choose among: films and series. */
export type ChipsKind = "movie" | "series";

/**
 * The genres GET /genres names for a list's kind, asked for once wanted (isWanted): the list's pop-up
 * opened, or a genre named by the address, whose name its button shows; not each time the list shows,
 * as most visits never open it. None while unread, and for a list with no chips (kind undefined).
 */
export function useGenres(kind: ChipsKind | undefined, isWanted: boolean): readonly Genre[] {
  const { api } = useServices();
  const [genres, setGenres] = useState<readonly Genre[]>([]);
  useEffect(() => {
    if (!isWanted || kind === undefined) {
      return undefined;
    }
    let isCurrent = true;
    allPages(async (query) => {
      const page = dataOrThrow(await api.GET("/genres", { params: { query: { kind, ...query } } }));
      return { entries: page.genres ?? [], nextCursor: page.nextCursor };
    })
      .then((every) => {
        if (isCurrent) {
          setGenres(every);
        }
      })
      .catch(reportFailure);
    return () => {
      isCurrent = false;
    };
  }, [api, kind, isWanted]);
  return genres;
}

/**
 * A list's chips (search-by-more §4.3), in its pop-up (D417): a genre from those the list's titles
 * have, a decade, and whether to keep only what the person has not watched. Each is a button, so a
 * remote reaches them with no typing; each choice is told whole, the page keeping it.
 */
export function ListChips(props: {
  readonly genres: readonly Genre[];
  readonly filters: ListFilters;
  readonly onChange: (next: ListFilters) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { filters, onChange } = props;
  return (
    <>
      <GenreChip
        genres={props.genres}
        genre={filters.genre}
        onChoose={(genre) => {
          onChange(chipsOf({ ...filters, genre }));
        }}
      />
      <ActionChooser
        fieldLabel={t("chips.decade")}
        current={
          filters.decade === undefined
            ? t("chips.anyDecade")
            : t("chips.decadeName", { decade: filters.decade })
        }
        options={[undefined, ...decades].map((decade) => ({
          key: String(decade ?? "any"),
          label: decade === undefined ? t("chips.anyDecade") : t("chips.decadeName", { decade }),
          isCurrent: decade === filters.decade,
          onChoose: () => {
            onChange(chipsOf({ ...filters, decade }));
          },
        }))}
      />
      <button
        type="button"
        className="list-chips__toggle"
        aria-pressed={filters.isUnwatchedOnly === true}
        onClick={() => {
          onChange(chipsOf({ ...filters, isUnwatchedOnly: filters.isUnwatchedOnly !== true }));
        }}
      >
        {t("chips.unwatched")}
      </button>
    </>
  );
}

/** The genre chip: the list's kind's genres (useGenres), and Any genre first. */
function GenreChip(props: {
  readonly genres: readonly Genre[];
  readonly genre: number | undefined;
  readonly onChoose: (genre: number | undefined) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const current = props.genres.find((genre) => genre.id === props.genre);
  return (
    <ActionChooser
      fieldLabel={t("chips.genre")}
      current={current?.name ?? t("chips.anyGenre")}
      options={[
        {
          key: "any",
          label: t("chips.anyGenre"),
          isCurrent: props.genre === undefined,
          onChoose: () => {
            props.onChoose(undefined);
          },
        },
        ...props.genres.map((genre) => ({
          key: String(genre.id),
          label: genre.name,
          isCurrent: genre.id === props.genre,
          onChoose: () => {
            props.onChoose(genre.id);
          },
        })),
      ]}
    />
  );
}
