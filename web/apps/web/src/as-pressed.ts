// SPDX-License-Identifier: MPL-2.0

import { useCallback, useEffect, useState } from "react";

import { useLatest } from "./latest.ts";

/**
 * How long a value pressed here waits for the receiver to say it before the receiver's word is taken
 * regardless: a press it never took (a refusal, a lost message) must not leave the control showing
 * what the receiver does not have.
 */
export const asPressedGivesUpAfterMs = 2000;

interface Pressed<T> {
  /** What the control shows. */
  readonly shown: T;
  /** What the receiver last said, as this state last read it. */
  readonly said: T;
  /** The value last sent, boxed so that a press of the same value again is a new wait; none outstanding. */
  readonly sent: { readonly value: T } | undefined;
}

/**
 * A remote's control of something the receiver keeps (the receiver's switches and steppers): shown
 * and stepped as pressed here, and sent at each press, so two quick presses send two values, each
 * stepped from the one before, never both from the receiver's slower word. The receiver's report is
 * adopted only where nothing is outstanding or it is the value last sent, so one of an earlier press
 * does not snap the control back; where it has not said the last value after
 * asPressedGivesUpAfterMs, its word is adopted anyway.
 */
export function useAsPressed<T>(
  said: T,
  send: (value: T) => void,
): readonly [shown: T, press: (value: T) => void] {
  const [state, setState] = useState<Pressed<T>>({ shown: said, said, sent: undefined });
  let current = state;
  if (!Object.is(state.said, said)) {
    // The receiver spoke anew: adopted at once with nothing outstanding or its last word said back.
    current =
      state.sent === undefined || Object.is(state.sent.value, said)
        ? { shown: said, said, sent: undefined }
        : { ...state, said };
    setState(current);
  }
  const saidRef = useLatest(said);
  const sendRef = useLatest(send);
  const { sent } = current;
  useEffect(() => {
    if (sent === undefined) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setState((now) =>
        now.sent === sent
          ? { shown: saidRef.current, said: saidRef.current, sent: undefined }
          : now,
      );
    }, asPressedGivesUpAfterMs);
    return () => {
      clearTimeout(timer);
    };
  }, [sent, saidRef]);
  const press = useCallback(
    (value: T) => {
      setState((now) => ({ ...now, shown: value, sent: { value } }));
      sendRef.current(value);
    },
    [sendRef],
  );
  return [current.shown, press];
}
