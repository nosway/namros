Community 운영

# S3 오브젝트 브라우저 연동 가이드

<div class="summary" markdown="1">

NAMROS는 운영 콘솔에 Object Explorer Lite를 제공하고, 전체 파일 관리 워크플로는 외부 S3 브라우저 도구 연동으로 해결합니다. 이렇게 하면 콘솔을 product operations plane 경계 안에 유지하면서도 Community 배포를 쉽게 점검하고 테스트할 수 있습니다.

</div>

## 권장 제품 방향

내장 콘솔은 두 번째 S3 클라이언트 구현이나 직접 메타데이터 편집기가 되어서는 안 됩니다. Community 범위는 버킷, prefix, 오브젝트, 버전, 태그, 보존, 암호화 상태를 읽기 전용으로 보여주는 Object Explorer Lite입니다. 업로드, 재귀 복사/이동, 대량 삭제, 인라인 편집, 일반 다운로드 워크플로는 검증된 외부 S3 호환 도구를 사용합니다.

단일 오브젝트 다운로드와 삭제는 이후 단계에서만 검토합니다. 이때도 게이트웨이 경로, RBAC, CSRF, plan/preflight/apply/verify/audit, Object Lock, 보존 정책, 감사 제어를 모두 사용해야 합니다.

## NAMROS Object Explorer Lite

| 기능 | Community 동작 | 경계 |
| --- | --- | --- |
| 버킷 목록 | 버킷과 운영 상태 요약을 표시합니다. | 읽기 전용 목록. |
| Prefix/오브젝트 목록 | prefix, delimiter, pagination, version-aware 목록을 지원합니다. | 페이로드 바이트 없음. |
| 오브젝트 상세 | HEAD metadata, tag, ETag, size, content type, version id, delete marker, retention, lock, encryption posture를 표시합니다. | private metadata 직접 해석 없음. |
| 다운로드/삭제/업로드 | 기본 비활성화. | 후속 승인 작업 정책 필요. |
| 메트릭 | raw object key 또는 임의 prefix를 metric label로 노출하지 않습니다. | Prefix 분석은 report job 사용. |

## 외부 도구 연동 매트릭스

| 도구 | 권장 용도 | NAMROS 정책 | 참고 |
| --- | --- | --- | --- |
| AWS CLI | 기본 S3 호환성, 자동화, 스모크 테스트. | 1급 호환성 대상. | [S3 클라이언트 가이드](s3-client-compatibility-guide.md) |
| MinIO client (`mc`) | 운영자 CLI browsing, copy, stat, mirror, 확장 S3 workflow. | 1급 호환성 대상. | [mc 스모크](s3-client-compatibility-guide.md#mc) |
| rclone | 마이그레이션, 동기화, 스크립트 기반 복사/삭제. | 1급 호환성 대상. | [rclone S3 backend](https://rclone.org/s3/) |
| Cyberduck | 운영자와 테스터용 데스크톱 GUI 오브젝트 브라우징. | 호환 도구로 문서화, 번들하지 않음. | [Cyberduck docs](https://docs.cyberduck.io/) |
| Brows3 | 데스크톱 S3 브라우저 후보. | NAMROS 호환성 검증 후 선택 가이드에 포함. | [Brows3](https://www.brows3.app/) |
| Filestash | Self-hosted web file manager. | 선택 연동만 문서화, 기본 번들 제외. | [Filestash S3 browser](https://www.filestash.app/s3-browser.html) |
| MinIO Console | 오브젝트 브라우저와 운영 UX benchmark. | Benchmark 전용, NAMROS dependency 아님. | [MinIO Console docs](https://minio.community/community/minio-object-store/administration/minio-console.html) |

## 보안 원칙

- Root credential, access key secret, session token, presigned URL을 HTML, 로그, support bundle, inspect 가능한 컨테이너 환경에 넣지 않습니다.
- 외부 도구에는 임시 또는 최소 권한 S3 credential을 우선 사용합니다.
- Object key와 metadata는 신뢰하지 않는 문자열로 취급합니다. HTML escape를 적용하고 복사된 값이 실행되지 않게 합니다.
- Multi-tenant 또는 공유 배포에서는 콘솔 가시성에 bucket/prefix allowlist를 적용합니다.
- RBAC, 명시적 승인, Object Lock/retention check, audit persistence가 완료되기 전에는 파괴적 오브젝트 작업을 비활성화합니다.

## 검증 체크리스트

1. 대상 endpoint에 대해 S3 클라이언트 스모크 명령을 실행합니다.
2. S3 클라이언트 가이드 기준으로 AWS CLI, MinIO client, rclone이 list, put, head, get, copy, move/delete, multipart copy를 수행하는지 확인합니다.
3. Cyberduck과 GUI 후보는 release 지원 도구로 문서화하기 전에 path-style endpoint 설정으로 검증합니다.
4. Object Explorer Lite 응답에 payload bytes, secret value, presigned URL이 없는지 확인합니다.
5. Object key와 prefix 값이 Prometheus label과 high-cardinality metric에 포함되지 않는지 확인합니다.

## 구현 단계

| 단계 | 범위 | 완료 조건 |
| --- | --- | --- |
| Phase 1 | 외부 도구와 redacted client recipe를 문서화합니다. | 호환성 가이드와 HTML 문서 검사 통과. |
| Phase 2 | 읽기 전용 Object Explorer Lite API와 콘솔 패널을 추가합니다. | List/head unit test로 payload/secret 누출 없음 확인. |
| Phase 3 | 선택적 단일 오브젝트 다운로드를 평가합니다. | Gateway-mediated path, RBAC, audit, policy control 완료. |
| Phase 4 | 승인된 단일 오브젝트 삭제/version delete를 평가합니다. | Plan/preflight/apply/verify/audit와 retention check 완료. |
