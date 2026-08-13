검증

# NAMROS 성능/Chaos/Soak 가이드

<div class="summary" markdown="1">

M22는 단발 스모크가 아니라 다중 노드 환경에서 성능, 안정성, 장애 전환, 복구, 데이터 정합성 증빙을 반복 수집하는 마일스톤입니다.

</div>

## 계층형 벤치마크 모델

| 계층 | 신호 |
| --- | --- |
| 네트워크 | 처리량, 지연 시간, 패킷 손실, 재시도 급증. |
| 스토리지/SBS | 세그먼트 쓰기/읽기 지연, 샤드 오류, healing backlog. |
| 메타데이터/TiKV | 트랜잭션 지연, 경합, 재시도 횟수. |
| 게이트웨이 | 요청 지연 백분위, first-byte 지연, PUT/UploadPart body-read 지연, 제한된 S3 error code counter, 캐시 hit/miss, 인증/서명 오류. |
| S3 클라이언트 | AWS CLI/mc/rclone/s3fs 워크로드 성공과 digest 검증. |

## Soak 워크로드

1. 버전 관리와 다양한 오브젝트 크기를 가진 버킷을 생성합니다.
2. PUT/GET/HEAD/Range/List/Copy/Delete 및 멀티파트 작업을 실행합니다.
3. Noisy-tenant throttle profile을 실행해 throttle된 tenant가 neighboring tenant를 굶기지 않는지 증명합니다.
4. 권한이 있는 환경에서는 선택적 라이프사이클, KMS, 중복 제거, EC 프로파일을 포함합니다.
5. 각 단계 이후 게이트웨이 전반의 오브젝트 digest를 검증합니다.

## Chaos 시나리오

| 시나리오 | 기대 동작 |
| --- | --- |
| 게이트웨이 종료/재시작 | etcd 레지스트리가 실패한 게이트웨이를 제거하고 남은 게이트웨이가 커밋된 네임스페이스 상태를 제공합니다. |
| etcd 멤버 손실 | 레지스트리는 etcd 상태에 따라 저하되며, 오브젝트 메타데이터 정본은 TiKV에 남습니다. |
| TiKV 일시 장애 | 게이트웨이는 예측 가능한 방식으로 요청을 실패시키고, 안전한 경우 재시도하며, 커밋된 오브젝트 손실 없이 복구합니다. |
| SBS data 재시작 | 쓰기는 안전하게 실패하거나 재시도되고, 읽기는 복구 후 digest를 검증합니다. |

## 리포트 산출물

```text
topology:
workload:
duration:
chaos timeline:
metrics summary:
digest verification:
failures:
retained logs:
reproduction commands:
```

MCP 도구 `namros.multi_node.soak.run`과 `namros.chaos_soak.latest`는 이 리포트 형식을 읽고 요약해야 합니다. [MCP 운영 가이드](mcp-operations-guide.md)를 참고하세요.
