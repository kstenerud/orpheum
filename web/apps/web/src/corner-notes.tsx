// SPDX-License-Identifier: MPL-2.0

import "./corner-notes.css";

import { useCallback, useEffect, useState, type JSX } from "react";

import { Pill } from "./pill.tsx";

/** How long each note at the film's bottom-left shows, in ms: by its own timer, not an animation. */
export const cornerNoteShownMs = 5000;

/** The most notes shown there at once; the oldest goes as one more comes. */
export const mostCornerNotes = 3;

/** One note at the film's bottom-left, its id telling it from another with the same words. */
export interface CornerNote {
  readonly id: number;
  readonly text: string;
}

/** The notes shown, and the id the next one takes. */
interface Told {
  readonly nextId: number;
  readonly notes: readonly CornerNote[];
}

/**
 * The notes at the film's bottom-left (D538 §1; standards/ui.md, over the film): news of other
 * people, "Sam joined", "Controlled from Living room TV". tell adds them, newest last, keeping
 * mostCornerNotes; each goes cornerNoteShownMs after it came (CornerNotes). first are told as its
 * user is drawn; tell may be called as its user renders.
 */
export function useCornerNotes(first: readonly string[] = []): {
  readonly notes: readonly CornerNote[];
  readonly tell: (texts: readonly string[]) => void;
  readonly forget: (id: number) => void;
} {
  const [told, setTold] = useState<Told>(() => added({ nextId: 0, notes: [] }, first));
  const tell = useCallback((texts: readonly string[]) => {
    setTold((was) => added(was, texts));
  }, []);
  const forget = useCallback((id: number) => {
    setTold((was) => ({ ...was, notes: was.notes.filter((note) => note.id !== id) }));
  }, []);
  return { notes: told.notes, tell, forget };
}

/** told with texts added after its notes, the oldest past mostCornerNotes gone. */
function added(told: Told, texts: readonly string[]): Told {
  const notes = [...told.notes, ...texts.map((text, index) => ({ id: told.nextId + index, text }))];
  return { nextId: told.nextId + texts.length, notes: notes.slice(-mostCornerNotes) };
}

/**
 * The notes, a Pill each, stacked at the film's bottom-left, the newest lowest: just above the
 * overlay's rows while they show, lifted by their measured height (overlay-measure.ts), and at the
 * film's bottom edge while they hide (corner-notes.css). Each tells onGone as its time is up.
 */
export function CornerNotes(props: {
  readonly notes: readonly CornerNote[];
  readonly onGone: (id: number) => void;
}): JSX.Element {
  return (
    <div className="corner-notes">
      {props.notes.map((note) => (
        <CornerNotePill key={note.id} note={note} onGone={props.onGone} />
      ))}
    </div>
  );
}

/** One note, read out once as it shows, gone after cornerNoteShownMs. */
function CornerNotePill(props: {
  readonly note: CornerNote;
  readonly onGone: (id: number) => void;
}): JSX.Element {
  const { note, onGone } = props;
  useEffect(() => {
    const timer = setTimeout(() => {
      onGone(note.id);
    }, cornerNoteShownMs);
    return () => {
      clearTimeout(timer);
    };
  }, [note.id, onGone]);
  return <Pill>{note.text}</Pill>;
}
