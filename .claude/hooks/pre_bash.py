#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: PreToolUse hook for shell commands: refuses adding dependencies, skipping git
# ABOUTME: hooks, force-pushing, and shell writes whose target is a gate file in this project, and
# ABOUTME: runs a test command holding the repository's test lock (scripts/gate_lock.py).
# ABOUTME: A speed bump, not a wall: a script can write a file without naming it on the
# ABOUTME: command line. The wall is the branch gate's Gate-Changed trailer and review.
"""Decide from what a command does, not from the words it contains.

A write is refused when one of its *targets* — a redirect, `tee`'s files, `cp`/`mv`'s
destination, `sed -i`'s files, `rm`'s operands — resolves to a gate path inside this project.
A command that merely mentions `Makefile`, greps for `go get`, or writes to a scratch copy of
the repository is not a write to a gate, and is let through (audit-2026-09-24 §4.1).
"""

from __future__ import annotations

import json
import os
import re
import shlex
import subprocess
import sys
from pathlib import Path

import agents
import hooklib
from gate_paths import is_gate, relative

ADD_DEPENDENCY = re.compile(r"\bgo\s+get\b|\b(?:pnpm|yarn)\s+add\b|\bpip3?\s+install\b")
NPM_INSTALL = frozenset({"i", "install", "add", "isntall"})
NPM_VALUE_FLAGS = frozenset({"--prefix", "-w", "--workspace", "-C", "--registry"})
NO_VERIFY = re.compile(r"--no-verify\b")
FORCE_PUSH = re.compile(r"\bgit\s+push\b.*(?:\s--force\b|\s-f\b|\s--force-with-lease\b)")
HEREDOC = re.compile(r"<<-?\s*(['\"]?)(\w+)\1[^\n]*\n.*?\n\s*\2\s*(?:\n|$)", re.S)
QUOTED = re.compile(r"'[^']*'|\"(?:[^\"\\]|\\.)*\"")
VARIABLE = re.compile(r"\$(?:\{(\w+)\}|(\w+))")
ASSIGNMENT = re.compile(r"^(\w+)=(.*)$", re.S)

# Commands whose every operand is written: removed, truncated, touched or re-moded.
WRITES_OPERANDS = frozenset(
    {"tee", "rm", "rmdir", "unlink", "shred", "truncate", "touch", "chmod", "chown", "chgrp"}
)
# Commands whose last operand, or `-t DIR`, is the destination.
WRITES_DESTINATION = frozenset({"cp", "mv", "install", "ln", "rsync"})
# Commands that edit their file operands in place when given `-i`.
EDITS_IN_PLACE = frozenset({"sed", "perl"})
# Words that run the command after them.
PREFIXES = frozenset({"sudo", "env", "nohup", "time", "command", "exec", "nice"})
SEPARATORS = frozenset({";", "&&", "||", "|", "&", "(", ")", ";;", "|&"})


def shell_text(command: str) -> str:
    """The command with heredoc bodies and quoted strings blanked.

    A commit message is text, not a command: its `--no-verify` or `go get` is not one.
    """
    return QUOTED.sub(" ", HEREDOC.sub(" ", command))


def tokens(command: str) -> list[str]:
    """Shell words and operators, heredoc bodies removed, newlines as `;`."""
    text = HEREDOC.sub(" ", command).replace("\n", " ; ")
    lexer = shlex.shlex(text, posix=True, punctuation_chars=True)
    lexer.whitespace_split = True
    try:
        return list(lexer)
    except ValueError:  # an unbalanced quote: fall back to words
        return text.split()


