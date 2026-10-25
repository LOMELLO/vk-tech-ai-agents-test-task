import json
import os
import subprocess
from pathlib import Path

import pytest

from mock_github import MockGitHub


@pytest.fixture
def github():
    with MockGitHub() as server:
        yield server


@pytest.fixture
def runner(github):
    binary = os.environ.get("GHREPORT_BIN")
    if not binary:
        raise pytest.UsageError("GHREPORT_BIN is not set")
    path = Path(binary)
    if not path.is_file():
        raise pytest.UsageError(f"GHREPORT_BIN does not exist: {binary}")

    def run(repos, cache_dir, *args, token=None, trace=None, out="-"):
        command = [str(path), "--repos", repos, "--out", out, "--cache-dir", str(cache_dir)]
        if trace is not None:
            command.extend(["--trace", str(trace)])
        command.extend(args)
        env = dict(os.environ)
        env["GITHUB_API_BASE_URL"] = github.url
        if token is not None:
            env["GITHUB_TOKEN"] = token
        return subprocess.run(
            command,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=env,
            timeout=120,
            check=False,
        )

    return run


def report_of(proc, code=0):
    assert proc.returncode == code, proc.stderr
    assert proc.stdout
    return json.loads(proc.stdout)


def trace_of(path):
    return [json.loads(line) for line in Path(path).read_text(encoding="utf-8").splitlines() if line]
