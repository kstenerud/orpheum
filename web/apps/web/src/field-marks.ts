// SPDX-License-Identifier: MPL-2.0

import { useCallback, useId, useState } from "react";

/**
 * What a field carries while the failure shown is about what was typed in it: marked invalid, and
 * described by the line that says why, so a screen reader on the field hears both. Nothing
 * otherwise.
 */
export interface FieldMark {
  readonly "aria-invalid"?: "true";
  readonly "aria-describedby"?: string;
}

/** A form's marks on the fields its last failure is about. */
export interface FieldMarks {
  /** The id the line saying what went wrong carries, which a marked field is described by. */
  readonly problemId: string;
  /** The mark of the field named field, by where the server locates it ("body.name"). */
  readonly markOf: (field: string) => FieldMark;
  /**
   * The field's change handler, onChange, which also takes its mark away: what was marked is no
   * longer what is typed.
   */
  readonly editing: <V>(field: string, onChange: (value: V) => void) => (value: V) => void;
}

/**
 * Marks the fields failed, the ones the failure shown is about (a server's refusal's are
 * failedFields'), aria-invalid and described by the line with problemId, until each is edited or
 * the failure goes: the form sent again, or another failure. A failure not about what was typed,
 * the network's or a server busy, names none and marks none. failure is what failed, compared by
 * identity: each failure is a new one.
 */
export function useFieldMarks(failure: unknown, failed: readonly string[]): FieldMarks {
  const problemId = useId();
  const [seen, setSeen] = useState<unknown>(failure);
  const [edited, setEdited] = useState<readonly string[]>([]);
  if (seen !== failure) {
    // A new failure, or none: what was edited since the last one no longer counts.
    setSeen(failure);
    setEdited([]);
  }
  const markOf = (field: string): FieldMark =>
    failure !== undefined && failed.includes(field) && !edited.includes(field)
      ? { "aria-invalid": "true", "aria-describedby": problemId }
      : {};
  const editing = useCallback(
    <V>(field: string, onChange: (value: V) => void) =>
      (value: V): void => {
        setEdited((fields) => (fields.includes(field) ? fields : [...fields, field]));
        onChange(value);
      },
    [],
  );
  return { problemId, markOf, editing };
}