def simple_commands(words: list[str]) -> list[tuple[list[str], list[str]]]:
    """Split into (arguments, redirect targets) per simple command."""
    commands: list[tuple[list[str], list[str]]] = []
    args: list[str] = []
    targets: list[str] = []
    i = 0
    while i < len(words):
        word = words[i]
        if word in SEPARATORS or (set(word) <= set(";&|()") and word):
            commands.append((args, targets))
            args, targets = [], []
        elif set(word) <= set("<>&|") and word:
            following = words[i + 1] if i + 1 < len(words) else ""
            if ">" in word and not (word.endswith("&") and following.isdigit()):
                targets.append(following)
            i += 1  # the operand of the redirect is not an argument
        elif word.isdigit() and i + 1 < len(words) and set(words[i + 1]) <= set("<>&|"):
            pass  # the file-descriptor number of `2>`
        else:
            args.append(word)
        i += 1
    commands.append((args, targets))
    return [(a, t) for a, t in commands if a or t]


def operands(args: list[str], value_flags: frozenset[str] = frozenset()) -> list[str]:
    out: list[str] = []
    skip = False
    for arg in args:
        if skip:
            skip = False
        elif arg in value_flags:
            skip = True
        elif not arg.startswith("-") or arg == "-":
            out.append(arg)
    return out


def in_place_files(args: list[str]) -> list[str]:
    """`sed -i`/`perl -pi` operands; the script given with -e is not a file."""
    flags = [a for a in args if a.startswith("-") and not a.startswith("--")]
    in_place = any("i" in f[1:].split(".")[0] for f in flags) or any(
        a.startswith("--in-place") for a in args
    )
    return operands(args, frozenset({"-e", "-f", "--expression", "--file"})) if in_place else []


def write_targets(args: list[str]) -> list[str]:
    name = os.path.basename(args[0])
    rest = args[1:]
    if name in WRITES_OPERANDS:
        return operands(rest)
    if name in WRITES_DESTINATION:
        for i, arg in enumerate(rest):
            if arg == "-t" and i + 1 < len(rest):
                return [rest[i + 1]]
            if arg.startswith("--target-directory="):
                return [arg.split("=", 1)[1]]
        found = operands(rest)
        return found[-1:] if len(found) >= 2 else []
    if name in EDITS_IN_PLACE:
        return in_place_files(rest)
    if name == "dd":
        return [a[3:] for a in rest if a.startswith("of=")]
    return []


def repoints_hooks(args: list[str]) -> bool:
    """`git -c core.hooksPath=…` or setting it with `git config`; reading it is fine."""
    if os.path.basename(args[0]) != "git":
        return False
    lowered = [a.lower() for a in args]
    if any(a.startswith("core.hookspath=") for a in lowered):
        return True
    if "config" not in lowered or "core.hookspath" not in lowered:
        return False
    after = lowered[lowered.index("core.hookspath") + 1:]
    changes = {"set", "unset", "--unset", "--unset-all", "--replace-all", "--add"}
    return bool(changes & set(lowered)) or any(not a.startswith("-") for a in after)


class Resolver:
    """Resolves a word to a path, following the command's own `cd` and `NAME=value`."""

    def __init__(self, cwd: Path) -> None:
        self.cwd = cwd
        self.names: dict[str, str] = {}

    def expand(self, word: str) -> str | None:
        """The word with variables replaced; None when one is unknown."""
        unknown = False

        def value(match: re.Match[str]) -> str:
            nonlocal unknown
            name = match.group(1) or match.group(2)
            found = self.names.get(name, os.environ.get(name))
            if found is None:
                unknown = True
                return ""
            return found

        expanded = os.path.expanduser(VARIABLE.sub(value, word))
        return None if unknown or "$(" in expanded or "`" in expanded else expanded

    def path(self, word: str) -> Path | None:
        expanded = self.expand(word)
        if expanded is None:
            # A path built from something we cannot see: judge the part we can, as if it
            # were written from the project root.
            tail = VARIABLE.sub("", word).lstrip("/")
            return hooklib.ROOT / tail if tail else None
        return self.cwd / expanded

    def observe(self, args: list[str]) -> None:
        if all(ASSIGNMENT.match(a) for a in args):
            for a in args:
                name, raw = ASSIGNMENT.match(a).groups()  # type: ignore[union-attr]
                self.names[name] = self.expand(raw) or ""
        elif args[0] == "cd" and len(args) > 1 and args[1] != "-":
            target = self.path(args[1])
            if target is not None:
                self.cwd = target
        elif args[0] == "cd":
            self.cwd = Path.home()


