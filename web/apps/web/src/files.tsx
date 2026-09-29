// SPDX-License-Identifier: MPL-2.0

import type { components, paths } from "@orpheum/sdk";
import { useCallback, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow } from "./problem.ts";
import { EndOfList } from "./end-of-list.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { usePages, type Page } from "./use-pages.ts";

/** One file the scanner saw, as the server sends it. */
export type SeenFile = components["schemas"]["FileBody"];

/** GET /files's query, the API's own, so a renamed parameter fails to compile (D227). */
type FilesQuery = NonNullable<paths["/files"]["get"]["parameters"]["query"]>;

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
export function groupByReason(files: readonly SeenFile[]): ReasonGroup[] {
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
      {groupByReason(props.files).map((group) => (
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
                  <p>
                    {t("files.errorDetail", {
                      text: file.errorDetail.text,
                      owner: file.errorDetail.ownerUid,
                      group: file.errorDetail.groupGid,
                      mode: file.errorDetail.modeText,
                    })}
                  </p>
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
  const [filingFilter, setFilingFilter] = useState<FilingChoice>("");
  return (
    <section aria-labelledby="files-heading">
      <h1 id="files-heading">{t("files.heading")}</h1>
      <label>
        {t("common.show")}
        <select
          value={filingFilter}
          onChange={(event) => {
            setFilingFilter(filingChoices.find((choice) => choice === event.target.value) ?? "");
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
        key={`${String(props.libraryId)}/${filingFilter}`}
        libraryId={props.libraryId}
        filingFilter={filingFilter}
      />
    </section>
  );
}

/** One library's files of one filing, read a page at a time. */
function FilePages(props: {
  readonly libraryId: number;
  readonly filingFilter: FilingChoice;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { libraryId, filingFilter } = props;
  const loadPage = useCallback(
    async (cursor: string): Promise<Page<SeenFile>> => {
      const query = {
        libraryId,
        filing: filingFilter,
        limit: 500,
        ...(cursor === "" ? {} : { cursor }),
      } satisfies FilesQuery;
      const filesPage = dataOrThrow(await api.GET("/files", { params: { query } }));
      return { items: filesPage.files ?? [], nextCursor: filesPage.nextCursor ?? "" };
    },
    [api, libraryId, filingFilter],
  );
  const pages = usePages(loadPage);
  return (
    <Loaded
      resource={pages.pagesSoFar}
      render={(page) => (
        <>
          <ReasonGroups files={page.items} />
          {page.nextCursor === "" ? null : (
            <EndOfList onVisible={pages.loadMore}>
              <button type="button" onClick={pages.loadMore} disabled={pages.isLoadingMore}>
                {t("common.showMore")}
              </button>
            </EndOfList>
          )}
        </>
      )}
    />
  );
}
