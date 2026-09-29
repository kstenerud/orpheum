// SPDX-License-Identifier: MPL-2.0

import "./title.css";

import { artUrl, type components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { dataOrThrow, problemText } from "./problem.ts";
import { Poster } from "./poster.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/** A title's detail, as the server sends it. */
export type TitleDetail = components["schemas"]["ItemDetailOutputBody"];
type Stream = components["schemas"]["StreamBody"];

/** A stream in a line: its kind, codec, language and channels, where it has them. */
export function StreamLine(props: { readonly stream: Stream }): JSX.Element | null {
  const { t } = useTranslation();
  const { stream } = props;
  const codec = stream.codec ?? "";
  const lang = stream.language ?? "";
  switch (stream.kind) {
    case "video":
      return <li>{t("title.streamVideo", { codec })}</li>;
    case "audio": {
      const channels =
        stream.channels === undefined ? "" : t("title.channels", { count: stream.channels });
      return <li>{t("title.streamAudio", { codec, lang, channels })}</li>;
    }
    case "subtitle":
      return <li>{t("title.streamSubtitle", { codec, lang })}</li>;
    default:
      return null;
  }
}

/** A title's page: its art, name, year and length, what it is about, its versions and extras. */
export function TitleView(props: { readonly title: TitleDetail }): JSX.Element {
  const { title } = props;
  return (
    <article className="title">
      {title.art.backdropHash === undefined ? null : (
        <img
          className="title__backdrop"
          src={artUrl(title.art.backdropHash, "landscape-l")}
          alt={""}
        />
      )}
      <div className="title__head">
        <TitleArt title={title} />
        <TitleFacts title={title} />
      </div>
      <Versions versions={title.versions ?? []} />
      <Extras extras={title.extras ?? []} />
    </article>
  );
}

/** A film's poster, or an episode's still. */
function TitleArt(props: { readonly title: TitleDetail }): JSX.Element | null {
  const { art, kind } = props.title;
  if (kind !== "episode") {
    return <Poster hash={art.posterHash} />;
  }
  return art.stillHash === undefined ? null : (
    <img className="title__still" src={artUrl(art.stillHash, "landscape-m")} alt={""} />
  );
}

/** A title's name, its place in its series, its year and length, and what it is about. */
function TitleFacts(props: { readonly title: TitleDetail }): JSX.Element {
  const { t } = useTranslation();
  const { title } = props;
  const place = title.episode;
  return (
    <div>
      <h1>{title.name}</h1>
      {place === undefined ? null : (
        <p>
          <Link to={{ screen: "collection", id: place.seriesId }}>
            {t("title.place", {
              series: place.seriesName,
              season: place.seasonNumber,
              number: place.episodeNumber,
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

/** A title's versions, each file with its path (for an admin) and its streams. */
function Versions(props: {
  readonly versions: readonly components["schemas"]["VersionBody"][];
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
              <Link to={{ screen: "play", fileId: file.id }} className="title__play">
                {t("title.play")}
              </Link>
              {file.path === undefined ? null : <code>{file.path}</code>}
              <ul>
                {(file.streams ?? []).map((stream) => (
                  <StreamLine key={stream.id} stream={stream} />
                ))}
              </ul>
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
            <Link to={{ screen: "title", id: extra.id }}>{extra.name}</Link>{" "}
            <span>
              {t(`extraType.${camelKey(extra.kind)}`, { defaultValue: t("extraType.unknown") })}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** Shows what a resource holds, or that it is loading, or why it failed. */
export function Loaded<T>(props: {
  readonly resource: Resource<T>;
  readonly render: (value: T) => JSX.Element;
}): JSX.Element {
  const { t } = useTranslation();
  const { resource } = props;
  if (resource.state === "loading") {
    return <p>{t("common.loading")}</p>;
  }
  if (resource.state === "failed") {
    return <p role="alert">{problemText(t, resource.cause)}</p>;
  }
  return props.render(resource.value);
}

/** A title's page, loaded by its id. */
export function Title(props: { readonly id: number }): JSX.Element {
  const { api } = useServices();
  const [title] = useResource(`title/${String(props.id)}`, async () => {
    const data = dataOrThrow(await api.GET("/items/{id}", { params: { path: { id: props.id } } }));
    return data;
  });
  return <Loaded resource={title} render={(value) => <TitleView title={value} />} />;
}
