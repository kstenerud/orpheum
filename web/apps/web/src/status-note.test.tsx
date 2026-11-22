// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { useEffect, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { StatusNote, useSaveState } from "./status-note.tsx";
import { mount, settleAnswers, type Mounted } from "./test-dom.tsx";

let shown: Mounted | undefined;

afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

describe("a status note", () => {
  it("shows nothing while there is nothing to tell", async () => {
    shown = await mount(<StatusNote told={undefined} />);
    expect(shown.container.innerHTML).toBe("");
  });

  it("carries the id a form's marked fields are described by, as an alert", async () => {
    shown = await mount(
      <StatusNote told={{ kind: "problem", text: "Wrong name or password", id: "f-problem" }} />,
    );
    const note = shown.container.querySelector("#f-problem");
    expect(note?.getAttribute("role")).toBe("alert");
    expect(note?.textContent).toBe("Wrong name or password");
  });

  it("tells what happened before the person arrived as a status, not an alert", async () => {
    shown = await mount(
      <StatusNote told={{ kind: "problem", text: "You were signed out", isFromBefore: true }} />,
    );
    const note = shown.container.querySelector(".status-line--problem");
    expect(note?.tagName).toBe("OUTPUT");
    expect(note?.getAttribute("role")).toBeNull();
    expect(note?.textContent).toBe("You were signed out");
  });
});

/** A setting that saves at once, sending once as it mounts, by send. */
function Saving(props: { readonly send: () => Promise<void> }): JSX.Element {
  const { told, save } = useSaveState();
  const { send } = props;
  useEffect(() => {
    void save(send);
  }, [save, send]);
  return <StatusNote told={told} />;
}

describe("a setting that saves itself (useSaveState)", () => {
  it('says "Saved" with its tick once the change is kept', async () => {
    shown = await mount(<Saving send={() => Promise.resolve()} />);
    await settleAnswers();
    expect(shown.container.querySelector("output.status-line--success")?.textContent).toBe("Saved");
  });

  it("shows the refusal in its place, as an alert", async () => {
    shown = await mount(<Saving send={() => Promise.reject(new Error("no"))} />);
    await settleAnswers();
    expect(shown.container.querySelector('[role="alert"].status-line--problem')).not.toBeNull();
    expect(shown.container.querySelector(".status-line--success")).toBeNull();
  });
});
