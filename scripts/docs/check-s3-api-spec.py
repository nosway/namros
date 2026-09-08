#!/usr/bin/env python3
"""Check the published NAMROS S3 support catalog against gateway routing."""

from __future__ import annotations

import json
import re
import sys
from collections import Counter
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = ROOT / "docs-src/api/namros-s3.openapi.json"
ROUTER = ROOT / "internal/s3api/routing/parser.go"
HANDLER = ROOT / "internal/gateway/s3_handlers.go"
REFERENCES = {
    ROOT / "docs-src/manuals/s3-api-compatibility-reference.md": {
        "Community": "community",
        "Partial": "community-partial",
        "Compatibility only": "compatibility-only",
        "Enterprise-gated": "enterprise-gated",
    },
    ROOT / "docs-src/manuals/ko/s3-api-compatibility-reference.md": {
        "Community": "community",
        "부분 지원": "community-partial",
        "호환성 전용": "compatibility-only",
        "Enterprise 게이트": "enterprise-gated",
    },
}
ALLOWED_STATUSES = {
    "community",
    "community-partial",
    "compatibility-only",
    "enterprise-gated",
}
ALLOWED_ROUTING_ALIASES = {
    "GetBucketLifecycleConfiguration": "GetBucketLifecycle",
    "PutBucketLifecycleConfiguration": "PutBucketLifecycle",
    "GetBucketAcl": "GetBucketACL",
    "PutBucketAcl": "PutBucketACL",
    "GetObjectAcl": "GetObjectACL",
    "PutObjectAcl": "PutObjectACL",
    "UploadPartCopy": "UploadPart",
}


def fail(message: str) -> None:
    print(f"[s3-api-spec-check] ERROR: {message}", file=sys.stderr)
    raise SystemExit(1)


def load_json(path: Path) -> dict:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        fail(f"missing file: {path.relative_to(ROOT)}")
    except json.JSONDecodeError as exc:
        fail(f"invalid JSON in {path.relative_to(ROOT)}: {exc}")


def routing_operations(source: str) -> tuple[set[str], dict[str, str]]:
    declarations = dict(
        re.findall(
            r"\b(Operation[A-Za-z0-9]+)\s+Operation\s*=\s*\"([^\"]+)\"",
            source,
        )
    )
    if not declarations:
        fail("could not find routing Operation declarations")
    declarations.pop("OperationUnsupported", None)
    return set(declarations.values()), declarations


def braced_block(source: str, marker: str) -> str:
    marker_index = source.find(marker)
    if marker_index < 0:
        fail(f"could not find {marker!r}")
    opening = source.find("{", marker_index + len(marker))
    if opening < 0:
        fail(f"could not find opening brace after {marker!r}")
    depth = 0
    for index in range(opening, len(source)):
        if source[index] == "{":
            depth += 1
        elif source[index] == "}":
            depth -= 1
            if depth == 0:
                return source[opening + 1 : index]
    fail(f"could not find closing brace after {marker!r}")


