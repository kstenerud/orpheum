// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { ButtonByRole } from "./button-by-role.tsx";
import { chipsOf, decades } from "./list-filters.ts";
import { ActionChooser } from "./route-chooser.tsx";
import type { ListFilters } from "./router.ts";

/** A genre the chip offers, as a list's first page names it (GET /items' genres, D480 §1). */
export type Genre = components["schemas"]["GenreBody"];

/** The kinds of list that have genres to choose among: films and series. */
export type ChipsKind = "film" | "series";

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
      <ButtonByRole
        className="list-chips__toggle"
        aria-pressed={filters.isUnwatchedOnly === true}
        onClick={() => {
          onChange(chipsOf({ ...filters, isUnwatchedOnly: filters.isUnwatchedOnly !== true }));
        }}
      >
        {t("chips.unwatched")}
      </ButtonByRole>
    </>
  );
}

/** The genre chip: the genres the list's first page names (useListCriteria), and Any genre first. */
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
