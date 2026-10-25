// SPDX-License-Identifier: MPL-2.0

/**
 * The button test's controllers (plans/tv-device-tests.md H8): the pads the app's shared poll reads
 * (input-pad-poll.ts), each press, pad seen and axis crossing sent to the log, the first pad shown
 * with its layout and the choice of OK, kept as the This device page keeps it (controller-choice.ts).
 * The app's navigation moves focus by a pad's arrows and presses by its OK, as on every page.
 */

import { useEffect, useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import type { Direction, OkButton } from "./input-buttons.ts";
import {
  listenToPads,
  type PadButtonPress,
  type PadHandlers,
  type PadState,
  type StickPress,
} from "./input-pad-poll.ts";
import { padMappingOf } from "./input-pads.ts";
import type { NotedEvent } from "./lab-events.ts";

/**
 * The pads connected as the shared poll reads them (input-pad-poll.ts), drawn again only when one
 * changed, not each frame; each change told to handlers.
 */
export function usePads(handlers: Partial<PadHandlers>): readonly PadState[] {
  const [pads, setPads] = useState<readonly PadState[]>([]);
  useEffect(() => {
    let shown = "";
    return listenToPads({
      ...handlers,
      onRead: (read) => {
        const reading = JSON.stringify(read);
        if (reading !== shown) {
          shown = reading;
          setPads(read);
        }
      },
    });
  }, [handlers]);
  return pads;
}

/** How much of a controller's name the log keeps: the server's bound (lab.go). A justified constant. */
export const padIdKept = 100;

/** When something came, on the page's clock, in whole milliseconds. */
function nowMs(): number {
  return Math.round(performance.now());
}

/**
 * What the test does with its pads: a press is noted, a pad seen and an axis sent, and an arrow's
 * time noted for its move's latency, before the app's navigation moves focus by it.
 */
export function usePadHandlers(
  note: (press: PadButtonPress | StickPress) => void,
  noteEvent: (event: NotedEvent) => void,
  onArrow: (direction: Direction) => void,
): Partial<PadHandlers> {
  return useMemo(
    () => ({
      onSeen: (pad) => {
        noteEvent({
          kind: "pad",
          padIndex: pad.index,
          padId: pad.id.slice(0, padIdKept),
          padMapping: padMappingOf(pad.mapping),
          padLayout: pad.layout,
          okButton: pad.okButton,
          atMs: nowMs(),
        });
      },
      onPress: (press) => {
        note(press);
      },
      onAxis: (crossing) => {
        noteEvent({
          kind: "axis",
          padIndex: crossing.index,
          axisIndex: crossing.axis,
          axisValue: crossing.value,
          atMs: nowMs(),
        });
      },
      onArrow,
    }),
    [note, noteEvent, onArrow],
  );
}

/** How much of a controller's name the page shows on its one line: enough to know it by. */
const padIdShown = 32;

/** The first controller connected, on one line: its name, its mapping, its layout, and its OK. */
export function PadLine(props: {
  readonly pads: readonly PadState[];
  readonly onChoose: (okButton: OkButton) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const pad = props.pads[0];
  if (pad === undefined) {
    return <p className="lab__small">{t("lab.input.controller.none")}</p>;
  }
  const id = pad.id.length > padIdShown ? `${pad.id.slice(0, padIdShown - 1)}…` : pad.id;
  // Every standard pad offers the choice: a pad's labels need not match the layout it reports.
  const isChosen = pad.isStandard;
  const layout = t(`lab.input.controller.layout.${pad.layout}`);
  const ok = t(`lab.input.controller.ok.${pad.okButton}`);
  let said = t("lab.input.controller.noStandard");
  if (pad.isStandard) {
    said =
      pad.layout === "unknown"
        ? t("lab.input.controller.unknown")
        : t("lab.input.controller.standard", { layout, ok });
  }
  return (
    <div className="lab__controls lab__small">
      <span>
        {t("lab.input.controller.line", { id, mapping: padMappingOf(pad.mapping), said })}
      </span>
      {isChosen ? (
        <fieldset className="lab__modes">
          <legend>{t("lab.input.controller.choose")}</legend>
          {(["bottom", "right"] as const).map((each) => (
            <button
              key={each}
              type="button"
              className="lab__button"
              aria-pressed={pad.okButton === each}
              onClick={() => {
                props.onChoose(each);
              }}
            >
              {t(`lab.input.controller.ok.${each}`)}
            </button>
          ))}
        </fieldset>
      ) : null}
    </div>
  );
}
