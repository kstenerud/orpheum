// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { keepsOpenAttribute } from "./popover.tsx";
import { Switch } from "./switch.tsx";
import { click, isSwitchOn, mount, switchNamed, type Mounted } from "./test-dom.tsx";

/** A switch as a setting holds it, each switch heard. */
function Tested(props: {
  readonly isOnAtStart: boolean;
  readonly status?: string;
  readonly heard: boolean[];
}): JSX.Element {
  const [isOn, setIsOn] = useState(props.isOnAtStart);
  return (
    <Switch
      label="Level loudness"
      status={props.status}
      isOn={isOn}
      onSwitch={(isNowOn) => {
        props.heard.push(isNowOn);
        setIsOn(isNowOn);
      }}
    />
  );
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("a switch, a setting applied the moment it is pressed (D538, interface rule 7)", () => {
  it("is one button of role switch, its state its aria-checked, which a press turns", async () => {
    const heard: boolean[] = [];
    mounted = await mount(<Tested isOnAtStart={false} heard={heard} />);
    const { container } = mounted;
    const found = switchNamed(container, "Level loudness");
    expect(found?.tagName).toBe("BUTTON");
    expect(found?.getAttribute("type")).toBe("button");
    expect(isSwitchOn(found)).toBe(false);
    await click(found);
    expect(isSwitchOn(found)).toBe(true);
    await click(found);
    expect(heard).toEqual([true, false]);
    expect(container.querySelectorAll("button, input")).toHaveLength(1);
  });

  it("names its status after its label, in brackets, as part of its name", async () => {
    mounted = await mount(<Tested isOnAtStart heard={[]} status="+4.1 dB" />);
    const found = switchNamed(mounted.container, "Level loudness (+4.1 dB)");
    expect(found?.querySelector(".switch__status")?.textContent).toBe(" (+4.1 dB)");
  });

  it("is marked to keep a pop-up holding it open, a setting and no choice (popover.tsx)", async () => {
    // ⁝'s and a cast remote's switches stay open for the next press (cast-remote.test.tsx).
    mounted = await mount(<Tested isOnAtStart={false} heard={[]} />);
    const found = switchNamed(mounted.container, "Level loudness");
    expect(found?.hasAttribute(keepsOpenAttribute)).toBe(true);
  });
});
