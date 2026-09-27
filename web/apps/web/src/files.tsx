// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useCallback, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ApiProblem } from "./problem.ts";
import { EndOfList } from "./end-of-list.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { usePages, type Page } from "./use-pages.ts";

/** One file the scanner saw, as the server sends it. */
export type SeenFile = components["schemas"]["FileBody"];

/** The filings to show: one, or every one (""). */
export type FilingChoice =
  "" | "listed" | "extra" | "attached" | "skipped" | "errored" | "unidentified";

const filingChoices: readonly FilingChoice[] = [
  "",
  "errored",
  "skipped",
  "unidentified",
  "extra",
  "attached",
  "listed",
];

/** Files with the same filing and reason, together: the complaint is never one file (D163). */
export interface ReasonGroup {
  readonly filing: string;
  readonly reason: string;
  readonly files: readonly SeenFile[];
}

/** Groups files by filing and reason, the largest group first. */
export function byReason(files: readonly SeenFile[]): ReasonGroup[] {
  const groups = new Map<string, { filing: string; reason: string; files: SeenFile[] }>();
  for (const file of files) {
    const reason = file.filingReason ?? "";
    const key = `${file.filing}\u0000${reason}`;
    const group = groups.get(key) ?? { filing: file.filing, reason, files: [] };
    group.files.push(file);
    groups.set(key, group);
  }
  return [...groups.values()].sort((first, second) => second.files.length - first.files.length);
}

/** The files grouped by why they were filed as they were, each group open to its paths. */
export function ReasonGroups(props: { readonly files: readonly SeenFile[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      {byReason(props.files).map((group) => (
        <details key={`${group.filing}/${group.reason}`}>
          <summary>
            {t("files.group", {
              filing: t(`filing.${group.filing}`, { defaultValue: group.filing }),
              reason: group.reason === "" ? t("files.noReason") : group.reason,
              count: group.files.length,
            })}
          </summary>
          <ul>
            {group.files.map((file) => (
              <li key={file.id}>
                <code>{file.path}</code>
                {file.errorDetail === undefined ? null : (
                  <p>{t("files.detail", { detail: file.errorDetail })}</p>
                )}
              </li>
            ))}
          </ul>
        </details>
      ))}
    </>
  );
}

/** What the scanner saw in a library: every file, whatever became of it (D163). */
export function Files(props: { readonly libraryId: number }): JSX.Element {
  const { t } = useTranslation();
  const [filing, setFiling] = useState<FilingChoice>("");
  return (
    <section aria-labelledby="files-heading">
      <h1 id="files-heading">{t("files.heading")}</h1>
      <label>
        {t("files.show")}
        <select
          value={filing}
          onChange={(event) => {
            setFiling(filingChoices.find((choice) => choice === event.target.value) ?? "");
          }}
        >
          {filingChoices.map((choice) => (
            <option key={choice} value={choice}>
              {choice === "" ? t("files.all") : t(`filing.${choice}`)}
            </option>
          ))}
        </select>
      </label>
      <FilePages
        key={`${String(props.libraryId)}/${filing}`}
        libraryId={props.libraryId}
        filing={filing}
      />
    </section>
  );
}

/** One library's files of one filing, read a page at a time. */
function FilePages(props: {
  readonly libraryId: number;
  readonly filing: FilingChoice;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { libraryId, filing } = props;
  const load = useCallback(
    async (cursor: string): Promise<Page<SeenFile>> => {
      const query = {
        library: libraryId,
        filing,
        limit: 500,
        ...(cursor === "" ? {} : { cursor }),
      };
      const { data, error } = await api.GET("/files", { params: { query } });
      if (data === undefined) {
        throw new ApiProblem(error.error);
      }
      return { items: data.files ?? [], next: data.nextCursor ?? "" };
    },
    [api, libraryId, filing],
  );
  const pages = usePages(load);
  return (
    <Loaded
      resource={pages.list}
      render={(page) => (
        <>
          <ReasonGroups files={page.items} />
          {page.next === "" ? null : (
            <EndOfList onVisible={pages.more}>
              <button type="button" onClick={pages.more} disabled={pages.isLoadingMore}>
                {t("browse.more")}
              </button>
            </EndOfList>
          )}
        </>
      )}
    />
  );
}
