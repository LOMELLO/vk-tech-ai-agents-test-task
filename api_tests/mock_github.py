import hashlib
import json
import threading
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

BASE_TIME = datetime(2026, 1, 1, tzinfo=timezone.utc)


def make_issue(number, created_at, title=None, author=None, labels=None, pull_request=False):
    issue = {
        "number": number,
        "title": title or f"Issue {number}",
        "user": {"login": author or f"user{number}"},
        "labels": [{"name": name} for name in (labels or [])],
        "created_at": created_at,
        "html_url": f"https://github.com/example/repo/issues/{number}",
    }
    if pull_request:
        issue["pull_request"] = {"url": f"https://api.github.com/repos/example/repo/pulls/{number}"}
    return issue


def iso(value):
    return value.strftime("%Y-%m-%dT%H:%M:%SZ")


def pages_issues():
    pull_requests = {5, 25, 55, 85, 115}
    return [
        make_issue(number, iso(BASE_TIME + timedelta(minutes=number)), pull_request=number in pull_requests)
        for number in range(1, 156)
    ]


def pr_issues():
    return [
        make_issue(number, iso(BASE_TIME + timedelta(minutes=number)), pull_request=number % 2 == 0)
        for number in range(1, 21)
    ]


def since_issues():
    return [
        make_issue(1, "2026-01-01T12:00:00Z"),
        make_issue(2, "2026-01-02T00:00:00Z"),
        make_issue(3, "2026-01-02T12:00:00Z"),
        make_issue(4, "2026-01-03T00:00:00Z"),
        make_issue(5, "2026-01-04T00:00:00Z", pull_request=True),
    ]


def schema_issues():
    return [
        make_issue(3, "2026-01-01T00:00:00Z", title="First", author="alice"),
        make_issue(7, "2026-01-02T00:00:00Z", title="Tie Seven", author="bob"),
        make_issue(5, "2026-01-02T00:00:00Z", title="Tie Five", author="carol", labels=["b", "a"]),
        make_issue(9, "2026-01-03T00:00:00Z", title="Later", author="dave", labels=["bug"]),
    ]


def cache_issues():
    return [make_issue(number, iso(BASE_TIME + timedelta(minutes=number))) for number in range(1, 106)]


def rate_issues():
    return [make_issue(number, iso(BASE_TIME + timedelta(minutes=number))) for number in range(1, 4)]


def trace_issues():
    return [make_issue(number, iso(BASE_TIME + timedelta(minutes=number))) for number in range(1, 3)]


def duplicate_issues():
    items = [make_issue(number, iso(BASE_TIME + timedelta(minutes=number))) for number in range(1, 101)]
    items.append(make_issue(1, "2026-01-01T00:01:00Z"))
    return items


class Scenario:
    def __init__(self, issues, etag=False, rate_limit_first=0):
        self.issues = issues
        self.etag = etag
        self.rate_limit_first = rate_limit_first
        self._per_page = None
        self._pages = []

    def pages(self, per_page):
        if self._per_page != per_page:
            self._per_page = per_page
            self._pages = [self.issues[index:index + per_page] for index in range(0, len(self.issues), per_page)]
        return self._pages


class MockState:
    def __init__(self):
        self.requests = []
        self.contract_violations = []
        self.counters = {}
        self.lock = threading.Lock()
        self.url = ""
        self.scenarios = {
            "mock/pages": Scenario(pages_issues()),
            "mock/prs": Scenario(pr_issues()),
            "mock/since": Scenario(since_issues()),
            "mock/schema": Scenario(schema_issues()),
            "mock/cache": Scenario(cache_issues(), etag=True),
            "mock/rate": Scenario(rate_issues(), rate_limit_first=1),
            "mock/trace": Scenario(trace_issues()),
            "mock/duplicates": Scenario(duplicate_issues()),
            "mock/empty": Scenario([]),
        }


class MockHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, format, *args):
        pass

    def do_GET(self):
        state = self.server.state
        parsed = urlsplit(self.path)
        query = {key: values[-1] for key, values in parse_qs(parsed.query, keep_blank_values=True).items()}
        headers = {key.lower(): value for key, value in self.headers.items()}
        with state.lock:
            state.requests.append({
                "path": parsed.path,
                "query": query,
                "headers": headers,
            })
        parts = parsed.path.strip("/").split("/")
        if len(parts) != 4 or parts[0] != "repos" or parts[3] != "issues":
            self._send(404, b'{"message":"Not Found"}')
            return
        owner, repo = parts[1], parts[2]
        key = f"{owner}/{repo}"
        violations = []
        expected_query = {"state": "open", "sort": "created", "direction": "asc", "per_page": "100"}
        for name, value in expected_query.items():
            if query.get(name) != value:
                violations.append(f"query {name}={query.get(name)!r}")
        if headers.get("accept") != "application/vnd.github+json":
            violations.append("accept header")
        if headers.get("x-github-api-version") != "2026-03-10":
            violations.append("api version header")
        if not headers.get("user-agent"):
            violations.append("user agent header")
        if violations:
            with state.lock:
                state.contract_violations.append({"repo": key, "violations": violations})
            self._send(400, b'{"message":"contract violation"}')
            return
        if key == "mock/missing":
            self._send(404, b'{"message":"Not Found"}')
            return
        if key == "mock/unauthorized":
            self._send(401, b'{"message":"Bad credentials"}')
            return
        if key == "mock/badjson":
            self._send(200, b'{"broken":', {"Content-Type": "application/json"})
            return
        scenario = state.scenarios.get(key)
        if scenario is None:
            self._send(404, b'{"message":"Not Found"}')
            return
        if scenario.rate_limit_first:
            with state.lock:
                count = state.counters.get(key, 0)
                state.counters[key] = count + 1
            if count < scenario.rate_limit_first:
                self._send(429, b'{"message":"API rate limit exceeded"}',
                           {"Retry-After": "1", "Content-Type": "application/json"})
                return
        try:
            page = int(query.get("page", "1"))
        except ValueError:
            page = 1
        pages = scenario.pages(100)
        items = pages[page - 1] if 1 <= page <= len(pages) else []
        body = json.dumps(items, ensure_ascii=False, separators=(",", ":")).encode()
        headers_out = {"Content-Type": "application/json"}
        if page < len(pages):
            base = state.url
            next_url = (f"{base}/repos/{owner}/{repo}/issues?state=open&sort=created&direction=asc"
                        f"&per_page=100&page={page + 1}")
            last_url = (f"{base}/repos/{owner}/{repo}/issues?state=open&sort=created&direction=asc"
                        f"&per_page=100&page={len(pages)}")
            headers_out["Link"] = f'<{next_url}>; rel="next", <{last_url}>; rel="last"'
        if scenario.etag:
            etag = '"' + hashlib.sha256(body).hexdigest() + '"'
            headers_out["ETag"] = etag
            if headers.get("if-none-match") == etag:
                if "Link" in headers_out:
                    self._send(304, b"", {"ETag": etag, "Link": headers_out["Link"]})
                else:
                    self._send(304, b"", {"ETag": etag})
                return
        self._send(200, body, headers_out)

    def _send(self, status, body, headers=None):
        self.send_response(status)
        for name, value in (headers or {}).items():
            self.send_header(name, value)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if body:
            self.wfile.write(body)


class MockGitHub:
    def __init__(self):
        self.state = MockState()
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), MockHandler)
        self.server.state = self.state
        self.state.url = f"http://127.0.0.1:{self.server.server_port}"
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)

    @property
    def url(self):
        return self.state.url

    @property
    def requests(self):
        with self.state.lock:
            return list(self.state.requests)

    @property
    def contract_violations(self):
        with self.state.lock:
            return list(self.state.contract_violations)

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, exc_type, exc, tb):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
