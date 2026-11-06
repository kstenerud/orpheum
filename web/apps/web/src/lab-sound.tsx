// SPDX-License-Identifier: MPL-2.0

import "./lab-words.ts";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { LabFrame } from "./lab-frame.tsx";
import { Loaded } from "./loaded.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { routeHref } from "./router.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/**
 * The sound timing test (the owner, 2026-11-06): the server's own sound timing clip, a flash and a
 * beep each second, asked for by what it is and played on the Play page as any film is, through the
 * player and its chunks, so its sound is a film's, levelled and moved by the device's sound offset as
 * a film's are; the player's own Sound earlier or later lines each beep up with its flash (D435 §6). A
 * clip the page drew itself proved nothing of how a film plays (the owner, 2026-11-06).
 */
export function LabSound(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [clip] = useResource(
    "lab-sound-clip",
    async () =>
      dataOrThrow(
        await api.GET("/system-videos/{purpose}/{codec}", {
          params: { path: { purpose: "sound-timing", codec: "h264-aac" } },
        }),
      ),
    useBackgroundChangesPoller(),
  );
  return (
    <LabFrame id="lab-sound" heading={t("lab.sound.heading")}>
      <p className="lab__lead">{t("lab.sound.explain")}</p>
      <Loaded
        resource={clip}
        render={(video) =>
          video.isReady ? (
            <p>
              <a className="lab__button" href={routeHref({ screen: "play", fileId: video.fileId })}>
                {t("lab.sound.play")}
              </a>
            </p>
          ) : (
            <output className="lab__small">{t("lab.sound.making")}</output>
          )
        }
      />
    </LabFrame>
  );
}
