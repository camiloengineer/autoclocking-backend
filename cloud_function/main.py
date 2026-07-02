"""HTTP Cloud Function for storing and listing autoclocking events."""
from __future__ import annotations

import json
from datetime import datetime, timezone
from typing import Any

import functions_framework
from flask import Request
from google.cloud import firestore

COLLECTION = "marcajes"
DEFAULT_LIMIT = 100
MAX_LIMIT = 500

VALID_ACTIONS = ("ENTRADA", "SALIDA", "FERIADO")
VALID_STATUS = ("success", "error", "info")

CORS_HEADERS = {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type",
}

_db = firestore.Client()


def _json_response(payload: dict[str, Any], status: int) -> tuple[str, int, dict[str, str]]:
    headers = {"Content-Type": "application/json", **CORS_HEADERS}
    return json.dumps(payload, default=str, ensure_ascii=False), status, headers


def _store_marcaje(request: Request) -> tuple[str, int, dict[str, str]]:
    body = request.get_json(silent=True)
    if not isinstance(body, dict):
        return _json_response({"error": "JSON body is required"}, 400)

    action_type = body.get("action_type")
    status_value = body.get("status")
    if action_type not in VALID_ACTIONS or status_value not in VALID_STATUS:
        return _json_response({"error": "Invalid action_type or status"}, 400)

    doc: dict[str, Any] = {
        "action_type": action_type,
        "status": status_value,
        "message": str(body.get("message", "")),
        "details": str(body.get("details", "")),
        "rut_masked": str(body.get("rut_masked", "")),
        "run_number": str(body.get("run_number", "")),
        "fecha_clt": str(body.get("fecha_clt", "")),
        "created_at": datetime.now(timezone.utc),
    }
    _, ref = _db.collection(COLLECTION).add(doc)
    return _json_response({"id": ref.id, **doc}, 201)


def _list_marcajes(request: Request) -> tuple[str, int, dict[str, str]]:
    raw_limit = request.args.get("limit", str(DEFAULT_LIMIT))
    try:
        limit = max(1, min(int(raw_limit), MAX_LIMIT))
    except ValueError:
        limit = DEFAULT_LIMIT

    query = (
        _db.collection(COLLECTION)
        .order_by("created_at", direction=firestore.Query.DESCENDING)
        .limit(limit)
    )
    items = [{"id": doc.id, **(doc.to_dict() or {})} for doc in query.stream()]
    return _json_response({"count": len(items), "items": items}, 200)


@functions_framework.http
def marcajes(request: Request) -> tuple[str, int, dict[str, str]]:
    if request.method == "OPTIONS":
        return "", 204, CORS_HEADERS
    if request.method == "POST":
        return _store_marcaje(request)
    if request.method == "GET":
        return _list_marcajes(request)
    return _json_response({"error": "Method not supported"}, 405)