def strip_prefixes(args: list[str]) -> list[str]:
    while args and (args[0] in PREFIXES or ASSIGNMENT.match(args[0])):
        args = args[1:]
    return args


def gate_writes(command: str, cwd: Path) -> list[str]:
    """Gate paths, relative to the project, that the command writes."""
    resolver = Resolver(cwd)
    hits: list[str] = []
    for args, redirects in simple_commands(tokens(command)):
        targets = list(redirects)
        if args:
            resolver.observe(args)
            runs = strip_prefixes(args)
            if runs:
                targets += write_targets(runs)
        for target in targets:
            path = resolver.path(target) if target else None
            rel = relative(str(path), hooklib.root_of(path)) if path else None
            if rel and (is_gate(rel) or is_gate(f"{rel}/x")):  # the file, or a gate directory
                hits.append(rel)
    return hits


def skips_hooks(command: str) -> bool:
    if NO_VERIFY.search(shell_text(command)):
        return True
    for args, _ in simple_commands(tokens(command)):
        runs = strip_prefixes(args)
        if runs and repoints_hooks(runs):
            return True
    return False


def npm_adds_package(command: str) -> bool:
    """Whether any `npm … install <name>` in the command names a package to add.

    `npm install` with no name only installs what the lockfile already records.
    """
    for args, _ in simple_commands(tokens(command)):
        args = strip_prefixes(args)
        if not args or args[0] != "npm":
            continue
        seen_install = False
        skip_value = False
        for token in args[1:]:
            if skip_value:
                skip_value = False
                continue
            if token in NPM_VALUE_FLAGS:
                skip_value = True
                continue
            if token.startswith("-"):
                continue
            if not seen_install:
                if token in NPM_INSTALL:
                    seen_install = True
                    continue
                break  # some other subcommand: run, ci, test
            return True
    return False


def runs_git_commit(command: str) -> bool:
    """Whether a simple command of command is `git commit`, after any prefix such as `env`."""
    for args, _ in simple_commands(tokens(command)):
        words = [word for word in args if not ASSIGNMENT.match(word)]
        while words and words[0] in PREFIXES:
            words = words[1:]
        if words and Path(words[0]).name == "git" and git_subcommand(words[1:]) == "commit":
            return True
    return False


# git's options that take the word after them as their value.
GIT_VALUE_OPTIONS = frozenset({"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path"})


def git_subcommand(words: list[str]) -> str:
    """The subcommand of a git command's words after `git`: the first that is not one of git's own
    options or an option's value; "" for none."""
    index = 0
    while index < len(words):
        word = words[index]
        if word in GIT_VALUE_OPTIONS:
            index += 2
        elif word.startswith("-"):
            index += 1
        else:
            return word
    return ""


# Make targets that run tests or lints: every gate's, whose names start so (`make help`).
LOADING_TARGET = re.compile(r"^(?:check|spot|test|cover|lint|go-test|go-lint|go-vet|web-test|web-lint)")
MAKE_VALUE_FLAGS = frozenset({"-C", "-f", "-j", "-l", "--directory", "--file", "--jobs"})
# Test runners and linters run by their own names, or through npx.
TEST_RUNNERS = frozenset({"vitest", "playwright", "golangci-lint", "pytest"})
# The gate's scripts that run tests or probes themselves.
GATE_SCRIPT = re.compile(r"(?:^|/)scripts/(?:check_\w+|spot_check|py_tests|web_tests|quiet_gate|revert_red_\w+)\.py$")


