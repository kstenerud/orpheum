// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { idempotent } from "./idempotent.ts";
import { byteSizeText } from "./number-text.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/**
 * The server's own videos, a debug tool on the System page (D435 §7): the node whose system folder
 * holds them, each video and whether it is made, and making them all again over the files there.
 */
export function SystemVideos(): JSX.Element {
  const { t, i18n } = useTranslation();
  const { api } = useServices();
  const [listed, reload] = useResource("system-videos", async () =>
    dataOrThrow(await api.GET("/system/videos")),
  );
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const rebuild = (): void => {
    runRequest(async (idempotencyKey) => {
      throwIfRefused(await api.POST("/system/videos/rebuild", idempotent(idempotencyKey)));
      reload();
    });
  };
  const view = listed.state === "loaded" ? listed.value : undefined;
  return (
    <section aria-labelledby="system-videos">
      <h2 id="system-videos">{t("system.videos.heading")}</h2>
      {view === undefined ? null : (
        <p>
          {view.nodeName === ""
            ? t("system.videos.nowhere")
            : t("system.videos.where", { node: view.nodeName, folder: view.folder })}
        </p>
      )}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <ul>
        {(view?.videos ?? []).map((video) => (
          <li key={video.fileName}>
            {video.isReady
              ? t("system.videos.ready", {
                  name: video.fileName,
                  size: byteSizeText(video.sizeBytes, i18n.language),
                })
              : t("system.videos.notReady", { name: video.fileName })}
          </li>
        ))}
      </ul>
      <button type="button" disabled={isBusy} onClick={rebuild}>
        {t("system.videos.rebuild")}
      </button>
    </section>
  );
}
