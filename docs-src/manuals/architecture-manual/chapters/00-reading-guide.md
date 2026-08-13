Chapter 00 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# Reading Guide

## Reader Paths

- S3 users: 01, 02, 06, 09
- Operators: 03, 04, 12, 13
- Storage reviewers: 05, 07, 08, 10
- Release reviewers: 11, 14

<div class="note" markdown="1">

**Edition scope.** This chapter defines how Community edition and Enterprise edition only markers are used throughout the manual. Use those markers as the source of truth when a page mixes public behavior with private-distribution feature descriptions.

</div>

This manual is organized around ownership boundaries. NAMROS is easier to review when each state transition is tied to the component that owns it and the backend where it becomes authoritative.

## Personas

| Reader | Primary Question | Chapters |
| --- | --- | --- |
| S3 application user | Which S3 behaviors can normal clients rely on? | 01, 02, 06, 09 |
| Platform operator | How do I run, observe, recover, and validate NAMROS? | 03, 04, 12, 13 |
| Architecture reviewer | Where is state authoritative and which component owns mutation? | 03, 04, 05, 06 |
| Enterprise reviewer | Which features are private distribution capabilities? | 08, 10, 11, 14 |

## Edition Markers

<span class="badge">Community</span> marks public source behavior. <span class="badge enterprise">Enterprise edition only</span> marks private distribution features. Community builds must not provide a flag, environment variable, or public build tag that turns them into Enterprise builds.

## How To Read The Object Path

The most important reading path is 04 -> 05 -> 07. Chapter 04 explains the metadata authority. Chapter 05 explains how an S3 bucket/key becomes an object version manifest. Chapter 07 explains how that manifest reaches local or SBS replicated bytes. Chapter 08 adds the Enterprise EC version of that same storage contract.

## Developer Source Map

| Question | Start In This Chapter | Then Inspect |
| --- | --- | --- |
| How is object visibility decided? | 04, 05, 06 | `internal/meta/repository.go`, `internal/meta/model/model.go` |
| How does an object map to SBS bytes? | 05, 07, 08 | `internal/storage/storage.go`, `internal/storage/sbs`, `internal/storage/classroute` |
| How do gateway failovers remain safe? | 03, 04, 12 | `internal/gateway`, `internal/coordination` |
| How are deletes protected? | 09, 10, 11 | `internal/meta`, `internal/gc`, `internal/dedupe` |
| Where is the public/private boundary? | 14 | `internal/edition`, release/source export checks |

## Operational Cross-links

| Topic | HTML Guide | Public Surface |
| --- | --- | --- |
| client compatibility | [S3 client compatibility](../../s3-client-compatibility-guide.md) | S3 API and external client smoke guidance |
| gateway coordination | [etcd HA guide](../../etcd-ha-cluster-install-operations-guide.md) | etcd registry and active-active gateway behavior |
| metadata backup/restore | [admin guide](../../admin-guide.md) | metadata export/import operation model |
| MCP operations | [MCP guide](../../mcp-operations-guide.md) | observe-first operations provider |
