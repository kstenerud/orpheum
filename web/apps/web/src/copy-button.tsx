// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";

import { copyToClipboard } from "./clipboard.ts";

/** What a `CopyButton` copies, says and does; its words are the caller's, already translated. */
export interface CopyButtonProps {
  readonly text: string;
  readonly label: string;
  /** Said once the text is copied. */
  readonly copiedNote: string;
  /** Said when the browser refuses the copy; undefined to say nothing. */
  readonly refusedNote?: string | undefined;
  /** Called when the browser refuses the copy, as to offer the text to select instead. */
  readonly onRefused?: (() => void) | undefined;
  /** The copy itself; the clipboard's unless a test gives its own. */
  readonly copy?: ((text: string) => Promise<void>) | undefined;
  readonly className?: string | undefined;
  /** A space parts the note from the button, for a button in a line of text, as a paragraph's. */
  readonly isNoteSpaced?: boolean;
}

/**
 * Copy, for text to paste or send, the one Copy every page uses (AGENTS rule 11): the button, then
 * its note once pressed, saying it was copied or, as a browser that may not use the clipboard does,
 * that it was refused, the text still left where the caller shows it to select.
 */
export function CopyButton(props: CopyButtonProps): JSX.Element {
  const [outcome, setOutcome] = useState<"copied" | "refused" | undefined>(undefined);
  const copy = props.copy ?? copyToClipboard;
  const note = { copied: props.copiedNote, refused: props.refusedNote, none: undefined }[
    outcome ?? "none"
  ];
  return (
    <>
      <button
        type="button"
        className={props.className}
        onClick={() => {
          copy(props.text).then(
            () => {
              setOutcome("copied");
            },
            () => {
              setOutcome("refused");
              props.onRefused?.();
            },
          );
        }}
      >
        {props.label}
      </button>
      {props.isNoteSpaced === true ? " " : null}
      {note === undefined ? null : <output>{note}</output>}
    </>
  );
}
