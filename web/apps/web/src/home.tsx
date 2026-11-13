// SPDX-License-Identifier: MPL-2.0

import "./home.css";

import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { EntryPoster, type BrowseKind, type Entry } from "./browse.tsx";
import { ContinueWatching, loadHome, type HomeRows } from "./continue-watching.tsx";
import { useLibrariesReached } from "./libraries-reached.ts";
import { Link } from "./link.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { PosterRow } from "./poster-row.tsx";
import type { Route } from "./router.ts";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";
import { IsAdminContext } from "./warning-sign.tsx";
import { useWarmsPlay } from "./warm-play.ts";

/**
 * Home (the owner, 2026-10-25: "an actual home page", which "should list half-finished stuff and also
 * newly added things. We'll also add more things later"): Continue Watching, then the newest films
 * and the newest series, each a row of posters. A row with nothing in it is not shown; with no library
 * at all, it says so (NoLibrariesYet). Every row comes in one read of GET /home, which one poll keeps
 * (D476, principles §30). Once it has settled it warms the Play page's code under a loader
 * (warm-play.ts).
 */
export function Home(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [home, readHomeAgain] = useResource("home", (signal) => loadHome(api, signal), {
    poller: useBackgroundChangesPoller(),
  });
  useWarmsPlay();
  return (
    <section className="home" aria-labelledby="home-heading">
      <h1 id="home-heading" className="home__heading">
        {t("home.heading")}
      </h1>
      <NoLibrariesYet />
      <ContinueWatching home={home} reload={readHomeAgain} />
      <NewestRow kind="film" home={home} />
      <NewestRow kind="series" home={home} />
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

/**
 * The newest films or series, newest first, as Home's read (home) has them from the libraries the
 * person keeps on Home (D366 §5): a row of posters, each leading to its page.
 */
function NewestRow(props: {
  readonly kind: BrowseKind;
  readonly home: Resource<HomeRows>;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { kind, home } = props;
  if (home.state !== "loaded") {
    return null;
  }
  const entries = (kind === "film" ? home.value.newestFilms : home.value.newestSeries) ?? [];
  if (entries.length === 0) {
    return null;
  }
  const headingId = `home-newest-${kind}`;
  return (
    <section className="home__row" aria-labelledby={headingId}>
      <div className="home__row-heading">
        <h2 id={headingId}>{t(kind === "film" ? "home.newestFilms" : "home.newestSeries")}</h2>
        <Link
          to={newestListRoute(kind)}
          label={t(kind === "film" ? "home.seeAllFilms" : "home.seeAllSeries")}
        >
          {t("home.seeAll")}
        </Link>
      </div>
      <PosterRow isArrival>
        {entries.map((entry: Entry) => (
          <EntryPoster key={entry.id} kind={kind} entry={entry} />
        ))}
      </PosterRow>
    </section>
  );
}

/**
 * Where a newest row's See all leads (the owner, 2026-11-04): Films or Series sorted by date added,
 * which runs newest first, the row's own order, past the row's last.
 */
export function newestListRoute(kind: BrowseKind): Route {
  return { screen: kind === "film" ? "films" : "series", sort: "added" };
}
