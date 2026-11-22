// SPDX-License-Identifier: MPL-2.0

import { useRef, type JSX } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import type { components } from "@orpheum/sdk";

import { useBackgroundChangesPoller } from "./polling.ts";
import {
  FileBrowser,
  inNaturalOrder,
  type Crumb,
  type FileBrowserShown,
  type FileRow,
} from "./file-browser.tsx";
import { useCommands } from "./input-layer.tsx";
import { Still } from "./poster.tsx";
import { dataOrThrow } from "./problem.ts";
import { itemRoute, type Route } from "./router.ts";
import { useServices } from "./services.tsx";
import { orderQuery, type SortChoice } from "./sort-choices.ts";
import type { Upward } from "./upward.ts";
import { useResource } from "./use-resource.ts";

type FolderVideo = components["schemas"]["FolderVideoBody"];
type Subfolder = components["schemas"]["SubfolderBody"];

/** A library's folder as Browse shows it: the folders in it, and its own videos, each with its file. */
interface LibraryFolder {
  readonly subfolders: readonly Subfolder[];
  readonly videos: readonly FolderVideo[];
}

/** How far into a video its resume point is, as a whole percent; none where it has none. */
export function percentWatched(video: FolderVideo): number | undefined {
  const { durationMs, resumePositionMs } = video;
  if (durationMs === undefined || durationMs <= 0 || resumePositionMs === undefined) {
    return undefined;
  }
  return Math.min(100, Math.round((resumePositionMs / durationMs) * 100));
}

/** What a video's row says under its file name: its title and year, or that it is not matched. */
function underOf(video: FolderVideo, t: TFunction): string {
  const { title } = video;
  if (title === undefined) {
    return t("fileBrowser.notMatched");
  }
  return title.year === undefined ? title.name : `${title.name} (${String(title.year)})`;
}

/** A video's length in minutes, as its row says it; none where it is not known. */
function lengthOf(video: FolderVideo, t: TFunction): string | undefined {
  return video.durationMs === undefined
    ? undefined
    : t("title.runtime", { minutes: Math.round(video.durationMs / 60_000) });
}

/**
 * The rows of a library's folder (D539 §3): its folders first, then its videos, each in natural
 * order. A folder opens into its own; a video plays, resuming where it was left, its More offering
 * Play from the start, its title page where it is matched, and Show in Files.
 */
function rowsOf(
  folder: LibraryFolder,
  libraryId: number,
  go: (route: Route) => void,
  folderRoute: (folder: string | undefined) => Route,
  t: TFunction,
): FileRow[] {
  const folders = inNaturalOrder(folder.subfolders, (subfolder) => subfolder.name).map(
    (subfolder): FileRow => ({
      key: `folder:${subfolder.path}`,
      kind: "folder",
      name: subfolder.name,
      details: t("fileBrowser.videos", { count: subfolder.videoCount }),
      onOpen: () => {
        go(folderRoute(subfolder.path));
      },
    }),
  );
  const videos = inNaturalOrder(folder.videos, (video) => video.name).map((video): FileRow => {
    const { title } = video;
    const play = (atMs: number | undefined): void => {
      go({
        screen: "play",
        fileId: video.fileId,
        ...(title === undefined ? {} : { itemId: title.id }),
        ...(atMs === undefined ? {} : { atMs }),
      });
    };
    const percent = percentWatched(video);
    return {
      key: `video:${String(video.fileId)}`,
      kind: "video",
      name: video.name,
      under: underOf(video, t),
      details: lengthOf(video, t),
      ...(percent === undefined ? {} : { percent }),
      onOpen: () => {
        play(video.resumePositionMs);
      },
      more: [
        {
          key: "start",
          label: t("fileBrowser.playFromStart"),
          onChoose: () => {
            play(0);
          },
        },
        ...(title === undefined
          ? []
          : [
              {
                key: "title",
                label: t("fileBrowser.titlePage"),
                onChoose: () => {
                  go(itemRoute(title.kind, title.id));
                },
              },
            ]),
        {
          key: "files",
          label: t("fileBrowser.showInFiles"),
          onChoose: () => {
            go({ screen: "files", libraryId });
          },
        },
      ],
    };
  });
  return [...folders, ...videos];
}

/** The path bar's crumbs from where the folder sits: the library, each folder above, the folder. */
function crumbsOf(upward: Upward, go: (route: Route) => void): Crumb[] {
  return [
    ...upward.crumbs.map((crumb) => ({
      key: crumb.label,
      name: crumb.label,
      onOpen: () => {
        go(crumb.to);
      },
    })),
    { key: "here", name: upward.here, onOpen: () => undefined },
  ];
}

