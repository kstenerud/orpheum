// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { listMyDevices, renameMyDevice } from "./accounts.ts";
import { chooseOk, okChoices, useOkChoice } from "./controller-choice.ts";
import { arrivalMark } from "./focus-navigation.ts";
import { Link } from "./link.tsx";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { DeviceName } from "./your-devices.tsx";

/**
 * Which face button of a game controller is OK in this browser (controller-choice.ts): automatic, by
 * the controller's make, unless the person chose; kept in the browser, so shown whether or not the
 * browser is one of their devices.
 */
function ControllerChoice(): JSX.Element {
  const { t } = useTranslation();
  const chosen = useOkChoice();
  return (
    <>
      <h2>{t("thisDevice.controller.heading")}</h2>
      <p>{t("thisDevice.controller.explain")}</p>
      <fieldset>
        <legend>{t("thisDevice.controller.legend")}</legend>
        {okChoices.map((choice) => (
          <label key={choice}>
            <input
              type="radio"
              name="controller-ok"
              value={choice}
              checked={chosen === choice}
              onChange={() => {
                chooseOk(choice);
              }}
            />
            {t(`thisDevice.controller.${choice}`)}
          </label>
        ))}
      </fieldset>
    </>
  );
}

/**
 * The settings of the device the person is using, in one place: its name, and which button of a
 * game controller is OK. How it makes sound is asked of no one (D336 §4); its sound offset is set
 * under the video. The person's other devices are on their account page.
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
              <div {...arrivalMark}>
                <DeviceName
                  device={device}
                  onRename={(_, name) => {
                    runRequest(async () => {
                      await renameMyDevice(api, device.id, name);
                      reload();
                    });
                  }}
                />
              </div>
              {problem === undefined ? null : <p role="alert">{problem}</p>}
            </>
          );
        }}
      />
      <ControllerChoice />
      <p>
        <Link to={{ screen: "account" }}>{t("thisDevice.elsewhere")}</Link>
      </p>
    </section>
  );
}