def loads_the_machine(command: str, cwd: Path) -> bool:
    """Whether a simple command of command runs tests, a lint, or a commit in this project (whose hook
    runs `make check`), through ./dev or not: work that beside a running check-pr slows both and makes
    their tests flaky (the owner, 2026-10-20). A commit in another repository, a sibling such as
    orpheum-ops, runs none of this project's gate and is not one (the owner, 2026-10-27)."""
    directory = cwd
    for args, _ in simple_commands(tokens(command)):
        words = [word for word in args if not ASSIGNMENT.match(word)]
        while words and (words[0] in PREFIXES or Path(words[0]).name == "dev"):
            words = words[1:]
        if words[:1] == ["cd"] and len(words) == 2:
            directory = directory / Path(words[1]).expanduser()
            continue
        if not words or not _runs_tests(Path(words[0]).name, words[1:]):
            continue
        if Path(words[0]).name == "git" and not in_this_project(git_directory(words[1:], directory)):
            continue
        return True
    return False


# Make targets that hold the test lock themselves, naming what they are (scripts/gate_lock.py's --as).
LOCKING_TARGET = re.compile(r"^(?:check|spot|check-pr\b.*|check-revert-red\b.*)$")
# Gate scripts that, run by hand, take as long as their gate: the Stop hook does not wait behind them.
LONG_SCRIPT = re.compile(r"(?:^|/)scripts/(?:check_revert_red|check_pr_snapshot|quiet_gate)\.py$")


def lock_kind(command: str) -> str | None:
    """What a command holds the repository's test lock as (scripts/gate_lock.py), or None when it
    needs no lock taken for it: it runs no tests, or only gates that take the lock themselves (make
    check, make spot, check-pr, revert-red's targets) and commits, whose spot check does. Agents' test
    runs by hand beside each other and beside the gates loaded the machine to 70 on 8 cores and failed
    each other's browser tests (the owner, 2026-11-04)."""
    kind = None
    for args, _ in simple_commands(tokens(command)):
        words = [word for word in args if not ASSIGNMENT.match(word)]
        while words and (words[0] in PREFIXES or Path(words[0]).name == "dev"):
            words = words[1:]
        if not words or not _runs_tests(Path(words[0]).name, words[1:]):
            continue
        name = Path(words[0]).name
        if name == "git":
            continue
        if name == "make" and all(LOCKING_TARGET.match(target) for target in operands(words[1:], MAKE_VALUE_FLAGS)
                                  if LOADING_TARGET.match(target)):
            continue
        if name.startswith("python") and any(LONG_SCRIPT.search(word) for word in operands(words[1:])):
            return "gate"
        kind = "test"
    return kind


def holding_the_lock(command: str, kind: str) -> str:
    """command, run holding the repository's test lock as kind: in its own shell, so a `cd` in it no
    longer carries to the next command."""
    script = hooklib.ROOT / "scripts" / "gate_lock.py"
    return f"{shlex.quote(sys.executable)} {shlex.quote(str(script))} --as {kind} bash -c {shlex.quote(command)}"


# The Bash tool's timeout when a call names none.
DEFAULT_TIMEOUT_MS = 120_000
# The longest timeout a call can ask for.
LONGEST_TIMEOUT_MS = 600_000


def waits_on_the_lock(command: str) -> bool:
    """Whether command takes the repository's test lock, itself (lock_kind), through a gate that takes
    it (make check, spot, check-pr, revert-red), or through a commit, whose spot check takes it."""
    if lock_kind(command) is not None:
        return True
    for args, _ in simple_commands(tokens(command)):
        words = [word for word in args if not ASSIGNMENT.match(word)]
        while words and (words[0] in PREFIXES or Path(words[0]).name == "dev"):
            words = words[1:]
        if not words:
            continue
        name = Path(words[0]).name
        if name == "git" and git_subcommand(words[1:]) == "commit":
            return True
        if name == "make" and any(LOCKING_TARGET.match(target) for target in operands(words[1:], MAKE_VALUE_FLAGS)):
            return True
    return False