/**
 * Browse, the file browser's view of a library's folders (D539 §3): every single library's Folders
 * view, in rows, a video playing on OK. Back goes up one folder, leaving the page only at the
 * library's top (§4); opening a folder is a step of the address, so the browser's Back and Forward
 * move between folders. On a wide screen a side panel shows the focused video (§5).
 */
export function LibraryFolders(props: {
  readonly libraryId: number;
  readonly folder: string | undefined;
  readonly choice: SortChoice;
  readonly upward: Upward;
  readonly folderRoute: (folder: string | undefined) => Route;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const { libraryId, folder, choice, upward } = props;
  const ref = useRef<HTMLDivElement>(null);
  // An other library's folder has no release date to sort by; its sorts never offer one.
  const sort = choice.sort === "released" ? "recorded" : choice.sort;
  const [result, reload] = useResource(
    `browse-folder/${String(libraryId)}/${folder ?? ""}/${sort}/${choice.order}`,
    async (): Promise<LibraryFolder> => {
      // No limit: the whole folder in one answer (D475).
      const page = dataOrThrow(
        await api.GET("/libraries/{id}/folders", {
          params: {
            path: { id: libraryId },
            query: { sort, ...orderQuery(choice), ...(folder === undefined ? {} : { folder }) },
          },
        }),
      );
      return { subfolders: page.subfolders ?? [], videos: page.videos ?? [] };
    },
    { poller: useBackgroundChangesPoller() },
  );
  const parent = upward.crumbs.at(-1);
  useCommands(ref, (detail, event) => {
    if (detail.command === "back" && parent !== undefined) {
      event.preventDefault();
      navigation.up(parent.to, parent.isIt);
    }
  });
  const go = (route: Route): void => {
    navigation.go(route);
  };
  const here = upward.here;
  const videos = result.state === "loaded" ? result.value.videos : [];
  const shown: FileBrowserShown =
    result.state === "loading"
      ? { kind: "loading" }
      : result.state === "failed"
        ? {
            kind: "told",
            told: {
              kind: "problem",
              text: t("fileBrowser.cantRead", { folder: here }),
              onTryAgain: reload,
            },
          }
        : {
            kind: "listed",
            groups: [
              { key: "here", rows: rowsOf(result.value, libraryId, go, props.folderRoute, t) },
            ],
            empty: {
              kind: "empty",
              text: t("fileBrowser.noVideos", { folder: here }),
              ...(parent === undefined
                ? {}
                : {
                    next: {
                      label: t("fileBrowser.up"),
                      onPress: () => {
                        navigation.up(parent.to, parent.isIt);
                      },
                    },
                  }),
            },
          };
  return (
    <div ref={ref} className="library-folders">
      <FileBrowser
        use="browse"
        crumbs={crumbsOf(upward, go)}
        listLabel={here}
        shown={shown}
        side={(row) => <SidePanel row={row} videos={videos} />}
      />
    </div>
  );
}

/**
 * Browse's side panel (D539 §5): the video focus rested on, its still where it is matched, its
 * title, length and how far it is watched; a folder, its name and count.
 */
function SidePanel(props: {
  readonly row: FileRow | undefined;
  readonly videos: readonly FolderVideo[];
}): JSX.Element | null {
  const { row } = props;
  if (row === undefined) {
    return null;
  }
  const video = props.videos.find((each) => `video:${String(each.fileId)}` === row.key);
  return (
    <>
      {video?.title === undefined ? null : <SideStill itemId={video.title.id} />}
      <h3 className="file-browser__side-name">{row.name}</h3>
      {row.under === undefined ? null : <p className="file-browser__side-said">{row.under}</p>}
      {row.details === undefined ? null : <p className="file-browser__side-said">{row.details}</p>}
    </>
  );
}

/** A matched video's still, as its title's page reads it. */
function SideStill(props: { readonly itemId: number }): JSX.Element | null {
  const { api } = useServices();
  const [item] = useResource(`file-browser-side/${String(props.itemId)}`, async () =>
    dataOrThrow(await api.GET("/items/{id}", { params: { path: { id: props.itemId } } })),
  );
  if (item.state !== "loaded") {
    return null;
  }
  const { art } = item.value;
  return (
    <Still
      className="file-browser__side-still"
      sourceSha256={art?.stillSha256 ?? art?.noPosterStillSha256}
      isNoPosterMarked={art?.stillSha256 === undefined}
    />
  );
}
