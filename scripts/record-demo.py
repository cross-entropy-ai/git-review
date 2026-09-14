#!/usr/bin/env python3
"""Record a guided tour with asciinema using a disposable demo repository."""

import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parent.parent
OUTPUT = ROOT / "docs" / "demo.cast"
COLUMNS, ROWS = 192, 48


def record_demo():
    for tool in ("asciinema", "tmux", "git", "bash"):
        if not shutil.which(tool):
            raise SystemExit(f"Install {tool} before running just record-demo")
    OUTPUT.parent.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="git-review-demo-") as temporary:
        temp = Path(temporary)
        repo = temp / "pulse-api"
        subprocess.run(["bash", str(ROOT / "scripts/create-fixture.sh"), str(repo)],
                       check=True, stdout=subprocess.DEVNULL)
        binaries = temp / "bin"
        binaries.mkdir()
        (binaries / "git-review").symlink_to(ROOT / "git-review")

        shell_config = temp / "bashrc"
        shell_config.write_text("PS1='\\[\\033[38;2;139;196;255m\\]$\\[\\033[0m\\] '\n")
        env = dict(os.environ)
        for key in ("NO_COLOR", "TMUX", "PROMPT_COMMAND", "BASH_ENV", "ENV"):
            env.pop(key, None)
        env.update(PATH=str(binaries) + os.pathsep + env["PATH"],
                   PS1="\033[38;2;139;196;255m$\033[0m ",
                   TERM="xterm-256color", SHELL="/bin/bash", COLORFGBG="15;0")

        env["BASH_SILENCE_DEPRECATION_WARNING"] = "1"
        socket = str(temp / "tmux.sock")
        recorder = None

        def tmux(*args):
            return subprocess.check_output(
                ["tmux", "-f", "/dev/null", "-S", socket, *args],
                env=env, text=True, stderr=subprocess.STDOUT)

        def keys(*args):
            tmux("send-keys", "-t", "demo", *args)

        def wait_for(text):
            deadline = time.monotonic() + 8
            while time.monotonic() < deadline:
                screen = tmux("capture-pane", "-p", "-t", "demo")
                if text in screen:
                    return
                time.sleep(0.05)
            raise RuntimeError(f"Demo did not reach {text!r}:\n{screen}")

        def caption(text):
            tmux("set-option", "-t", "demo", "status-format[0]",
                 "#[fg=#8bc4ff,bold] git review #[fg=#a0aab8,nobold] | " + text)

        def command(text):
            for char in text:
                tmux("send-keys", "-t", "demo", "-l", char)
                time.sleep(0.035)
            keys("Enter")

        try:
            tmux("new-session", "-d", "-x", str(COLUMNS), "-y", str(ROWS), "-c", str(repo),
                 "-s", "demo", "/bin/bash", "--noprofile", "--rcfile", str(shell_config))
            tmux("set-option", "-t", "demo", "status-position", "top")
            tmux("set-option", "-t", "demo", "status-style", "bg=#233e5a,fg=#e1e7ef")
            tmux("set-option", "-t", "demo", "window-style", "bg=#191919,fg=#e1e7ef")
            caption("Review the whole branch, right in your terminal.")
            wait_for("$")
            attach = shlex.join(["tmux", "-S", socket, "attach-session", "-t", "demo"])
            recorder = subprocess.Popen([
                "asciinema", "rec", "--headless", "--quiet", "--overwrite",
                "--output-format", "asciicast-v2", "--window-size", f"{COLUMNS}x{ROWS}",
                "--capture-env", "TERM", "--idle-time-limit", "3",
                "--title", "git review — Review your branch. Keep your place.",
                "--command", attach, str(OUTPUT)], env=env)
            time.sleep(0.8)
            command("git review")
            wait_for("0/8 viewed")
            time.sleep(2.5)
            caption("See every file. Fold the noise.  [C]")
            keys("C")
            time.sleep(2)
            caption("Open the change that matters.  [n / Space]")
            keys("n", "n", "n", "n", "n", "Space")
            wait_for("package server")
            caption("Read the change inline: additions and deletions in one flow.")
            time.sleep(3)
            caption("Compare old and new side by side.  [s Split]")
            keys("s")
            wait_for("DIFF · SPLIT")
            wait_for("func Handler")
            time.sleep(5)
            caption("Back to inline with one key. Your place stays put.  [s Inline]")
            keys("s")
            wait_for("s Split")
            wait_for("package server")
            time.sleep(3)
            caption("Keep your place. Mark files viewed as you go.  [v]")
            keys("v")
            time.sleep(1.5)
            keys("g", "v")
            time.sleep(1)
            keys("v")
            time.sleep(1)
            keys("v")
            wait_for("4/8 viewed")
            time.sleep(1.5)
            caption("Switch to a tree. Your review stays with you.  [t]")
            keys("t")
            time.sleep(2.5)
            caption("Find the file you need, instantly.  [f]")
            keys("f")
            for char in "server":
                keys(char)
                time.sleep(0.14)
            keys("Enter")
            wait_for("package server")
            time.sleep(3)
            caption("Come back later. Your viewed files are remembered.")
            keys("q")
            time.sleep(0.3)
            command("git review")
            wait_for("4/8 viewed")
            time.sleep(2.5)
            keys("q")
            time.sleep(0.3)
            (repo / "client.ts").write_text(
                'export const healthPath = "/health";\n'
                'export const retryCount = 5;\n')
            (repo / "internal/server/ready.go").write_text(
                'package server\n\nimport "net/http"\n\n'
                '// Ready reports whether the service can accept traffic.\n'
                'func Ready(w http.ResponseWriter, r *http.Request) {\n'
                '\tw.Header().Set("Content-Type", "application/json")\n'
                '\tw.Write([]byte(`{"ready":true}`))\n}\n')
            caption("Local edits? git review automatically opens Working tree.")
            command("git status --short")
            time.sleep(1.5)
            command("git review")
            wait_for("Mode: Working tree")
            wait_for("0/2 viewed")
            keys("t", "n")
            wait_for("func Ready")
            time.sleep(3)
            caption("Switch to committed changes without leaving the review.  [m]")
            keys("m")
            wait_for("Mode: Committed")
            wait_for("4/8 viewed")
            time.sleep(2.5)
            caption("Back to local changes, including untracked files.  [m]")
            keys("m")
            wait_for("Mode: Working tree")
            wait_for("0/2 viewed")
            time.sleep(2.5)
            caption("A focused review. One binary.  git review")
            time.sleep(2)
            keys("q")
            time.sleep(0.3)
            command("exit")
            if recorder.wait(timeout=10) != 0:
                raise RuntimeError("asciinema recording failed")
        finally:
            subprocess.run(["tmux", "-S", socket, "kill-server"],
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if recorder is not None and recorder.poll() is None:
                recorder.terminate()
                recorder.wait(timeout=10)
    header, events = OUTPUT.read_text().split("\n", 1)
    metadata = json.loads(header)
    metadata.pop("command", None)
    OUTPUT.write_text(json.dumps(metadata, ensure_ascii=False) + "\n" + events)
    print(f"Recorded {OUTPUT.relative_to(ROOT)}")


if __name__ == "__main__":
    record_demo()
