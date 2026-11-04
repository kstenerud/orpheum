// SPDX-License-Identifier: MPL-2.0

import { reportFailure, type components } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { chipsOf, decades } from "./list-filters.ts";
import { dataOrThrow } from "./problem.ts";
import { ActionChooser } from "./route-chooser.tsx";
import type { ListFilters } from "./router.ts";
import { useServices } from "./services.tsx";

/** A genre the chip offers, as GET /genres names it. */
type Genre = components["schemas"]["GenreBody"];

/**
 * Films' or Series' chips (search-by-more §4.3): a genre from those the list's titles have, a decade,
 * and whether to keep only what the person has not watched. Each is a button, so a remote reaches
 * them with no typing; each choice is told whole, the page keeping it in the address.
 */
export function ListChips(props: {
  readonly kind: "movie" | "series";
  readonly filters: ListFilters;
  readonly onChange: (next: ListFilters) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { filters, onChange } = props;
  return (
    <div className="list-chips" data-focus-container="">
      <GenreChip
        kind={props.kind}
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
    </div>
  );
}

/** The genre chip: the genres GET /genres names for the list's kind, and Any genre first. */
function GenreChip(props: {
  readonly kind: "movie" | "series";
  readonly genre: number | undefined;
  readonly onChoose: (genre: number | undefined) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  // Asked for when first wanted, its list opened or a genre named by the address, not each time the
  // list shows: most visits never open it.
  const [isWanted, setIsWanted] = useState(props.genre !== undefined);
  const [genres, setGenres] = useState<readonly Genre[]>([]);
  const { kind } = props;
  useEffect(() => {
    if (!isWanted) {
      return undefined;
    }
    let isCurrent = true;
    api
      .GET("/genres", { params: { query: { kind } } })
      .then((answer) => {
        if (isCurrent) {
          setGenres(dataOrThrow(answer).genres ?? []);
        }
      })
      .catch(reportFailure);
    return () => {
      isCurrent = false;
    };
  }, [api, kind, isWanted]);
  const current = genres.find((genre) => genre.id === props.genre);
  return (
    <ActionChooser
      fieldLabel={t("chips.genre")}
      onOpenChange={(isOpen) => {
        if (isOpen) {
          setIsWanted(true);
        }
      }}
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
        ...genres.map((genre) => ({
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
