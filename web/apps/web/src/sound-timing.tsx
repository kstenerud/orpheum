// SPDX-License-Identifier: MPL-2.0

import "./devices-words.ts";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
import { Loaded } from "./loaded.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/**
 * The sound timing test (the owner, 2026-11-06), a page of This device's since the device lab went
 * (D555 §12; the owner, 2026-11-25: the sound-timing page stays): the server's own sound timing
 * clip, a flash and a beep each second, asked for by what it is and played on the Play page as any
 * film is, through the player and its chunks, so its sound is a film's, levelled and moved by the
 * device's sound offset as a film's are; the player's own Sound earlier or later lines each beep up
 * with its flash (D435 §6). A clip the page drew itself proved nothing of how a film plays (the
 * owner, 2026-11-06).
 */
export function SoundTiming(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [clip] = useResource(
    "sound-timing-clip",
    async () =>
      dataOrThrow(
        await api.GET("/system-videos/{purpose}/{codec}", {
          params: { path: { purpose: "soundTiming", codec: "h264-aac" } },
        }),
      ),
    { poller: useBackgroundChangesPoller(), isSingleUse: true }, // its clip plays on this page alone
  );
  return (
    <section className="this-device" aria-labelledby="sound-timing-heading">
      <p>
        <Link to={{ screen: "this-device" }}>{t("thisDevice.soundTiming.back")}</Link>
      </p>
      <h1 id="sound-timing-heading">{t("thisDevice.soundTiming.heading")}</h1>
      <p>{t("thisDevice.soundTiming.explain")}</p>
      <Loaded
        resource={clip}
        render={(video) =>
          video.isReady ? (
            <p>
              <Link to={{ screen: "play", fileId: video.fileId }}>
                {t("thisDevice.soundTiming.play")}
              </Link>
            </p>
          ) : (
            <output>{t("thisDevice.soundTiming.making")}</output>
          )
        }
      />
    </section>
  );
}
