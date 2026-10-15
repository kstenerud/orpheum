// SPDX-License-Identifier: MPL-2.0

import { BusyGroup } from "./busy-group.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import "./title.css";

import { artUrl, type components } from "@orpheum/sdk";
import { useEffect, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { languageName } from "./language-name.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { dataOrThrow, problemText, throwIfRefused, unavailableText } from "./problem.ts";
import { GuessedMark, MatchCorrection } from "./match-correction.tsx";
import { Poster } from "./poster.tsx";
import { UnplayableSign } from "./unplayable-sign.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";
import { watchingOf } from "./watching.ts";
import { Remembered, useRemembered } from "./remembered.ts";

/** A title's detail, as the server sends it. */
export type TitleDetail = components["schemas"]["ItemDetailOutputBody"];
type Stream = components["schemas"]["StreamBody"];

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
  const { t } = useTranslation();
  const { title } = props;
  return (
    <article className="title">
      {title.art.backdropSha256 === undefined ? null : (
        <img
          className="title__backdrop"
          src={artUrl(title.art.backdropSha256, "landscape-l")}
          alt={""}
        />
      )}
      <div className="title__head">
        <TitleArt title={title} />
        <TitleFacts title={title} />
      </div>
      {title.kind === "movie" && props.onMatchCorrected !== undefined ? (
        <MatchCorrection itemId={title.id} onCorrected={props.onMatchCorrected} />
      ) : null}
      {title.unavailable === undefined ? null : (
        <output className="title__unavailable">
          <UnplayableSign unavailable={title.unavailable} /> {unavailableText(t, title.unavailable)}
        </output>
      )}
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
    return (
      <span className="poster-frame">
        <Poster sourceSha256={art.posterSha256} />
        <GuessedMark isGuessed={props.title.isGuessed} />
      </span>
    );
  }
  return art.stillSha256 === undefined ? null : (
    <img className="title__still" src={artUrl(art.stillSha256, "landscape-m")} alt={""} />
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
      {episodePlace === undefined ? null : (
        <p>
          <Link to={{ screen: "series-page", collectionId: episodePlace.seriesId }}>
            {t("title.place", {
              series: episodePlace.seriesName,
              season: episodePlace.seasonNumber,
              number: episodePlace.episodeNumber,
            })}
          </Link>
        </p>
      )}
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
 * Shows what a resource holds, or that it is loading, or why it failed: in problemText's words,
 * or in describeProblem's for a screen that knows more about its failures, and that it is asked
 * for again by itself when its poller will.
 */
export function Loaded<T>(props: {
  readonly resource: Resource<T>;
  readonly render: (value: T) => JSX.Element;
  readonly describeProblem?: (cause: unknown) => string;
}): JSX.Element {
  const { t } = useTranslation();
  const { resource } = props;
  if (resource.state === "loading") {
    return <p className="title__loading">{t("common.loading")}</p>;
  }
  if (resource.state === "failed") {
    const describe = props.describeProblem ?? ((cause: unknown) => problemText(t, cause));
    const why = describe(resource.cause);
    return (
      <p role="alert">
        {resource.willAskAgain === true ? `${why} ${t("problem.askingAgain")}` : why}
      </p>
    );
  }
  return props.render(resource.value);
}

/** The file a title's Play goes to first, which its page warms up; undefined if none plays now. */
export function firstPlayableFile(title: TitleDetail): number | undefined {
  if (title.unavailable !== undefined) {
    return undefined;
  }
  return title.versions?.[0]?.files?.[0]?.id;
}

/** The titles' pages this tab has shown. */
const rememberedTitles = new Remembered<TitleDetail>();

/** A title's page, loaded by its id. Opening it warms up its first file (D258 §1). */
export function Title(props: { readonly id: number }): JSX.Element {
  const { api, playableFormats } = useServices();
  const [loaded, reload] = useResource(
    `title/${String(props.id)}`,
    async () => dataOrThrow(await api.GET("/items/{id}", { params: { path: { id: props.id } } })),
    useBackgroundChangesPoller(),
  );
  const title = useRemembered(`title/${String(props.id)}`, loaded, rememberedTitles);
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
    <BusyGroup isBusy={retiring.isBusy}>
      <Loaded
        resource={title}
        render={(value) => (
          <TitleView title={value} onRetireFile={retireFile} onMatchCorrected={reload} />
        )}
      />
    </BusyGroup>
  );
}
