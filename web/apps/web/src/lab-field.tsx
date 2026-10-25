// SPDX-License-Identifier: MPL-2.0

import { useEffect, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import type { NotedEvent } from "./lab-events.ts";

/** When an event came, on the page's clock, in whole milliseconds. */
function nowMs(): number {
  return Math.round(performance.now());
}

/** An input event's inputType, where the browser gives one, as the log takes it: letters, 40 at most. */
function inputTypeOf(event: SyntheticEvent<HTMLInputElement>): string | undefined {
  const inputType: unknown = Reflect.get(event.nativeEvent, "inputType");
  if (typeof inputType !== "string") {
    return undefined;
  }
  return inputType.replaceAll(/[^A-Za-z]/gu, "").slice(0, inputTypeKept);
}

/** How much of an inputType the log keeps: the server's bound (lab.go). A justified constant. */
const inputTypeKept = 40;

/**
 * The button test's text field (plans/tv-device-tests.md H6): what a TV's browser and its on-screen
 * keyboard do with a field, sent as it happens: focus entering and leaving it, each input and
 * composition's start and end, with the value's length only, never its text, which could be anything
 * a person types. Its keys are the page's, flagged as in the field; the arrows are the field's but at
 * the value's ends (lab-focus.ts).
 */
export function FieldTest(props: { readonly noteEvent: (event: NotedEvent) => void }): JSX.Element {
  const { t } = useTranslation();
  const { noteEvent } = props;
  const noteOf =
    (kind: NotedEvent["kind"]) =>
    (event: { readonly currentTarget: HTMLInputElement }): void => {
      noteEvent({ kind, valueLength: event.currentTarget.value.length, atMs: nowMs() });
    };
  return (
    <input
      type="text"
      className="lab__type"
      autoComplete="off"
      placeholder={t("lab.input.field.placeholder")}
      aria-label={t("lab.input.field.label")}
      onFocus={noteOf("fieldIn")}
      onBlur={noteOf("fieldOut")}
      onCompositionStart={noteOf("compositionStart")}
      onCompositionEnd={noteOf("compositionEnd")}
      onInput={(event) => {
        const inputType = inputTypeOf(event);
        noteEvent({
          kind: "fieldInput",
          valueLength: event.currentTarget.value.length,
          ...(inputType === undefined ? {} : { inputType }),
          atMs: nowMs(),
        });
      }}
    />
  );
}

/**
 * Each change of the visual viewport's size, sent as it comes: whether a TV's or a phone's on-screen
 * keyboard shrinks the page it covers (H6). A size already sent is not sent again.
 */
export function useViewportChanges(noteEvent: (event: NotedEvent) => void): void {
  useEffect(() => {
    if (!("visualViewport" in globalThis) || globalThis.visualViewport === null) {
      return undefined;
    }
    const viewport = globalThis.visualViewport;
    let sent = `${String(Math.round(viewport.width))}x${String(Math.round(viewport.height))}`;
    const resized = (): void => {
      const width = Math.round(viewport.width);
      const height = Math.round(viewport.height);
      const size = `${String(width)}x${String(height)}`;
      if (size !== sent) {
        sent = size;
        noteEvent({ kind: "viewport", width, height, atMs: nowMs() });
      }
    };
    viewport.addEventListener("resize", resized);
    return () => {
      viewport.removeEventListener("resize", resized);
    };
  }, [noteEvent]);
}
