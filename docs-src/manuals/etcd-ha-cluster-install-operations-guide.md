Community Production Operations

# etcd HA Installation & Operations Guide

## Role

<span class="badge">Community</span> In NAMROS, etcd manages the gateway registry and health leases. It is not the authoritative store for object metadata or payloads, but rather a coordination backend representing the availability and control-plane health of the gateway fleet.

## Lab And HA Topologies

| Topology | Members | Use |
| --- | --- | --- |
| single-node lab | 1 | local smoke, lease expiry behavior |
| 3-node HA | 3 | normal production-like quorum |
| 5-node HA | 5 | larger failure tolerance with higher write quorum cost |

Production deployments should use TLS, authentication, durable storage, and explicit member names. This guide documents the NAMROS integration points; cluster hardening remains an operator responsibility.

## Local Lab Ports

```sh
etcd --name namros-local \
  --data-dir .namros/etcd \
  --listen-client-urls http://127.0.0.1:12379 \
  --advertise-client-urls http://127.0.0.1:12379 \
  --listen-peer-urls http://127.0.0.1:12380 \
  --initial-advertise-peer-urls http://127.0.0.1:12380 \
  --initial-cluster namros-local=http://127.0.0.1:12380 \
  --initial-cluster-state new
```

macOS default ports `2379/2380` are often already in use by local TiKV/PD or previous etcd runs. The documented local smoke path uses `12379/12380` to avoid the common conflict.

## Gateway Integration

All gateways in the same group must use the same endpoint set and registry root. Gateway ids must be unique.

```sh
NAMROS_ETCD_ENDPOINTS=127.0.0.1:12379 \
NAMROS_GATEWAY_REGISTRY_PREFIX=/namros/gateways \
make smoke-etcd-registry
```

The smoke checks registry key creation, heartbeat refresh, and lease expiry removal after the gateway is killed without revoke.

## Operations

| Task | Command/Check | Expected Result |
| --- | --- | --- |
| member health | `etcdctl endpoint health` | all endpoints healthy |
| registry list | `etcdctl get --prefix /namros/gateways` | gateway keys with fresh leases |
| backup | `etcdctl snapshot save` | snapshot artifact stored off-node |
| member replacement | remove failed member, add replacement, verify quorum | cluster returns to healthy quorum |

## Failure Modes

| Signal | Likely Cause | Response |
| --- | --- | --- |
| bind error on peer port | port already used | choose alternate client/peer ports and data-dir |
| registry key missing | gateway not started, wrong root, lease expired | verify gateway config and endpoint set |
| stale gateway key | lease refresh path broken | check heartbeat logs and lease TTL |
| quorum lost | too many member failures | restore quorum before relying on active-active health |
