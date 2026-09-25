// SPDX-License-Identifier: MPL-2.0

import "./play.css";

import type { Api, components, Menu } from "@orpheum/sdk";
import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { useServices } from "./services.tsx";
import { camelKey, Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** The server's decision on how a file plays here (cut-a §6). */
export type Decision = components["schemas"]["StartPlaybackOutputBody"];

/**
 * Asks the server how a file plays in this browser. The link is local until M4 gives the
 * server an outside address.
 */
export async function requestPlayback(api: Api, fileId: number, menu: Menu): Promise<Decision> {
  const { data, error } = await api.POST("/playback", {
    body: { fileId, menu, link: "local" },
  });
  if (data === undefined) {
    throw new Error(error.error.message);
  }
  return data;
}

/** A decision, shown: the original playing, or why it needs preparing. The reason is always on screen (D188). */
export function PlayView(props: { readonly decision: Decision }): JSX.Element {
  const { t } = useTranslation();
  const { decision } = props;
  if (decision.kind === "direct" && decision.url !== undefined) {
    return <DirectVideo url={decision.url} />;
  }
  const reasons = (decision.reasons ?? [])
    .map((reason) => t(`play.reason.${camelKey(reason)}`))
    .join(", ");
  return (
    <div className="play">
      <p>{t("play.needsPreparing", { reasons })}</p>
      {decision.url === undefined ? <p>{t("play.notYet")}</p> : null}
    </div>
  );
}

/** The original file, playing; when the browser cannot play it, why, in words. */
function DirectVideo(props: { readonly url: string }): JSX.Element {
  const { t } = useTranslation();
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const failed = (event: SyntheticEvent<HTMLVideoElement>): void => {
    setFailure(mediaFailure(event.currentTarget.error));
  };
  return (
    <div className="play">
      {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- subtitles arrive with M3's subtitle slice (cut-a §6) */}
      <video
        className="play__video"
        src={props.url}
        controls
        autoPlay
        playsInline
        onError={failed}
      />
      {failure === undefined ? (
        <p className="play__reason">{t("play.direct")}</p>
      ) : (
        <p role="alert">{t("play.failed", { reason: failure })}</p>
      )}
    </div>
  );
}

/** What a media element's error says, in the browser's words, or its code when it gives none. */
export function mediaFailure(error: Pick<MediaError, "code" | "message"> | null): string {
  if (error === null) {
    return "unknown";
  }
  return error.message === "" ? `MediaError ${String(error.code)}` : error.message;
}

/** The player's screen for one file. */
export function Play(props: { readonly fileId: number }): JSX.Element {
  const { api, menu } = useServices();
  const [decision] = useResource(`play/${String(props.fileId)}`, () =>
    requestPlayback(api, props.fileId, menu()),
  );
  return <Loaded resource={decision} render={(value) => <PlayView decision={value} />} />;
}
