"""HTTP Cloud Function that triggers the AutoClocking marcaje workflow.

Invoked by Cloud Scheduler (entrada and salida jobs) via OIDC. Reads the GitHub
fine-grained token from a Secret Manager secret mounted as an environment
variable and fires a ``workflow_dispatch`` on the ``autoclocking-backend``
repository, so the Go marcaje job runs on demand instead of relying on GitHub
Actions cron, whose scheduling lateness silently skipped the entrada window. The
dispatched job resolves ENTRADA vs SALIDA from the local Chile time. A
``dry_run`` flag validates the token against GitHub without dispatching, so the
full chain can be verified with no workflow run and no Firestore footprint.
"""
from __future__ import annotations

import json
import os
import urllib.request
from typing import Any

import functions_framework
from flask import Request

REPO = "camiloengineer/autoclocking-backend"
WORKFLOW_FILE = "main.yml"
WORKFLOW_URL = f"https://api.github.com/repos/{REPO}/actions/workflows/{WORKFLOW_FILE}"
DISPATCH_URL = f"{WORKFLOW_URL}/dispatches"
TOKEN_ENV = "GITHUB_DISPATCH_TOKEN"
REQUEST_TIMEOUT_SECONDS = 20


def _github(method: str, url: str, token: str, body: dict[str, Any] | None) -> int:
    """Perform an authenticated GitHub API request and return the HTTP status."""
    data = json.dumps(body).encode("utf-8") if body is not None else None
    request = urllib.request.Request(url, data=data, method=method)
    request.add_header("Authorization", f"Bearer {token}")
    request.add_header("Accept", "application/vnd.github+json")
    request.add_header("X-GitHub-Api-Version", "2022-11-28")
    with urllib.request.urlopen(request, timeout=REQUEST_TIMEOUT_SECONDS) as response:
        return int(response.status)


@functions_framework.http
def trigger_marcaje(request: Request) -> tuple[str, int, dict[str, str]]:
    """Fire a workflow_dispatch on the marcaje workflow, or validate in dry_run."""
    token = os.environ[TOKEN_ENV].strip()
    headers = {"Content-Type": "application/json"}

    dry_run = str(request.args.get("dry_run", "")).lower() in ("1", "true", "yes")
    if dry_run:
        status = _github("GET", WORKFLOW_URL, token, None)
        return json.dumps({"dry_run": True, "workflow_read_status": status}), 200, headers

    debug = str(request.args.get("debug", "")).lower() in ("1", "true", "yes")
    body: dict[str, Any] = {"ref": "main"}
    if debug:
        body["inputs"] = {"debug_mode": "true"}

    status = _github("POST", DISPATCH_URL, token, body)
    ok = status == 204
    return json.dumps({"dispatched": ok, "github_status": status, "debug": debug}), (200 if ok else 502), headers
