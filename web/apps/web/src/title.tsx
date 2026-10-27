// SPDX-License-Identifier: MPL-2.0

import { BackButton } from "./back-button.tsx";
import { BusyGroup } from "./busy-group.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import "./title.css";

import type { components } from "@orpheum/sdk";
import { useEffect, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CastToDevice } from "./cast-to.tsx";

import { camelKey } from "./catalogue-key.ts";
import { languageName } from "./language-name.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { dataOrThrow, throwIfRefused } from "./problem.ts";
import { FilingReportButton } from "./filing-report.tsx";
import { MatchCorrection } from "./match-correction.tsx";
import { StatusNotes } from "./status-cues.tsx";
import { Poster, WideArt } from "./poster.tsx";
import { Link } from "./link.tsx";
import type { Route } from "./router.ts";
import { usePageInterest } from "./page-interest.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";
import { watchingOf } from "./watching.ts";
import { firstPlayableFile } from "./first-playable.ts";
import { Loaded } from "./loaded.tsx";
import { groupName, placeCode } from "./episode-place.ts";
import { Remembered, useRemembered } from "./remembered.ts";

// Where they were before a screen that shows no title used them: a title's page loads with the player
// (later-screens.tsx), and they do not (the audit of 2026-10-25, W1).
export { firstPlayableFile, Loaded };

/** A title's detail, as the server sends it. */
export type TitleDetail = components["schemas"]["ItemDetailOutputBody"];
type Stream = components["schemas"]["StreamBody"];
type EpisodePlace = components["schemas"]["EpisodePlaceBody"];

/** A stream in a line: its kind, codec, language and channels, where it has them. */
export function StreamLine(props: { readonly stream: Stream }): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { stream } = props;
  const codec = stream.codec ?? "";
  const language =
    stream.language === undefined || stream.language === ""
      ? ""
      : languageName(stream.language, i18n.language);
  switch (stream.kind) {
    case "video":
      return <li>{t("title.streamVideo", { codec })}</li>;
    case "audio": {
      const channels =
        stream.channelCount === undefined
          ? ""
          : t("title.channels", { count: stream.channelCount });
      return <li>{t("title.streamAudio", { codec, language, channels })}</li>;
    }
    case "subtitle":
      return <li>{t("title.streamSubtitle", { codec, language })}</li>;
    default:
      return null;
  }
}

/** A title's page: its art, name, year and length, what it is about, its versions and extras. */
export function TitleView(props: {
  readonly title: TitleDetail;
  /** Says one file that cannot be read now is gone for good (D241 §5); an admin's. */
  readonly onRetireFile?: (fileId: number) => void;
  /** Told a film's match was corrected, an admin's (D301 §2): the page reads it again. */
  readonly onMatchCorrected?: () => void;
}): JSX.Element {
  const { title } = props;
  return (
    <article className="title">
      <WideArt
        sourceSha256={title.art.backdropSha256}
        size="landscape-l"
        className="title__backdrop"
      />
      <div className="title__head">
        <TitleArt title={title} />
        <TitleFacts title={title} />
      </div>
      {title.kind === "movie" && props.onMatchCorrected !== undefined ? (
        <MatchCorrection itemId={title.id} onCorrected={props.onMatchCorrected} />
      ) : null}
      {title.kind === "extra" ? null : <FilingReportButton itemId={title.id} />}
      <StatusNotes subject={title} />
      <Versions
        versions={title.versions ?? []}
        isPlayable={title.unavailable === undefined}
        {...(props.onRetireFile === undefined ? {} : { onRetireFile: props.onRetireFile })}
      />
      <Extras extras={title.extras ?? []} />
    </article>
  );
}

/** A film's poster, or an episode's still. */
function TitleArt(props: { readonly title: TitleDetail }): JSX.Element | null {
  const { art, kind } = props.title;
  if (kind !== "episode") {
    return <Poster sourceSha256={art.posterSha256} />;
  }
  return art.stillSha256 === undefined ? null : (
    <WideArt sourceSha256={art.stillSha256} size="landscape-m" className="title__still" />
  );
}

