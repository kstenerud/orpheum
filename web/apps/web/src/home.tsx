// SPDX-License-Identifier: MPL-2.0

import "./home.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { EntryPoster, type BrowseKind, type Entry } from "./browse.tsx";
import { ContinueWatching } from "./continue-watching.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/**
 * How many of the newest films or series a home row shows, a best guess: three screens of a Fire TV's
 * row of six, the rest a list's own page away. A justified constant.
 */
export const newestShown = 18;

/**
 * Home (the owner, 2026-10-25: "an actual home page", which "should list half-finished stuff and also
 * newly added things. We'll also add more things later"): Continue Watching, then the newest films
 * and the newest series, each a row of posters. A row with nothing in it is not shown.
 */
export function Home(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section className="home" aria-labelledby="home-heading">
      <h1 id="home-heading" className="home__heading">
        {t("home.heading")}
      </h1>
      <ContinueWatching />
      <NewestRow kind="movie" />
      <NewestRow kind="series" />
    </section>
  );
}

/** The newest films or series, newest first, as a row of posters, each leading to its page. */
export function NewestRow(props: { readonly kind: BrowseKind }): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { kind } = props;
  const [entries] = useResource(
    `newest-${kind}`,
    async () => {
      const query = { kind, sort: "added" as const, limit: newestShown };
      return dataOrThrow(await api.GET("/titles", { params: { query } })).titles ?? [];
    },
    useBackgroundChangesPoller(),
  );
  if (entries.state !== "loaded" || entries.value.length === 0) {
    return null;
  }
  const headingId = `home-newest-${kind}`;
  return (
    <section className="home__row" aria-labelledby={headingId}>
      <h2 id={headingId}>{t(kind === "movie" ? "home.newestFilms" : "home.newestSeries")}</h2>
      <ul className="poster-row" data-focus-container="">
        {entries.value.map((entry: Entry) => (
          <EntryPoster key={entry.id} kind={kind} entry={entry} />
        ))}
      </ul>
    </section>
  );
}
