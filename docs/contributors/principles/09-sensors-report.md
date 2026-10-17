> **ABOUTME:** Principle §9: Sensors report; the core decides.

# §9 — Sensors report; the core decides

Every component that observes the world — a media node, a webhook, a plugin, a probe — reports
**what it saw**. Interpretation, policy and action live in the core, which is the only place
that holds history, configuration and the user's preferences.

The test for which side of the line something falls on: *could this component determine it from
what it observed, with no memory of previous observations and no knowledge of the library?* If
yes it is a fact and the sensor reports it. If no it is a judgement and belongs in the core.

Three reasons this is not merely tidiness. A sensor that judges needs **history**, which means
duplicating core state and creating a second source of truth. A sensor is often **the component
under suspicion** — a node whose mount just vanished is not who should rule on what that means.
And scattering judgement means **nobody can reason about how the library reached its state**,
nor test that logic without hardware.

**A component reports according to its own visibility, and that bound is a fact rather than an
excuse.** A confined component sees less, not nothing — a relay that cannot read the film still
sees throughput, connection state, latency per leg and every drop. Limited visibility is not
limited usefulness, and it is usually the *unseen* part that would not have helped anyway:
nobody diagnoses stuttering by looking at video frames. Where a component sits in the path of
another's reports, it **pipes them through** rather than summarising them.

**A sensor may be asked to measure across several inputs; it may never choose them.**
"Find the segment repeated across this season" is a measurement a node can make and a *set* it
cannot assemble, because "these twelve files are one season" is a conclusion the core reached
from parses the node is forbidden to make ([D101](../decisions/working-notes.md)). So the core
supplies the list and the node supplies the number. A sensor that picks its own comparison set
has quietly acquired a copy of the library's grouping logic, and it will be wrong differently
from the original.

**The largest operator in this field arrived at the same split.** A Netflix Open Connect
Appliance — hardware inside a network Netflix does not control — does exactly two things:
*"report their status … and what files they have stored on disk"*, and *"serve content via
HTTP/HTTPS when it is requested by a client device"*. Steering, fill behaviour, storage layout
and telemetry interpretation are all in the control plane. That is this principle at planetary
scale, and it is independent evidence rather than a borrowed idea
([node-transport.md](../design/research/node-transport.md) §1).

**Bites when:** a component starts withholding, suppressing or correcting its own output
"helpfully"; when you find yourself giving a sensor a threshold, a memory, a copy of
something the core already knows, or the job of deciding which inputs go together; or when
confinement gets used as a reason a component cannot report. ([D42](../decisions/working-notes.md), [D73](../decisions/working-notes.md),
[D101](../decisions/working-notes.md), and the same reasoning as
[D37](../decisions/working-notes.md).)
