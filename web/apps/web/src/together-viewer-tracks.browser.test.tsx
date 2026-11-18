// SPDX-License-Identifier: MPL-2.0

/**
 * The Watch Together guest's film and its subtitle chooser as a real browser lays them out, on a
 * phone's width, with the stylesheets the viewer loads: a subtitle named by a whole release name keeps
 * to the screen, and so does the film above it.
 */

import "./tokens.css";
import "./base.css";
import "./film-player.css";
import "./route-chooser.css";
import "./subtitles.css";
import "./together-viewer.css";
import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { ActionChooser } from "./route-chooser.tsx";
import { mount } from "./test-dom.tsx";

/** A phone's width, the owner's iPhone's, in CSS pixels. */
const phoneWidthPx = 375;

/** A subtitle as a downloaded one is named: a release name with no space to wrap at. */
const releaseNamed = `English · ${"x.".repeat(40)}srt · downloaded, unverified`;

describe("a Watch Together guest's film and its subtitles, laid out by a real browser", () => {
  it("keep to a phone's width, the subtitle's long name wrapping, not widening the film", async () => {
    // The owner, 2026-11-18, the guest's iPhone: they chose the one subtitle, and "the video screen
    // grew such that it ran off the screen".
    const mounted = await mount(
      <div style={{ width: `${phoneWidthPx}px` }}>
        <section className="together-viewer__viewing">
          <div className="film-player__stage" style={{ aspectRatio: "16 / 9" }} />
          <div className="together-viewer__tracks">
            <div className="subtitles">
              <ActionChooser
                fieldLabel="Subtitles"
                current={releaseNamed}
                options={[
                  {
                    key: "release",
                    label: releaseNamed,
                    isCurrent: true,
                    onChoose: () => undefined,
                  },
                ]}
              />
            </div>
          </div>
        </section>
      </div>,
    );
    try {
      const viewing = mounted.container.querySelector<HTMLElement>(".together-viewer__viewing");
      const stage = mounted.container.querySelector<HTMLElement>(".film-player__stage");
      const chooser = mounted.container.querySelector<HTMLElement>(".route-chooser__button");
      if (viewing === null || stage === null || chooser === null) {
        throw new Error("the film and its subtitles are not drawn");
      }
      expect(viewing.scrollWidth).toBeLessThanOrEqual(phoneWidthPx);
      expect(stage.getBoundingClientRect().width).toBeLessThanOrEqual(phoneWidthPx);
      expect(chooser.getBoundingClientRect().right).toBeLessThanOrEqual(
        viewing.getBoundingClientRect().left + phoneWidthPx,
      );
    } finally {
      mounted.unmount();
    }
  });
});
