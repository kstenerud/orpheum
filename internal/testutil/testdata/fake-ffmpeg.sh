#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The fake ffmpeg and ffprobe (standards/testing.md §11). testutil.InstallFakeFFmpeg
# ABOUTME: copies it into a test's directory as both tools; it records each run and replies.
#
# Each run records what it was given under calls/: its name, its arguments (NUL-separated) and
# its environment and what it read on stdin, so the test can assert the argument list, the
# explicit environment and the input. It
# then replies as the test asked: reply.stdout to stdout, or reply.ffmpeg.stdout when it is ffmpeg
# and that is present, reply.stderr to stderr, and the exit status in reply.status; with reply.output
# present, ffmpeg first writes it to its last argument when that names a file: URL, as a run making
# a file does. With reply.hold present it starts a grandchild that holds stdout open,
# records its pid in grandchild.pid, says "ready" into the FIFO named by reply.hold, and waits,
# so a test can cancel it and see that the whole process group died. With reply.progress present it
# writes it to descriptor 3, a watched run's -progress pipe, first. With reply.hang present it then
# hangs, as a stalled FFmpeg does, with its pid in hung.pid: until SIGINT, FFmpeg's graceful quit,
# when reply.hang says "until-quit", or until SIGKILL, ignoring SIGINT and SIGTERM, when it says
# "until-killed".
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
/bin/cat >"$record.stdin"

if [ -e "$directory/reply.hang" ]; then
	# Written whole and then renamed: a reader that sees hung.pid sees the pid, though this run is
	# killed a moment after, as the node's death kills it.
	printf '%s\n' "$$" >"$directory/hung.pid.$$"
	/bin/mv "$directory/hung.pid.$$" "$directory/hung.pid"
	if [ "$(/bin/cat "$directory/reply.hang")" = until-killed ]; then
		# Before the progress, which the test waits for; an ignored signal stays ignored across exec.
		trap '' INT TERM
	fi
fi

if [ -e "$directory/reply.progress" ]; then
	/bin/cat "$directory/reply.progress" >&3
fi

if [ -e "$directory/reply.hang" ]; then
	exec /bin/sleep 600
fi

if [ -e "$directory/reply.hold" ]; then
	/bin/sleep 600 &
	printf '%s\n' "$!" >"$directory/grandchild.pid"
	printf 'ready\n' >"$directory/reply.hold"
	wait
fi

if [ "$tool" = ffmpeg ] && [ -e "$directory/reply.output" ]; then
	output=
	for output in "$@"; do :; done
	case "$output" in
	file:*) /bin/cat "$directory/reply.output" >"${output#file:}" ;;
	esac
fi

if [ "$tool" = ffmpeg ] && [ -e "$directory/reply.ffmpeg.stdout" ]; then
	/bin/cat "$directory/reply.ffmpeg.stdout"
else
	/bin/cat "$directory/reply.stdout"
fi
/bin/cat "$directory/reply.stderr" >&2
exit "$(/bin/cat "$directory/reply.status")"