def wait_refusal(event: dict, command: str) -> str | None:
    """Why command should not run in the foreground now: waiting for the test lock would use more than
    half its timeout, and the tool would kill it as it waits (the lock's wargaming, 2026-11-04: make
    check takes up to 5 min, an agent's commit behind one timed out at 2 min, before it began)."""
    tool_input = event.get("tool_input") or {}
    if tool_input.get("run_in_background") or not waits_on_the_lock(command):
        return None
    limit = float(tool_input.get("timeout") or DEFAULT_TIMEOUT_MS) / 1000
    import gate_lock  # noqa: PLC0415 (only a command that waits on the lock needs it)

    try:
        wait, said = gate_lock.expected_wait(hooklib.ROOT)
    except (OSError, subprocess.CalledProcessError):
        return None
    if wait <= limit / 2:
        return None
    needed_ms = int((wait * 2 + 60) * 1000)
    longer = (f"or give it a timeout of at least {needed_ms} ms" if needed_ms <= LONGEST_TIMEOUT_MS
              else "the wait is past the longest timeout a call can have")
    return (f"This command takes the repository's test lock, which is busy: {said}. Waiting would take about "
            f"{wait / 60:.0f} min, more than half this call's {limit:.0f} s timeout, and the tool would kill it "
            f"while it waits. Run it with run_in_background (you are told when it ends), {longer}. "
            "`python3 scripts/gate_lock.py --held` names the holder.")


def git_directory(words: list[str], directory: Path) -> Path:
    """The directory a git command's words run it in, from directory: each `-C` moves it on."""
    for option, value in zip(words, words[1:], strict=False):
        if option == "-C":
            directory = directory / Path(value).expanduser()
    return directory


def in_this_project(directory: Path) -> bool:
    """Whether directory is in this clone or one of its worktrees, which share its git directory; a
    directory git cannot place counts as in it, so a doubt never lets a commit beside check-pr."""
    def common_dir(where: Path) -> str | None:
        done = subprocess.run(["git", "-C", str(where), "rev-parse", "--path-format=absolute", "--git-common-dir"],
                              capture_output=True, text=True, check=False)
        return str(Path(done.stdout.strip()).resolve()) if done.returncode == 0 else None

    theirs = common_dir(directory)
    return theirs is None or theirs == common_dir(hooklib.ROOT)


def _runs_tests(name: str, rest: list[str]) -> bool:
    if name == "make":
        return any(LOADING_TARGET.match(target) for target in operands(rest, MAKE_VALUE_FLAGS))
    if name == "git":
        return git_subcommand(rest) == "commit"
    if name == "go":
        return operands(rest)[:1] in (["test"], ["vet"])
    if name == "npm":
        named = operands(rest, NPM_VALUE_FLAGS)
        return named[:1] in (["test"], ["t"]) or (
            named[:1] == ["run"] and len(named) > 1 and re.match(r"(?:test|lint|check)", named[1]) is not None)
    if name == "npx":
        return any(word in TEST_RUNNERS for word in operands(rest)[:1])
    if name.startswith("python"):
        return any(GATE_SCRIPT.search(word) for word in operands(rest))
    return name in TEST_RUNNERS


def hooks_installed(root: Path) -> bool:
    """Whether git runs this project's hooks in root: core.hooksPath is its .githooks, as `make setup`
    sets it, or the same directory by its absolute path, as a tool that adds a worktree rewrites it.
    Without them a commit records whatever the tree holds, the gate passed or not."""
    done = subprocess.run(["git", "-C", str(root), "config", "--get", "core.hooksPath"], capture_output=True,
                          text=True, check=False)
    hooks_path = done.stdout.strip()
    if hooks_path == ".githooks":
        return True
    return bool(hooks_path) and Path(hooks_path).is_absolute() and \
        Path(hooks_path).resolve() == (root / ".githooks").resolve()


