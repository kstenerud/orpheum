#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: make's shell while quiet_gate.py runs a gate: runs each recipe line in bash, and records
# ABOUTME: its target, start, end and exit in $STEP_CLOCK, so the gate can say how long each step took.
#
# Usage, as make's SHELL with `.SHELLFLAGS = $@ <bash's flags>`: step_clock.sh TARGET FLAGS... COMMAND.
# A $(shell ...) at parse time has no target, so its first word is a flag: then it is bash alone.
# One line per recipe line, appended (a short write with O_APPEND is whole beside the others):
# TARGET <tab> START <tab> END <tab> EXIT <tab> MAKELEVEL <tab> PARENT, the times in seconds since the
# epoch. PARENT is the step whose recipe started the sub-make this line ran in (empty at the top): each
# step exports its name, and a sub-make's shells inherit it, so a step that ran others is named exactly,
# where a deeper step merely within its time may be a sibling's.
case "${1:-}" in
  "" | -*) exec bash "$@" ;;
esac
target=$1
shift
parent=${STEP_CLOCK_STEP:-}
start=${EPOCHREALTIME:-$(date +%s.%N)}
code=0
STEP_CLOCK_STEP=$target bash "$@" || code=$?
printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$target" "$start" "${EPOCHREALTIME:-$(date +%s.%N)}" "$code" "${MAKELEVEL:-0}" "$parent" >>"${STEP_CLOCK:-/dev/null}"
exit "$code"
