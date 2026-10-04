// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import type { RequestState } from "./use-request.ts";

/**
 * What a person's request to an outside provider waits for, in words (D231 §12): the provider asked
 * us to wait, counting down to the try the request makes on its own; the check of the provider after
 * it was not answering; or a request past a couple of seconds. Nothing while it is not held back.
 */
export function RequestWait(props: {
  readonly requestState: RequestState;
  /** The provider's name: TMDB, OpenSubtitles. */
  readonly provider: string;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { requestState, provider } = props;
  if (requestState.retryInSeconds !== undefined) {
    return (
      <output>
        {t("problem.wait.askedToWait", { provider, seconds: requestState.retryInSeconds })}
      </output>
    );
  }
  if (requestState.isBusy && requestState.isChecking) {
    return <output>{t("problem.wait.checking", { provider })}</output>;
  }
  if (requestState.isBusy && requestState.isSlow) {
    return <output>{t("problem.wait.stillWaiting", { provider })}</output>;
  }
  return null;
}
