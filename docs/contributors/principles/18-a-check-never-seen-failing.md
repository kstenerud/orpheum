> **ABOUTME:** Principle §18: A check you have never seen fail proves nothing.

# §18 — A check you have never seen fail proves nothing

"Denied", "not found", "0 dropped", "skipped" and "passed" are the cheapest results there are:
almost any broken setup produces them. **So before trusting a pass, see the same check fail on a
case where failure is certain** ([D190](../decisions/working-notes.md)). Three forms of it keep
recurring:

- **A missing prerequisite fails; it does not skip.** A yoloAI test tier was gated on an
  environment variable nothing set, and skipped itself for months on the only platform that
  could run it; building the gate that caught it found two security tests that had never run.
  Only a CI step's or lab job's named `ORPHEUM_TEST_UNCONTROLLED` list may be absent, and a test
  that cannot run on a platform is listed as uncovered, never silently passed.
- **A negative result needs a positive sibling, and empty is invalid.** A yoloAI run reported
  *"7 PASS / 0 FAIL"* and concluded the inverse of the truth, because the sandbox lacked the
  capability under test. Its guard for a security invariant passed on a shell parse error
  (A22). Our escape test first opens the forbidden path unconfined and sees it succeed
  ([D112](../decisions/working-notes.md)).
- **A fixture is built the way the real caller builds it.** Six yoloAI tests certified a function
  no backend could reach, because the fake had a combination of capabilities no real backend has
  (A15); another fix passed every test and did nothing, because the tests supplied a dependency
  the real caller lacks (A27). Red-on-revert shows a test is wired to the code, never that the
  code is wired to the program. The conformance suite is the check no fake can satisfy.

**Bites when:** a test passes on a refusal, an absence, a zero or a skip; a fixture hands the code
something its real caller would not have; or a gate has no test that makes it fail.
