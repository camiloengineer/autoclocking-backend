"""HTTP Cloud Function that purges marcajes older than the retention window.

Invoked daily by Cloud Scheduler. Deletes every document in the ``marcajes``
collection whose ``created_at`` is older than ``RETENTION_DAYS`` days, keeping a
rolling window. Strictly scoped to ``marcajes`` so sibling collections in the
shared default database are never touched. A ``dry_run`` flag reports the count
that would be deleted without mutating anything.
"""
from __future__ import annotations

import json
import os
from datetime import datetime, timedelta, timezone
from typing import Any

import functions_framework
from flask import Request
from google.cloud import firestore

COLLECTION = "marcajes"
RETENTION_DAYS = int(os.environ.get("RETENTION_DAYS", "30"))
BATCH_SIZE = 400

_db = firestore.Client()


def _expired_query(cutoff: datetime, limit: int):
    return (
        _db.collection(COLLECTION)
        .where(filter=firestore.FieldFilter("created_at", "<", cutoff))
        .limit(limit)
    )


def _count_expired(cutoff: datetime) -> int:
    aggregation = _db.collection(COLLECTION).where(
        filter=firestore.FieldFilter("created_at", "<", cutoff)
    ).count()
    result = aggregation.get()
    return int(result[0][0].value)


def _purge_expired(cutoff: datetime) -> int:
    deleted = 0
    while True:
        docs = list(_expired_query(cutoff, BATCH_SIZE).stream())
        if not docs:
            break
        batch = _db.batch()
        for doc in docs:
            batch.delete(doc.reference)
        batch.commit()
        deleted += len(docs)
        if len(docs) < BATCH_SIZE:
            break
    return deleted


@functions_framework.http
def purge_marcajes(request: Request) -> tuple[str, int, dict[str, str]]:
    dry_run = str(request.args.get("dry_run", "")).lower() in ("1", "true", "yes")
    cutoff = datetime.now(timezone.utc) - timedelta(days=RETENTION_DAYS)

    affected = _count_expired(cutoff) if dry_run else _purge_expired(cutoff)

    payload: dict[str, Any] = {
        "collection": COLLECTION,
        "retention_days": RETENTION_DAYS,
        "cutoff": cutoff.isoformat(),
        "dry_run": dry_run,
        "deleted": 0 if dry_run else affected,
        "would_delete": affected if dry_run else 0,
    }
    return json.dumps(payload, default=str), 200, {"Content-Type": "application/json"}
