#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: The fake ffmpeg and ffprobe (standards/testing.md §11). testutil.InstallFakeFFmpeg
# ABOUTME: copies it into a test's directory as both tools; it records each run and replies.
#
# Each run records what it was given under calls/: its name, its arguments (NUL-separated) and
# its environment, so the test can assert the argument list and the explicit environment. It
# then replies as the test asked: reply.stdout to stdout, reply.stderr to stderr, and the exit
# status in reply.status. With reply.hold present it starts a grandchild that holds stdout open,
# records its pid in grandchild.pid, says "ready" into the FIFO named by reply.hold, and waits,
# so a test can cancel it and see that the whole process group died.
#
# It names every command by absolute path: execx runs it with an empty environment.
set -eu

directory=${0%/*}
tool=${0##*/}
record="$directory/calls/$tool.$$"

printf '%s\n' "$tool" >"$record.tool"
: >"$record.arguments"
for argument in "$@"; do
	printf '%s\0' "$argument" >>"$record.arguments"
done
/usr/bin/env >"$record.environment"

if [ -e "$directory/reply.hold" ]; then
	/bin/sleep 600 &
	printf '%s\n' "$!" >"$directory/grandchild.pid"
	printf 'ready\n' >"$directory/reply.hold"
	wait
fi

/bin/cat "$directory/reply.stdout"
/bin/cat "$directory/reply.stderr" >&2
exit "$(/bin/cat "$directory/reply.status")"