/**
 * An episode's series and its place, each a link up: one press from an episode reached by search or
 * Continue Watching to its season (research/back-navigation-2026-10-24.md). In an ordering the person
 * chose over aired, the place is its group there, "Disc 2 · 3", with its aired place as a second line,
 * the number other sites use; one that ordering leaves out says so (episode-orderings.md §7 Q8).
 */
function EpisodePlaceFacts(props: { readonly place: EpisodePlace }): JSX.Element {
  const { t } = useTranslation();
  const { place } = props;
  const series = (
    <Link to={{ screen: "series-page", collectionId: place.seriesId }}>{place.seriesName}</Link>
  );
  if (place.ordering === undefined) {
    return (
      <p>
        {series}
        {t("title.placeBetween")}
        <Link
          to={{ screen: "series-page", collectionId: place.seriesId, season: place.seasonNumber }}
        >
          {t("title.placeSeason", { season: place.seasonNumber })}
        </Link>
        {t("title.placeEpisode", { number: place.episodeNumber })}
      </p>
    );
  }
  const inOrdering = place.inOrdering;
  return (
    <>
      <p>
        {series}
        {t("title.placeBetween")}
        {inOrdering === undefined ? (
          t("ordering.notInThisOrderNote")
        ) : (
          <InOrdering place={place} inOrdering={inOrdering} />
        )}
      </p>
      <p className="title__aired-place">
        {t("ordering.airedPlace", { season: place.seasonNumber, number: place.episodeNumber })}
      </p>
    </>
  );
}

/**
 * An episode's place in the ordering its person chose: "Episode 812" in one run, else its group
 * there, a link up to it, and its number in it.
 */
function InOrdering(props: {
  readonly place: EpisodePlace;
  readonly inOrdering: NonNullable<EpisodePlace["inOrdering"]>;
}): JSX.Element {
  const { t } = useTranslation();
  const { place, inOrdering } = props;
  if (inOrdering.isOnlyGroup === true) {
    return <>{placeCode(t, place.ordering, inOrdering)}</>;
  }
  const group = { number: inOrdering.groupNumber, name: inOrdering.groupName };
  return (
    <>
      <Link to={{ screen: "series-page", collectionId: place.seriesId, season: group.number }}>
        {groupName(t, place.ordering, group)}
      </Link>
      {t("title.placeBetween")}
      {inOrdering.numberInGroup}
    </>
  );
}

/** A title's name, its place in its series, its year and length, and what it is about. */
function TitleFacts(props: { readonly title: TitleDetail }): JSX.Element {
  const { t } = useTranslation();
  const { title } = props;
  const episodePlace = title.episode;
  return (
    <div>
      <h1>{title.name}</h1>
      {episodePlace === undefined ? null : <EpisodePlaceFacts place={episodePlace} />}
      <p className="title__facts">
        {title.year === undefined ? null : <span>{title.year}</span>}
        {title.runtimeMs === undefined ? null : (
          <span>{t("title.runtime", { minutes: Math.round(title.runtimeMs / 60_000) })}</span>
        )}
        {title.identificationState === "unidentified" ? (
          <span>{t("title.unidentified")}</span>
        ) : null}
      </p>
      {title.tagline === undefined ? null : <p className="title__tagline">{title.tagline}</p>}
      {title.overview === undefined ? null : <p>{title.overview}</p>}
    </div>
  );
}

/**
 * A title's versions, each file with its path (for an admin) and its streams, and a way to play it
 * unless none can be played now (D241 §3).
 */
