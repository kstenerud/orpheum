// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { listMyDevices, renameMyDevice } from "./accounts.ts";
import { Link } from "./link.tsx";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { DeviceName } from "./your-devices.tsx";

/**
 * The settings of the device the person is using, in one place: its name. How it makes sound is
 * asked of no one (D336 §4); its sound offset is set under the video. The person's other devices
 * are on their account page.
 */
export function ThisDevice(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [devices, reload] = useResource("my-devices", () => listMyDevices(api));
  const { problem, runRequest } = useRequest((cause) => problemText(t, cause));
  return (
    <section className="this-device" aria-labelledby="this-device-heading">
      <h1 id="this-device-heading">{t("thisDevice.heading")}</h1>
      <p>{t("thisDevice.explain")}</p>
      <Loaded
        resource={devices}
        render={(list) => {
          const device = list.find((each) => each.isThisDevice);
          if (device === undefined) {
            return <p>{t("thisDevice.notListed")}</p>;
          }
          return (
            <>
              <h2>{t("thisDevice.name")}</h2>
              <DeviceName
                device={device}
                onRename={(_, name) => {
                  runRequest(async () => {
                    await renameMyDevice(api, device.id, name);
                    reload();
                  });
                }}
              />
              {problem === undefined ? null : <p role="alert">{problem}</p>}
            </>
          );
        }}
      />
      <p>
        <Link to={{ screen: "account" }}>{t("thisDevice.elsewhere")}</Link>
      </p>
    </section>
  );
}
