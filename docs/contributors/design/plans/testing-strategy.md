> **ABOUTME:** How a media server gets tested — the corpus, the tiers, and what can and cannot
> run in CI — which matters more here than usual because agents write the code and the gate is
> what catches them.

# Testing strategy

- **Status:** UNSPECIFIED — `../../principles/README.md` states the disposition; no strategy exists.
- **Depends on:** —

## Why it needs its own research

Most of this product's correctness lives in places a normal unit test cannot reach: real media
files, real hardware encoders, real client decoders, real filesystems. The incumbents' trackers
are full of bugs that could only have been caught with a corpus — HDR tone-mapping regressions,
"works for four minutes then errors", NVENC ignoring `-force_key_frames`, captions destroyed by
a transcode, subtitle desync on resume.

And the project's stated premise is that strong typing plus strong linting make it easier for
agents to write quality software. Types and lints catch a class of error; **none of the above is
in that class.** The gate has to be tests, and the tests need a corpus.

## What the research must settle

- **The corpus.** What files, how are they obtained or generated, where do they live, and how
  does a contributor without 200 GB of media run the suite? Are synthetic files (FFmpeg-generated
  with known properties) enough for most of it, with a small real set for the rest?
- **Tiers.** What runs per-commit, what runs nightly, what needs hardware that CI does not have.
- **Golden-output testing for the encoder.** Bit-exactness is not available across FFmpeg
  versions or hardware; what is the assertion instead — probe-level properties, VMAF/SSIM
  thresholds, segment-boundary alignment?
- **How to test a client's playback** without a human watching. Headless browsers get part of
  web; Android instrumentation gets part of Android; TV and Apple largely do not.
- **Conformance tests as a plugin-API artefact** — [D19](../../decisions/working-notes.md) and
  [D23](../../decisions/working-notes.md) both already require them (a transcoder that claims a
  profile must produce it; a transcoder must carry closed captions through). That is a test
  suite that ships as part of the contract.
