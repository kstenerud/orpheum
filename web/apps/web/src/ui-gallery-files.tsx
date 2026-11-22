// SPDX-License-Identifier: MPL-2.0

import "./gallery-words.ts";

import { type JSX } from "react";
import { useTranslation } from "react-i18next";

import { FileBrowser, type FileBrowserShown, type FileRow } from "./file-browser.tsx";

/** A folder row of the gallery's samples, opening nothing. */
function folderRow(name: string, details?: string): FileRow {
  return { key: name, kind: "folder", name, details, onOpen: () => undefined };
}

/**
 * The file browser's states (D539 §1, §2), each drawn as Choose would draw it: loading, an empty
 * folder, a folder whose count the machine has not finished, a folder it cannot read, and a machine
 * that is not answering at the places.
 */
export function GalleryFileBrowser(): JSX.Element {
  const { t } = useTranslation();
  const crumbs = [
    { key: "places", name: t("fileBrowser.places"), onOpen: () => undefined },
    { key: "here", name: "nas: /tank/media", onOpen: () => undefined },
  ];
  const states: readonly { readonly key: string; readonly shown: FileBrowserShown }[] = [
    { key: "gallery.fileBrowserLoading", shown: { kind: "loading" } },
    {
      key: "gallery.fileBrowserEmpty",
      shown: {
        kind: "listed",
        groups: [{ key: "here", rows: [] }],
        empty: { kind: "empty", text: t("fileBrowser.noFolders", { folder: "media" }) },
      },
    },
    {
      key: "gallery.fileBrowserSlowCount",
      shown: {
        kind: "listed",
        groups: [
          {
            key: "here",
            rows: [
              folderRow(t("gallery.fileBrowserCounted"), t("fileBrowser.videos", { count: 12 })),
              folderRow(t("gallery.fileBrowserUncounted")),
            ],
          },
        ],
        empty: { kind: "empty", text: "" },
      },
    },
    {
      key: "gallery.fileBrowserError",
      shown: {
        kind: "told",
        told: {
          kind: "problem",
          text: t("fileBrowser.cantRead", { folder: "media" }),
          onTryAgain: () => undefined,
        },
      },
    },
    {
      key: "gallery.fileBrowserOffline",
      shown: {
        kind: "listed",
        groups: [
          {
            key: "attic",
            heading: "attic",
            rows: [],
            told: { kind: "problem", text: t("fileBrowser.notAnswering", { machine: "attic" }) },
          },
        ],
        empty: { kind: "empty", text: "" },
      },
    },
  ];
  return (
    <section aria-labelledby="gallery-file-browser">
      <h2 id="gallery-file-browser">{t("gallery.fileBrowser")}</h2>
      {states.map((state) => (
        <div key={state.key} className="ui-gallery__notes">
          <h3>{t(state.key)}</h3>
          <FileBrowser
            use="choose"
            crumbs={state.key === "gallery.fileBrowserOffline" ? crumbs.slice(0, 1) : crumbs}
            listLabel="media"
            shown={state.shown}
          />
        </div>
      ))}
    </section>
  );
}
