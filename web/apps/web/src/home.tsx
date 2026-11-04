// SPDX-License-Identifier: MPL-2.0

import "./home.css";

import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { EntryPoster, type BrowseKind, type Entry } from "./browse.tsx";
import { ContinueWatching } from "./continue-watching.tsx";
import { useLibrariesReached } from "./libraries-reached.ts";
import { Link } from "./link.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { PosterRow } from "./poster-row.tsx";
import { dataOrThrow } from "./problem.ts";
import type { Route } from "./router.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";
import { IsAdminContext } from "./warning-sign.tsx";
import { useWarmsPlay } from "./warm-play.ts";

/**
 * How many of the newest films or series a home row shows, a best guess: three screens of a Fire TV's
 * row of six, the rest a list's own page away. A justified constant.
 */
export const newestShown = 18;

/**
 * Home (the owner, 2026-10-25: "an actual home page", which "should list half-finished stuff and also
 * newly added things. We'll also add more things later"): Continue Watching, then the newest films
 * and the newest series, each a row of posters. A row with nothing in it is not shown; with no library
 * at all, it says so (NoLibrariesYet). Once it has settled it warms the Play page's code under a loader
 * (warm-play.ts).
 */
export function Home(): JSX.Element {
  const { t } = useTranslation();
  useWarmsPlay();
  return (
    <section className="home" aria-labelledby="home-heading">
      <h1 id="home-heading" className="home__heading">
        {t("home.heading")}
      </h1>
      <NoLibrariesYet />
      <ContinueWatching />
      <NewestRow kind="movie" />
      <NewestRow kind="series" />
    </section>
  );
}

/**
 * Where the person reaches no library, what to do: an admin, who sees every library, is led to make
 * one; anyone else is told to ask (staging's Home before its libraries were made was blank, 2026-11-01).
 */
function NoLibrariesYet(): JSX.Element | null {
  const { t } = useTranslation();
  const isAdmin = use(IsAdminContext);
  const libraries = useLibrariesReached();
  if (libraries.state !== "loaded" || libraries.value.length > 0) {
    return null;
  }
  if (!isAdmin) {
    return <p className="home__empty">{t("browse.noLibrariesAtAll")}</p>;
  }
  return (
    <p className="home__empty">
      {t("home.noLibrariesAdmin")}{" "}
      <Link to={{ screen: "new-library" }}>{t("home.makeLibrary")}</Link>
    </p>
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
      // A row of Home: of the libraries the person keeps on Home (D366 §5).
      const query = { kind, sort: "added" as const, limit: newestShown, home: true };
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
      <div className="home__row-heading">
        <h2 id={headingId}>{t(kind === "movie" ? "home.newestFilms" : "home.newestSeries")}</h2>
        <Link
          to={newestListRoute(kind)}
          label={t(kind === "movie" ? "home.seeAllFilms" : "home.seeAllSeries")}
        >
          {t("home.seeAll")}
        </Link>
      </div>
      <PosterRow isArrival>
        {entries.value.map((entry: Entry) => (
          <EntryPoster key={entry.id} kind={kind} entry={entry} />
        ))}
      </PosterRow>
    </section>
  );
}

/**
 * Where a newest row's See all leads (the owner, 2026-11-04): Films or Series sorted by date added,
 * which runs newest first, the row's own order, past the row's newestShown.
 */
export function newestListRoute(kind: BrowseKind): Route {
  return { screen: kind === "movie" ? "films" : "series", sort: "added" };
}
