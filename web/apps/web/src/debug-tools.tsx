// SPDX-License-Identifier: MPL-2.0

import { createContext, use, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";

/**
 * The signed-in person's Show debug tools, as the page last read it (GET /me's showsDebugTools), and
 * how to have it read again once changed. Off for anyone not signed in, and until read.
 */
export interface DebugToolsChoice {
  readonly isShown: boolean;
  /** Reads the person's choice again: called once a change of it is kept. */
  readonly onChanged: () => void;
}

/**
 * The person's Show debug tools, given by the page that read who they are: the signed-in shell, and
 * a signed-in guest's Watch Together page (app.tsx). Hidden where nothing gives it.
 */
export const DebugToolsContext = createContext<DebugToolsChoice>({
  isShown: false,
  onChanged: () => undefined,
});

/**
 * Whether the signed-in person shows debug tools: their own setting, off until they turn it on, kept
 * on the server so it follows them to every device (the owner, 2026-10-29).
 */
export function useShowsDebugTools(): boolean {
  return use(DebugToolsContext).isShown;
}

/**
 * Marks its children a debug tool, the one way a screen marks one (the owner, 2026-10-29: "a Show
 * debug tools option that shows every control on any screen marked as a debug tool"): they render
 * only while the signed-in person's Show debug tools is on.
 */
export function DebugTool(props: { readonly children: ReactNode }): ReactNode {
  return useShowsDebugTools() ? props.children : null;
}

/**
 * The person's own Show debug tools on their account, with what it does: a change is kept on the
 * server at once (PUT /me/preferences), read again for every screen, and a refusal said beside it.
 */
export function DebugToolsSetting(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { isShown, onChanged } = use(DebugToolsContext);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  return (
    <section aria-labelledby="debug-tools-heading">
      <h2 id="debug-tools-heading">{t("debugTools.heading")}</h2>
      <p>{t("debugTools.hint")}</p>
      <label>
        <input
          type="checkbox"
          checked={isShown}
          onChange={(event) => {
            setProblem(undefined);
            const showsDebugTools = event.target.checked;
            api
              .PUT("/me/preferences", { body: { showsDebugTools } })
              .then(throwIfRefused)
              .then(onChanged, (cause: unknown) => {
                setProblem(problemText(t, cause));
              });
          }}
        />{" "}
        {t("debugTools.show")}
      </label>
      {problem === undefined ? null : <output>{problem}</output>}
    </section>
  );
}
