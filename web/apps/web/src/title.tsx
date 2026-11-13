// SPDX-License-Identifier: MPL-2.0

import { BusyGroup } from "./busy-group.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import "./title.css";

import type { components } from "@orpheum/sdk";
import { use, useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { camelKey } from "./catalogue-key.ts";
import { TitlePeople } from "./credits.tsx";
import { languageName } from "./language-name.ts";
import { channelsText } from "./tracks-note.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { dataOrThrow, throwIfRefused } from "./problem.ts";
import { identifiedCorrectlyAction, identifiedIncorrectlyAction } from "./match-correction.tsx";
import { MoreActions, type MoreAction } from "./more-actions.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { IsAdminContext } from "./warning-sign.tsx";
import { StatusNotes } from "./status-cues.tsx";
import { Poster, WideArt } from "./poster.tsx";
import { TitleHead } from "./title-head.tsx";
import { clockText } from "./timeline.tsx";
import { useUpNext } from "./around-file.ts";
import { Link } from "./link.tsx";
import type { Route } from "./router.ts";
import { noteEpisodeReached } from "./episode-reached.ts";
import { i18n } from "./i18n.ts";
import { UpLink, UpTrail, useLibraryContext } from "./up-trail.tsx";
import {
  episodeCrumbs,
  episodeHere,
  libraryCrumb,
  parentOf,
  seasonOfEpisode,
  seriesCrumb,
  titleCrumb,
  type LibraryContext,
  type Upward,
} from "./upward.ts";
import { usePageInterest } from "./page-interest.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";
import { watchingOf } from "./watching.ts";
import { firstPlayableFile } from "./first-playable.ts";
import { titleOrThrow } from "./item-detail.ts";
import { Loaded } from "./loaded.tsx";
import { groupName, placeCode } from "./episode-place.ts";
import { rememberedInTheTab, useRemembered } from "./remembered.ts";
import { MarkWatched } from "./watched-mark.tsx";

// Where they were before a screen that shows no title used them: a title's page loads with the player
// (later-screens.tsx), and they do not (the audit of 2026-10-25, W1).
export { firstPlayableFile, Loaded };

/** A title's detail, as the server sends it. */
export type TitleDetail = components["schemas"]["ItemBody"];
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

/** The episode after one, as the server's up-next says it (GET /files/{id}/up-next). */
type NextUp = components["schemas"]["NextUpBody"];

/**
 * A title's page, a film's, an episode's or an extra's, in the title template (title-head.tsx,
 * ui-audit-2026-11-04.md R8): its backdrop behind its first screen; an episode's series and season
 * above its name; one line of facts; the actions row, Play or Resume first (the owner, 2026-10-29:
 * "Play is the page's first button, not its last"), Start over when resuming, an episode's Next
 * episode, Mark as watched and the "…" of what is rarely used, the choice of version among them; what
 * it is about, clamped; then its cast and crew, its versions' technical detail and its extras. Play
 * plays the first version's first file (firstPlayableFile), the one its page warms up.
 */
export function TitleView(props: {
  readonly title: TitleDetail;
  /** Says one file that cannot be read now is gone for good (D241 §5); an admin's. */
  readonly onRetireFile?: (fileId: number) => void;
  /** Told a film's match was corrected, an admin's (D301 §2): the page reads it again. */
  readonly onMatchCorrected?: () => void;
  /** Told it was marked watched or unwatched by hand (D370 §4): the page reads it again. */
  readonly onWatchedChanged?: () => void;
  /** Where its Play resumes, as its warm-up was told: 0 at its start, undefined while unknown. */
  readonly resumeMs?: number | undefined;
  /** An episode's next, which its Next episode plays; undefined where none is known. */
  readonly nextEpisode?: NextUp | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { title, nextEpisode } = props;
  const fileId = firstPlayableFile(title);
  return (
    <article className="title">
      <TitleHead
        backdropSha256={title.art.backdropSha256}
        art={<TitleArt title={title} />}
        above={title.episode === undefined ? null : <EpisodePlaceFacts place={title.episode} />}
        name={title.name}
        facts={<TitleFacts title={title} />}
        notes={<StatusNotes subject={title} />}
        actions={
          <>
            {fileId === undefined ? null : (
              <PlayActions fileId={fileId} resumeMs={props.resumeMs} />
            )}
            {nextEpisode === undefined || nextEpisode.unavailable !== undefined ? null : (
              <Link to={{ screen: "play", fileId: nextEpisode.fileId }} className="title__action">
                {t("title.nextEpisode")}
              </Link>
            )}
            {/* An extra is a different work, never marked watched (D370 §2, D162). */}
            {title.kind === "extra" || props.onWatchedChanged === undefined ? null : (
              <MarkWatched
                target={{ kind: "title", id: title.id }}
                isWatched={title.isWatched === true}
                onChanged={props.onWatchedChanged}
              />
            )}
          </>
        }
        hasMainAction={fileId !== undefined}
        more={<TitleMoreActions title={title} onMatchCorrected={props.onMatchCorrected} />}
        tagline={title.tagline}
        overview={title.overview}
      />
      <TitlePeople credits={title.credits} />
      <Versions
        versions={title.versions ?? []}
        isPlayable={title.unavailable === undefined}
        {...(props.onRetireFile === undefined ? {} : { onRetireFile: props.onRetireFile })}
      />
      <Extras extras={title.extras ?? []} />
    </article>
  );
}

/**
 * The actions row's first: Play, or, where its person stopped partway, Resume with the point, which
 * plays from there as Play always has (the server's resume position), and Start over, from its start.
 */
function PlayActions(props: {
  readonly fileId: number;
  readonly resumeMs: number | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { fileId, resumeMs } = props;
  const isResuming = resumeMs !== undefined && resumeMs > 0;
  return (
    <>
      <Link to={{ screen: "play", fileId }} className="title__play">
        {isResuming ? t("title.resumeFrom", { at: clockText(resumeMs) }) : t("title.play")}
      </Link>
      {isResuming ? (
        <Link to={{ screen: "play", fileId, atMs: 0 }} className="title__action">
          {t("play.startOver")}
        </Link>
      ) : null}
    </>
  );
}

/**
 * A title's rarely used actions, under its "…": an admin's "Identified incorrectly" (none for an
 * extra, a work of its film's), its match chosen again only for a film, the one kind of title matched
 * itself (D301 §2), and only where the page is told of a correction; while its match is a guess,
 * "Identified correctly", which confirms it (D301 §4); then, for everyone, where it has more than
 * one version to play, the choice of which (ui-audit-2026-11-04.md §3). Its File details are the
 * section below its cast alone, the one place for its technical detail (D418 §5).
 */
function TitleMoreActions(props: {
  readonly title: TitleDetail;
  readonly onMatchCorrected: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const isAdmin = use(IsAdminContext);
  const { title, onMatchCorrected } = props;
  const actions: MoreAction[] = [];
  if (isAdmin && title.kind !== "extra") {
    const isWork = title.kind === "film" && onMatchCorrected !== undefined;
    const onCorrected = onMatchCorrected ?? (() => undefined);
    actions.push(identifiedIncorrectlyAction(t, { itemId: title.id, isWork, onCorrected }));
    if (title.isGuessed === true) {
      actions.push(identifiedCorrectlyAction(t, { itemId: title.id, onConfirmed: onCorrected }));
    }
  }
  const versions = title.versions ?? [];
  if (title.unavailable === undefined && versions.length > 1) {
    actions.push({
      key: "versions",
      label: t("title.chooseVersion"),
      dialog: (close) => (
        <ModalDialog label={t("title.chooseVersion")} onClose={close}>
          <h2>{t("title.chooseVersion")}</h2>
          <VersionChoice versions={versions} />
        </ModalDialog>
      ),
    });
  }
  return <MoreActions actions={actions} />;
}

/** Each version of a title by its number, a Play for each of its files: the "…"'s choice. */
function VersionChoice(props: {
  readonly versions: readonly components["schemas"]["VersionBody"][];
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="more-actions__offers">
      {props.versions.map((version, index) =>
        (version.files ?? []).map((file) => (
          <Link key={file.id} to={{ screen: "play", fileId: file.id }} className="title__action">
            {t("title.playVersion", { index: index + 1 })}
          </Link>
        )),
      )}
    </div>
  );
}

/** A film's poster, else its list card's still (D362 §2); an episode's still. */
function TitleArt(props: { readonly title: TitleDetail }): JSX.Element | null {
  const { art, kind } = props.title;
  if (kind !== "episode") {
    return <Poster sourceSha256={art.posterSha256} stillSha256={art.noPosterStillSha256} />;
  }
  return art.stillSha256 === undefined ? null : (
    <WideArt sourceSha256={art.stillSha256} size="landscape-m" className="title__still" />
  );
}

/**
 * An episode's series and its place, each a link up, an Up adding no step to the history (D367 §7):
 * one press from an episode reached by search or Continue Watching to its season
 * (research/back-navigation-2026-10-24.md). In an ordering the person
 * chose over aired, the place is its group there, "Disc 2 · 3", with its aired place as a second line,
 * the number other sites use; one that ordering leaves out says so (episode-orderings.md §7 Q8).
 */
function EpisodePlaceFacts(props: { readonly place: EpisodePlace }): JSX.Element {
  const { t } = useTranslation();
  const { place } = props;
  const series = (
    <UpLink crumb={seriesCrumb(place.seriesId, place.seriesName)}>{place.seriesName}</UpLink>
  );
  if (place.ordering === undefined) {
    return (
      <p>
        {series}
        {t("title.placeBetween")}
        <UpLink crumb={seriesCrumb(place.seriesId, place.seriesName, place.seasonNumber)}>
          {t("title.placeSeason", { season: place.seasonNumber })}
        </UpLink>
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
      <UpLink crumb={seriesCrumb(place.seriesId, place.seriesName, group.number)}>
        {groupName(t, place.ordering, group)}
      </UpLink>
      {t("title.placeBetween")}
      {inOrdering.numberInGroup}
    </>
  );
}

/**
 * A title's facts, one line (R8): its year, its length, the sound of the file its Play plays, and
 * that it is not identified yet. What the server does not send, a rating or a picture's size, is
 * left out.
 */
function TitleFacts(props: { readonly title: TitleDetail }): JSX.Element {
  const { t } = useTranslation();
  const { title } = props;
  const sound = soundOf(t, title);
  return (
    <>
      {title.year === undefined ? null : <span>{title.year}</span>}
      {title.runtimeMs === undefined ? null : (
        <span>{t("title.runtime", { minutes: Math.round(title.runtimeMs / 60_000) })}</span>
      )}
      {sound === undefined ? null : <span>{sound}</span>}
      {title.identificationState === "unidentified" ? <span>{t("title.unidentified")}</span> : null}
    </>
  );
}

/**
 * The sound of the file a title's Play plays, its first version's first file: its first audio
 * stream's codec and channels as the player's own sound line names them, "TRUEHD · 7.1", "AAC · mono";
 * undefined where it has none.
 */
function soundOf(t: TFunction, title: TitleDetail): string | undefined {
  const streams = title.versions?.[0]?.files?.[0]?.streams ?? [];
  const audio = streams.find((stream) => stream.kind === "audio" && (stream.codec ?? "") !== "");
  if (audio === undefined) {
    return undefined;
  }
  const name = (audio.codec ?? "").toUpperCase();
  return audio.channelCount === undefined
    ? name
    : t("title.factSound", { codec: name, channels: channelsText(t, audio.channelCount) });
}

/**
 * A title's versions' technical detail, below its cast (R8): each file with its path (for an admin)
 * and its streams, closed until asked for, and, for an admin, a way to say one that cannot be read
 * now is gone for good (D241 §5). Its Play is the actions row's; another version's, the "…"'s.
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
      <VersionsDetail
        versions={versions}
        retiring={
          props.isPlayable || props.onRetireFile === undefined ? undefined : props.onRetireFile
        }
      />
    </section>
  );
}

/** Each version's files, each its path and streams in a closed File details. */
function VersionsDetail(props: {
  readonly versions: readonly components["schemas"]["VersionBody"][];
  /** Where given, an admin's retire of each file with a path. */
  readonly retiring?: ((fileId: number) => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { versions, retiring } = props;
  return (
    <>
      {versions.map((version, index) => (
        <div key={version.id}>
          {versions.length === 1 ? null : <h3>{t("title.version", { index: index + 1 })}</h3>}
          {(version.files ?? []).map((file) => (
            <div key={file.id} className="title__file">
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
              {retiring === undefined || file.path === undefined ? null : (
                <ConfirmButton
                  label={t("title.retireFile")}
                  consequence={t("title.retireFileConsequence")}
                  confirmLabel={t("title.retireFileConfirm")}
                  onConfirm={() => {
                    retiring(file.id);
                  }}
                />
              )}
            </div>
          ))}
        </div>
      ))}
    </>
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
 * Where a title's page sits (D367 §2): an episode under its series, at its season, under the library
 * it is listed in; an extra under its film; a film or an other video under its library. Before the
 * title is read, under Films, so a page that cannot be read still has a way up.
 */
export function titleUpward(title: TitleDetail | undefined, context: LibraryContext): Upward {
  if (title === undefined) {
    return { crumbs: [libraryCrumb("film", undefined, context)], here: "" };
  }
  const libraries = title.libraries ?? undefined;
  const place = title.episode;
  if (place !== undefined) {
    return {
      crumbs: episodeCrumbs(place, libraries, context),
      here: episodeHere(context.t, place, title.name),
    };
  }
  const library = libraryCrumb(title.kind === "other" ? "other" : "film", libraries, context);
  const film = title.filmOfExtra;
  return {
    crumbs: film === undefined ? [library] : [library, titleCrumb(film.id, film.name)],
    here: title.name,
  };
}

/** The page above a title's, as titleUpward has it, where nothing is behind it in the history. */
export function titleUp(
  title: TitleDetail | undefined,
  context: LibraryContext = { t: i18n.t, behind: [] },
): Route {
  return parentOf(titleUpward(title, context))?.to ?? { screen: "films" };
}

/** Notes an episode's page shown, which its series' page shows when returned to (D367 §9). */
function useEpisodeReached(title: TitleDetail | undefined): void {
  const place = title?.episode;
  const id = title?.id;
  const seriesId = place?.seriesId;
  const season = place === undefined ? undefined : seasonOfEpisode(place);
  useEffect(() => {
    if (seriesId !== undefined && id !== undefined) {
      noteEpisodeReached(seriesId, { season, episodeId: id });
    }
  }, [seriesId, season, id]);
}

/** The titles' pages this tab has shown. */
const rememberedTitles = rememberedInTheTab<TitleDetail>();

/**
 * Warms up fileId, the first file a title's page plays, while the page shows it (D258 §1); its
 * preload is made only where the page's media do not go over WebRTC (D260 §3's revision of
 * 2026-10-28). Returns where it resumes, as the warm-up was told, undefined until it is.
 */
function useWarmsUp(fileId: number | undefined): number | undefined {
  const { api, playableFormats, pathMoves } = useServices();
  const [resume, setResume] = useState<{ readonly fileId: number; readonly ms: number }>();
  useEffect(() => {
    if (fileId === undefined) {
      return undefined;
    }
    const watching = watchingOf(api);
    watching.warmUp(fileId, playableFormats(), () => pathMoves?.isOverWebrtc() === true);
    let isCurrent = true;
    void watching.warmResumeMs(fileId).then((ms) => {
      if (isCurrent && ms !== undefined) {
        setResume({ fileId, ms });
      }
    });
    return () => {
      isCurrent = false;
    };
  }, [api, playableFormats, pathMoves, fileId]);
  return resume !== undefined && resume.fileId === fileId ? resume.ms : undefined;
}

/** A title's page, loaded by its id. Opening it warms up its first file (D258 §1). */
export function Title(props: { readonly id: number }): JSX.Element {
  const { api } = useServices();
  const [loaded, reload] = useResource(
    `title/${String(props.id)}`,
    async (signal) =>
      titleOrThrow(
        dataOrThrow(
          await api.GET("/items/{id}", {
            params: { path: { id: props.id } },
            signal: signal ?? null,
          }),
        ),
      ),
    { poller: useBackgroundChangesPoller() },
  );
  const title = useRemembered(`title/${String(props.id)}`, loaded, rememberedTitles);
  usePageInterest(props.id);
  const shown = title.state === "loaded" ? title.value : undefined;
  useEpisodeReached(shown);
  const context = useLibraryContext();
  const fileId = shown === undefined ? undefined : firstPlayableFile(shown);
  const resumeMs = useWarmsUp(fileId);
  // An episode's next, as its player's Up next asks for it: a film has none to ask for.
  const nextEpisode = useUpNext(fileId ?? 0, shown?.kind === "episode" && fileId !== undefined);
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
      <UpTrail upward={titleUpward(shown, context)} />
      <BusyGroup isBusy={retiring.isBusy}>
        <Loaded
          resource={title}
          render={(value) => (
            <TitleView
              title={value}
              onRetireFile={retireFile}
              onMatchCorrected={reload}
              onWatchedChanged={reload}
              resumeMs={resumeMs}
              nextEpisode={nextEpisode?.episode}
            />
          )}
        />
      </BusyGroup>
    </>
  );
}