def main() -> None:
    event = hooklib.read_event()
    command = str((event.get("tool_input") or {}).get("command") or "")
    if not command:
        return
    refusals: list[str] = []
    cwd = Path(str(event.get("cwd") or hooklib.ROOT))
    adds = ADD_DEPENDENCY.search(shell_text(command)) or npm_adds_package(command)
    if adds and not hooklib.owner_allows("ORPHEUM_ALLOW_DEPS", event):
        refusals.append(
            "Adding a dependency is the owner's decision (AGENTS.md rule 3). Propose it: the "
            "name, the job it does, and why the standard library or an allowed package cannot. "
            "`npm ci` and `go mod download` are fine."
        )
    elif skips_hooks(command):
        refusals.append("Git hooks are part of the gate; don't bypass or repoint them.")
    elif runs_git_commit(command) and not hooks_installed(hooklib.ROOT):
        # A commit made after a failed `make check`, chained with `;`, went in because nothing ran
        # the gate at the commit: this clone had no hooks (the owner, 2026-10-08: "If you have to
        # remember to do the && properly, that's a process failure").
        refusals.append(
            "Git's hooks are not installed in this clone (core.hooksPath is not .githooks), so this "
            "commit would not run the gate. Run `make setup` first, then commit."
        )
    elif FORCE_PUSH.search(shell_text(command)):
        refusals.append("No force-pushes. Add a commit that fixes the problem instead.")
    elif loads_the_machine(command, cwd) and (run := hooklib.check_pr_running()) is not None:
        # Edits made while check-pr ran, and tests run beside it, failed its runs again and again
        # though a written rule forbade them (the owner, 2026-10-20: "trying to remember will lead
        # to failures"). It runs in its own tree now, so edits are fine; tests and commits wait.
        import check_pr_snapshot  # noqa: PLC0415

        refusals.append(
            f"check-pr is running ({check_pr_snapshot.describe(run)}) in its own tree. This command "
            "runs tests, a lint or a commit, which beside it slow both and make tests flaky. Edit "
            "freely; test and commit when it ends (you are told). If it is stuck, find its processes "
            "with `ps aux | grep '[c]heck_pr_snapshot'` and stop them."
        )
    elif not hooklib.owner_allows("ORPHEUM_ALLOW_GATE_EDIT", event):
        hits = gate_writes(command, cwd)
        if hits:
            refusals.append(
                f"This command writes {', '.join(sorted(set(hits)))}, which is a gate "
                "(scripts/gate_paths.py). Gates are the owner's to change; tell the owner what "
                "to change and why."
            )

    if refusals:
        hooklib.deny(refusals[0])
        return
    if in_this_project(cwd) and (waited := wait_refusal(event, command)) is not None:
        hooklib.deny(waited)
        return
    kind = lock_kind(command)
    if kind is not None and not in_this_project(cwd):
        kind = None
    run = holding_the_lock(command, kind) if kind is not None else command
    if agents.is_agent_id(event.get("agent_id")):
        tag_for_its_agent(event, run)
    elif run != command:
        rewrite(event, run)


def rewrite(event: dict, command: str) -> None:
    """Runs command in place of the one the tool was given."""
    tool_input = dict(event.get("tool_input") or {})
    tool_input["command"] = command
    json.dump({"hookSpecificOutput": {"hookEventName": "PreToolUse", "updatedInput": tool_input}}, sys.stdout)


def tag_for_its_agent(event: dict, command: str) -> None:
    """A subagent's command, rewritten to carry its agent's id, its stdin closed and, in the
    background, a time limit (agents.wrapped); the agent recorded as acting, so neither its
    processes nor its worktree are taken for a stopped agent's."""
    agent_id = str(event["agent_id"])
    agents.mark_active(agent_id)
    tool_input = dict(event.get("tool_input") or {})
    rewrite(event, agents.wrapped(command, agent_id, is_background=bool(tool_input.get("run_in_background"))))


if __name__ == "__main__":
    main()
