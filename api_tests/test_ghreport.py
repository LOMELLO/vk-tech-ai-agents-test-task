import json
import time
from datetime import datetime, timezone

from conftest import report_of, trace_of
from mock_github import pages_issues, since_issues


def assert_sorted(issues):
    keys = [(issue["created_at"], issue["number"]) for issue in issues]
    assert keys == sorted(keys)


def test_pagination_and_limit(github, runner, tmp_path):
    proc = runner("mock/pages", tmp_path / "cache", "--limit", "120")
    report = report_of(proc)
    repo = report["repos"][0]
    assert repo["repo"] == "mock/pages"
    assert repo["error"] is None
    assert repo["count"] == 120
    assert len(repo["issues"]) == 120
    expected = [item["number"] for item in pages_issues() if "pull_request" not in item][:120]
    assert [item["number"] for item in repo["issues"]] == expected
    assert len({item["number"] for item in repo["issues"]}) == 120
    assert {5, 25, 55, 85, 115}.isdisjoint(item["number"] for item in repo["issues"])
    assert len(github.requests) >= 2
    assert any(request["query"].get("page") == "2" for request in github.requests)
    assert github.contract_violations == []
    assert_sorted(repo["issues"])


def test_pull_request_filter(github, runner, tmp_path):
    proc = runner("mock/prs", tmp_path / "cache", "--limit", "100")
    report = report_of(proc)
    repo = report["repos"][0]
    numbers = [item["number"] for item in repo["issues"]]
    assert repo["error"] is None
    assert numbers == [1, 3, 5, 7, 9, 11, 13, 15, 17, 19]
    assert repo["count"] == len(numbers)
    assert all(item["number"] % 2 == 1 for item in repo["issues"])
    assert github.contract_violations == []