function Versions(props: {
  readonly versions: readonly components["schemas"]["VersionBody"][];
  readonly isPlayable: boolean;
  readonly onRetireFile?: (fileId: number) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { versions } = props;
  if (versions.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="title-versions">
      <h2 id="title-versions">{t("title.versions")}</h2>
      {versions.map((version, index) => (
        <div key={version.id}>
          {versions.length === 1 ? null : <h3>{t("title.version", { index: index + 1 })}</h3>}
          {(version.files ?? []).map((file) => (
            <div key={file.id} className="title__file">
              {props.isPlayable ? (
                <Link to={{ screen: "play", fileId: file.id }} className="title__play">
                  {t("title.play")}
                </Link>
              ) : null}
              {/* What the file is, for whoever wants it, kept out of the way (the owner, 2026-10-06). */}
              <details className="title__details">
                <summary>{t("title.fileDetails")}</summary>
                {file.path === undefined ? null : <code>{file.path}</code>}
                <ul>
                  {(file.streams ?? []).map((stream) => (
                    <StreamLine key={stream.id} stream={stream} />
                  ))}
                </ul>
              </details>
              {/* Cast before anything plays, from where the person left it (D337; §9 Q1). */}
              {props.isPlayable ? (
                <CastToDevice fileId={file.id} positionMs={() => undefined} />
              ) : null}
              {/* An admin sees the path, and says a file that cannot be read now is gone (D241 §5). */}
              {props.isPlayable ||
              file.path === undefined ||
              props.onRetireFile === undefined ? null : (
                <ConfirmButton
                  label={t("title.retireFile")}
                  consequence={t("title.retireFileConsequence")}
                  confirmLabel={t("title.retireFileConfirm")}
                  onConfirm={() => props.onRetireFile?.(file.id)}
                />
              )}
            </div>
          ))}
        </div>
      ))}
    </section>
  );
}

/** A title's extras, each leading to its own page. */
function Extras(props: {
  readonly extras: readonly components["schemas"]["ExtraBody"][];
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.extras.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="title-extras">
      <h2 id="title-extras">{t("title.extras")}</h2>
      <ul>
        {props.extras.map((extra) => (
          <li key={extra.id}>
            <Link to={{ screen: "title", itemId: extra.id }}>{extra.name}</Link>{" "}
            <span>
              {t(`extraType.${camelKey(extra.extraType)}`, {
                defaultValue: t("extraType.unknown"),
              })}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * The page above a title's: an episode's season, else the films. An extra names no film it belongs
 * to, so its Back goes up to the films only when the history has no page of this app's before it.
 */
export function titleUp(title: TitleDetail | undefined): Route {
  const place = title?.episode;
  return place === undefined ? { screen: "films" } : episodeUp(place);
}

/**
 * The page above an episode: its season in the aired order, else its group in the ordering the
 * person chose, the series' top where that ordering leaves it out.
 */
export function episodeUp(place: EpisodePlace): Route {
  if (place.ordering === undefined) {
    return { screen: "series-page", collectionId: place.seriesId, season: place.seasonNumber };
  }
  // In an ordering the person chose, its group there; the series' top for one it leaves out.
  return place.inOrdering === undefined
    ? { screen: "series-page", collectionId: place.seriesId }
    : { screen: "series-page", collectionId: place.seriesId, season: place.inOrdering.groupNumber };
}

/** The titles' pages this tab has shown. */
const rememberedTitles = new Remembered<TitleDetail>();

/** A title's page, loaded by its id. Opening it warms up its first file (D258 §1). */
export function Title(props: { readonly id: number }): JSX.Element {
  const { api, playableFormats } = useServices();
  const [loaded, reload] = useResource(
    `title/${String(props.id)}`,
    async (signal) =>
      dataOrThrow(
        await api.GET("/items/{id}", {
          params: { path: { id: props.id } },
          signal: signal ?? null,
        }),
      ),
    useBackgroundChangesPoller(),
  );
  const title = useRemembered(`title/${String(props.id)}`, loaded, rememberedTitles);
  usePageInterest(props.id);
  const warmFileId = title.state === "loaded" ? firstPlayableFile(title.value) : undefined;
  useEffect(() => {
    if (warmFileId !== undefined) {
      watchingOf(api).warmUp(warmFileId, playableFormats());
    }
  }, [api, playableFormats, warmFileId]);
  // A refusal shows the title as the server has it, as success does.
  const retiring = useRequest(() => "");
  const retireFile = (fileId: number): void => {
    retiring.runRequest(async (idempotencyKey) => {
      try {
        throwIfRefused(
          await api.POST("/files/{id}/retire", {
            params: { path: { id: fileId } },
            ...idempotent(idempotencyKey),
          }),
        );
      } finally {
        reload();
      }
    });
  };
  return (
    <>
      <BackButton up={titleUp(title.state === "loaded" ? title.value : undefined)} />
      <BusyGroup isBusy={retiring.isBusy}>
        <Loaded
          resource={title}
          render={(value) => (
            <TitleView title={value} onRetireFile={retireFile} onMatchCorrected={reload} />
          )}
        />
      </BusyGroup>
    </>
  );
}