def main() -> None:
    spec = load_json(SPEC)
    if not str(spec.get("openapi", "")).startswith("3.1."):
        fail("OpenAPI document must use the 3.1.x line supported by the docs viewer")

    matrix = spec.get("x-namros-operation-matrix")
    if not isinstance(matrix, list) or not matrix:
        fail("x-namros-operation-matrix must be a non-empty array")

    operation_ids = [entry.get("operationId") for entry in matrix]
    invalid_ids = [operation_id for operation_id in operation_ids if not isinstance(operation_id, str) or not operation_id]
    if invalid_ids:
        fail("every matrix entry must have a non-empty operationId")
    duplicates = sorted(name for name, count in Counter(operation_ids).items() if count != 1)
    if duplicates:
        fail(f"duplicate operationId values: {', '.join(duplicates)}")

    for entry in matrix:
        missing = [field for field in ("method", "resource", "selector", "status", "notes") if not entry.get(field)]
        if missing:
            fail(f"{entry['operationId']} is missing fields: {', '.join(missing)}")
        if entry["status"] not in ALLOWED_STATUSES:
            fail(f"{entry['operationId']} has unknown status {entry['status']!r}")

    router_source = ROUTER.read_text(encoding="utf-8")
    expected_routes, declarations = routing_operations(router_source)
    route_mappings: list[str] = []
    for entry in matrix:
        operation_id = entry["operationId"]
        routing_operation = entry.get("routingOperation", operation_id)
        expected_alias = ALLOWED_ROUTING_ALIASES.get(operation_id)
        if expected_alias is not None and routing_operation != expected_alias:
            fail(f"{operation_id} must map to routing operation {expected_alias}")
        if expected_alias is None and "routingOperation" in entry:
            fail(f"{operation_id} declares an unapproved routingOperation alias")
        route_mappings.append(routing_operation)

    published_routes = set(route_mappings)
    if published_routes != expected_routes:
        missing = sorted(expected_routes - published_routes)
        extra = sorted(published_routes - expected_routes)
        fail(f"router/catalog drift; missing={missing}, extra={extra}")
    expected_route_counts = Counter(
        [operation_id for operation_id in operation_ids if operation_id in expected_routes]
        + list(ALLOWED_ROUTING_ALIASES.values())
    )
    if Counter(route_mappings) != expected_route_counts:
        fail(
            "router/catalog alias count drift; "
            f"actual={dict(Counter(route_mappings))}, expected={dict(expected_route_counts)}"
        )

    handler_source = HANDLER.read_text(encoding="utf-8")
    dispatch_switch = braced_block(handler_source, "switch req.Operation")
    dispatch_names: list[str] = []
    for case_clause in re.findall(r"\bcase\s+([^:]+):", dispatch_switch):
        dispatch_names.extend(re.findall(r"routing\.(Operation[A-Za-z0-9]+)", case_clause))
    dispatch_counts = Counter(dispatch_names)
    expected_names = set(declarations)
    missing_handlers = sorted(expected_names - set(dispatch_names))
    extra_handlers = sorted(set(dispatch_names) - expected_names)
    duplicate_handlers = sorted(name for name, count in dispatch_counts.items() if count != 1)
    if missing_handlers or extra_handlers or duplicate_handlers:
        fail(
            "gateway dispatch drift; "
            f"missing={missing_handlers}, extra={extra_handlers}, duplicates={duplicate_handlers}"
        )

    upload_part_case = re.search(
        r"case routing\.OperationUploadPart:(.*?)(?=\n\tcase |\n\tdefault:)",
        dispatch_switch,
        re.DOTALL,
    )
    if upload_part_case is None or not all(
        token in upload_part_case.group(1)
        for token in ('c.GetHeader("x-amz-copy-source")', "h.uploadPartCopy", "h.uploadPart")
    ):
        fail("UploadPart gateway dispatch must preserve the UploadPartCopy header branch")

    dispatched: list[str] = []
    dispatch_locations: dict[str, list[tuple[str, str]]] = {}
    for path, path_item in spec.get("paths", {}).items():
        if "?" in path:
            fail(f"query string must not be embedded in OpenAPI path: {path}")
        if not isinstance(path_item, dict):
            continue
        for method, operation in path_item.items():
            if isinstance(operation, dict):
                logical_operations = operation.get("x-namros-dispatches", [])
                dispatched.extend(logical_operations)
                for operation_id in logical_operations:
                    dispatch_locations.setdefault(operation_id, []).append((method.upper(), path))
    dispatch_counts = Counter(dispatched)
    missing_dispatch = sorted(set(operation_ids) - set(dispatched))
    extra_dispatch = sorted(set(dispatched) - set(operation_ids))
    duplicate_dispatch = sorted(name for name, count in dispatch_counts.items() if count != 1)
    if missing_dispatch or extra_dispatch or duplicate_dispatch:
        fail(
            "OpenAPI dispatcher drift; "
            f"missing={missing_dispatch}, extra={extra_dispatch}, duplicates={duplicate_dispatch}"
        )
    misplaced_dispatch = sorted(
        entry["operationId"]
        for entry in matrix
        if dispatch_locations.get(entry["operationId"])
        != [(entry["method"].upper(), entry["resource"])]
    )
    if misplaced_dispatch:
        fail(f"OpenAPI path/method dispatch drift: {', '.join(misplaced_dispatch)}")

    catalog_statuses = {entry["operationId"]: entry["status"] for entry in matrix}
    for reference, status_labels in REFERENCES.items():
        try:
            text = reference.read_text(encoding="utf-8")
        except FileNotFoundError:
            fail(f"missing reference document: {reference.relative_to(ROOT)}")
        documented_statuses: dict[str, str] = {}
        documented_rows: list[str] = []
        for line in text.splitlines():
            columns = [column.strip() for column in line.split("|")]
            if len(columns) < 5 or not columns[1].startswith("`") or not columns[1].endswith("`"):
                continue
            operation_id = columns[1][1:-1]
            status = status_labels.get(columns[3])
            if status is not None:
                documented_statuses[operation_id] = status
                documented_rows.append(operation_id)

        missing_from_doc = sorted(set(operation_ids) - set(documented_statuses))
        extra_in_doc = sorted(set(documented_statuses) - set(operation_ids))
        duplicate_rows = sorted(
            operation_id
            for operation_id, count in Counter(documented_rows).items()
            if count != 1
        )
        if missing_from_doc or extra_in_doc or duplicate_rows:
            fail(
                f"{reference.relative_to(ROOT)} table drift; "
                f"missing={missing_from_doc}, extra={extra_in_doc}, duplicates={duplicate_rows}"
            )
        status_drift = sorted(
            operation_id
            for operation_id, status in catalog_statuses.items()
            if documented_statuses[operation_id] != status
        )
        if status_drift:
            fail(
                f"{reference.relative_to(ROOT)} has catalog status drift: "
                + ", ".join(status_drift)
            )

    print(
        "[s3-api-spec-check] passed: "
        f"{len(matrix)} logical operations, {len(expected_routes)} router operations, "
        f"{len(spec.get('paths', {}))} OpenAPI paths"
    )


if __name__ == "__main__":
    main()
