// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { BusyGroup } from "./busy-group.tsx";
import { timeText } from "./date-time-text.ts";
import { FormRow } from "./form-row.tsx";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** One playback heard from lately. */
type Playing = components["schemas"]["NowPlayingBody"];

/**
 * What is playing now, for the admin (cut-a §5): who, on which device, what, and how; and stopping
 * a playback with a message its player shows within one report interval (D243 §9).
 */
export function NowPlaying(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [playing, reload] = useResource(
    "now-playing",
    async () => dataOrThrow(await api.GET("/now-playing")).playbacks ?? [],
    useBackgroundChangesPoller(),
  );
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const stop = (playbackId: number, message: string): void => {
    runRequest(async (idempotencyKey) => {
      throwIfRefused(
        await api.POST("/now-playing/{id}/stop", {
          params: { path: { id: playbackId } },
          body: { message },
          ...idempotent(idempotencyKey),
        }),
      );
      reload();
    });
  };
  return (
    <section aria-labelledby="now-playing">
      <h2 id="now-playing">{t("nowPlaying.heading")}</h2>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <BusyGroup isBusy={isBusy}>
        <Loaded
          resource={playing}
          render={(list) =>
            list.length === 0 ? (
              <p>{t("nowPlaying.none")}</p>
            ) : (
              <ul>
                {list.map((each) => (
                  <PlayingRow key={each.id} playing={each} onStop={stop} />
                ))}
              </ul>
            )
          }
        />
      </BusyGroup>
    </section>
  );
}

/** One playback: who plays what where, how, since when, and its stop with a message. */
function PlayingRow(props: {
  readonly playing: Playing;
  readonly onStop: (playbackId: number, message: string) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { playing } = props;
  const [message, setMessage] = useState("");
  const how =
    playing.delivery === "direct"
      ? t("nowPlaying.direct")
      : t("nowPlaying.prepared", { reasons: (playing.notDirectReasons ?? []).join(", ") });
  return (
    <li>
      <p>
        {t("nowPlaying.who", {
          person: playing.personName,
          device: playing.deviceName ?? t("nowPlaying.noDevice"),
          name: playing.name,
          since: timeText(playing.startedAt, i18n.language),
        })}{" "}
        {how}
      </p>
      <FormRow
        onSubmit={() => {
          props.onStop(playing.id, message);
        }}
      >
        <label>
          {t("nowPlaying.message")}{" "}
          <input
            maxLength={500}
            value={message}
            onChange={(event) => {
              setMessage(event.target.value);
            }}
          />
        </label>
        <button type="submit">{t("nowPlaying.stop")}</button>
      </FormRow>
    </li>
  );
}