def test_since_filter(github, runner, tmp_path):
    proc = runner("mock/since", tmp_path / "cache", "--since", "2026-01-02")
    report = report_of(proc)
    repo = report["repos"][0]
    assert repo["error"] is None
    assert [item["number"] for item in repo["issues"]] == [2, 3, 4]
    assert repo["count"] == 3
    since = datetime(2026, 1, 2, tzinfo=timezone.utc)
    for item in repo["issues"]:
        created = datetime.strptime(item["created_at"], "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)
        assert created >= since
    expected = [item["number"] for item in since_issues() if item["created_at"] >= "2026-01-02T00:00:00Z"
                and "pull_request" not in item]
    assert [item["number"] for item in repo["issues"]] == expected
    assert github.contract_violations == []


def test_cache_and_conditional_requests(github, runner, tmp_path):
    cache = tmp_path / "cache"
    trace_one = tmp_path / "trace-one.jsonl"
    trace_two = tmp_path / "trace-two.jsonl"
    first = runner("mock/cache", cache, "--limit", "105", "--trace", trace_one, token="cache-secret")
    first_report = report_of(first)
    first_count = len(github.requests)
    second = runner("mock/cache", cache, "--limit", "105", "--trace", trace_two, token="cache-secret")
    second_report = report_of(second)
    assert first_report["repos"] == second_report["repos"]
    entries_one = trace_of(trace_one)
    entries_two = trace_of(trace_two)
    assert entries_one
    assert all(entry["from_cache"] is False and entry["status"] == 200 for entry in entries_one)
    assert entries_two
    assert all(entry["from_cache"] is True and entry["status"] == 304 for entry in entries_two)
    assert len(entries_one) == len(entries_two)
    first_requests = github.requests[:first_count]
    second_requests = github.requests[first_count:]
    assert first_requests
    assert all("if-none-match" not in request["headers"] for request in first_requests)
    assert second_requests
    assert all(request["headers"].get("if-none-match") for request in second_requests)
    assert "cache-secret" not in trace_one.read_text(encoding="utf-8")
    assert "cache-secret" not in trace_two.read_text(encoding="utf-8")
    assert "cache-secret" not in first.stdout
    for path in cache.rglob("*"):
        if path.is_file():
            assert "cache-secret" not in path.read_text(encoding="utf-8")


def test_rate_limit_retry(github, runner, tmp_path):
    trace = tmp_path / "trace.jsonl"
    started = time.monotonic()
    proc = runner("mock/rate", tmp_path / "cache", "--trace", trace)
    elapsed = time.monotonic() - started
    report = report_of(proc)
    assert report["repos"][0]["count"] == 3
    assert elapsed >= 0.8
    entries = trace_of(trace)
    assert [entry["status"] for entry in entries] == [429, 200]
    assert [entry["attempt"] for entry in entries] == [1, 2]
    assert all(entry["from_cache"] is False for entry in entries)
    assert github.contract_violations == []


def test_unauthorized(github, runner, tmp_path):
    proc = runner("mock/unauthorized", tmp_path / "cache")
    assert proc.returncode == 3
    assert json.loads(proc.stdout) == {"error": "unauthorized"}


def test_partial_not_found(github, runner, tmp_path):
    proc = runner("mock/missing,mock/schema", tmp_path / "cache")
    report = report_of(proc, code=1)
    missing, good = report["repos"]
    assert missing["repo"] == "mock/missing"
    assert missing["error"] == "not_found"
    assert missing["count"] == 0
    assert missing["issues"] == []
    assert good["repo"] == "mock/schema"
    assert good["error"] is None
    assert good["count"] == 4


def test_invalid_response(github, runner, tmp_path):
    proc = runner("mock/badjson,mock/schema", tmp_path / "cache")
    report = report_of(proc, code=1)
    bad, good = report["repos"]
    assert bad["error"] == "invalid_response"
    assert bad["count"] == 0
    assert bad["issues"] == []
    assert good["error"] is None


def test_all_failed(github, runner, tmp_path):
    proc = runner("mock/missing", tmp_path / "cache")
    assert proc.returncode == 3
    assert json.loads(proc.stdout) == {"error": "all_failed"}


def test_invalid_arguments(github, runner, tmp_path):
    proc = runner("mock/empty", tmp_path / "cache", "--limit", "0")
    assert proc.returncode == 2
    assert proc.stdout == ""
    assert proc.stderr


def test_report_schema(github, runner, tmp_path):
    proc = runner("mock/schema", tmp_path / "cache")
    report = report_of(proc)
    assert set(report) == {"generated_at", "repos"}
    datetime.strptime(report["generated_at"], "%Y-%m-%dT%H:%M:%SZ")
    assert len(report["repos"]) == 1
    repo = report["repos"][0]
    assert set(repo) == {"repo", "error", "count", "issues"}
    assert repo["repo"] == "mock/schema"
    assert repo["error"] is None
    assert isinstance(repo["count"], int)
    assert repo["count"] == len(repo["issues"]) == 4
    expected = [3, 5, 7, 9]
    assert [item["number"] for item in repo["issues"]] == expected
    for item in repo["issues"]:
        assert set(item) == {"number", "title", "author", "labels", "created_at", "url"}
        assert isinstance(item["number"], int)
        assert isinstance(item["title"], str)
        assert isinstance(item["author"], str)
        assert isinstance(item["labels"], list)
        assert all(isinstance(label, str) for label in item["labels"])
        assert isinstance(item["created_at"], str)
        assert isinstance(item["url"], str)
        datetime.strptime(item["created_at"], "%Y-%m-%dT%H:%M:%SZ")
    assert_sorted(repo["issues"])
    five = next(item for item in repo["issues"] if item["number"] == 5)
    assert five["labels"] == ["b", "a"]


def test_trace_fields_and_no_authorization(github, runner, tmp_path):
    trace = tmp_path / "trace.jsonl"
    proc = runner("mock/trace", tmp_path / "cache", "--trace", trace, token="trace-secret")
    report = report_of(proc)
    assert report["repos"][0]["count"] == 2
    entries = trace_of(trace)
    assert len(entries) == len(github.requests)
    for entry in entries:
        assert set(entry) == {"ts", "method", "url", "status", "attempt", "from_cache", "error"}
        assert entry["method"] == "GET"
        datetime.strptime(entry["ts"], "%Y-%m-%dT%H:%M:%SZ")
        assert entry["url"].startswith(github.url + "/repos/mock/trace/issues")
        assert entry["status"] == 200
        assert entry["attempt"] == 1
        assert entry["from_cache"] is False
        assert entry["error"] is None
        assert "authorization" not in entry
    text = trace.read_text(encoding="utf-8")
    assert "trace-secret" not in text
    assert "Authorization" not in text
    assert all(request["headers"].get("authorization") == "Bearer trace-secret" for request in github.requests)


def test_duplicate_numbers_across_pages(github, runner, tmp_path):
    proc = runner("mock/duplicates", tmp_path / "cache", "--limit", "200")
    report = report_of(proc)
    repo = report["repos"][0]
    numbers = [item["number"] for item in repo["issues"]]
    assert repo["count"] == 100
    assert numbers == list(range(1, 101))
    assert len(github.requests) >= 2
    assert github.contract_violations == []
