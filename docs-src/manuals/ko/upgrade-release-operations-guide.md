릴리스 운영 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS 업그레이드/릴리스 운영 가이드

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition 릴리스 게이트와 Enterprise edition only private distribution 패키징 안내를 함께 다룹니다. Enterprise release gate와 폐쇄망 private enterprise packaging은 공개 Community 제공 항목이 아닙니다.

</div>

이 문서는 NAMROS 클러스터의 릴리스 준비 상태 검증, 서비스 영향을 최소화하는 무중단 업그레이드, 롤백, 핫픽스, 폐쇄망 및 사설 Enterprise 환경 배포 절차를 설명하는 운영 라이프사이클 가이드입니다.

## 릴리스 게이트 체크리스트

```sh
make test
make test-community
make html-docs-check
make check-publication-readiness
make production-scale-check
make export-community
make container-local-smoke
```

<span class="badge enterprise">Enterprise edition only</span> 릴리스 게이트는 private distribution이 해당 기능을 포함하는 경우 EC, 중복 제거, WORM, KMS, 컴플라이언스, IAM, chaos/soak 프로파일을 추가합니다.

## 업그레이드 흐름

1. 메타데이터를 내보내고 릴리스 준비 상태 리포트를 캡처합니다.
2. 에디션 식별자와 기능 권한 카탈로그를 검증합니다.
3. 게이트웨이 하나 또는 canary 환경을 먼저 업그레이드합니다.
4. 전체 배포 전에 호환성 및 메타데이터 스모크를 실행합니다.
5. 업그레이드 후 리포트와 작업 감사를 기록합니다.

## 롤백 요구 사항

| 영역 | 요구 사항 |
| --- | --- |
| 메타데이터 스키마 | 상위/하위 버전 호환성 또는 명시적 마이그레이션 경계. |
| 바이너리 | 이전 산출물과 설정 보존. |
| 리포트 | 업그레이드 전/후 증빙 보존. |

## 폐쇄망 및 사설 릴리스

Community 소스 내보내기와 사설 Enterprise overlay 조립은 별도의 릴리스 트랙입니다. 폐쇄망 릴리스에는 산출물 체크섬, 의존성 매니페스트, 오프라인 클라이언트 도구 참조, 설치 후 스모크 산출물이 필요합니다.

Community 소스 내보내기는 `scripts/release/write-release-artifact-metadata.sh`를 통해 release metadata를 기록합니다. metadata 디렉터리에는 `release-metadata.json`, `checksums.sha256`, `provenance.json`, `go-modules.txt`, `sbom-status.json`이 포함됩니다. `check-publication-readiness`는 metadata tooling을 검증하고 public export에서 사설 planning note, lab hostname, Enterprise 구현 참조를 차단합니다.

참고: [릴리스와 에디션 경계](../architecture-manual/chapters/14-release-and-edition-boundaries.md).
