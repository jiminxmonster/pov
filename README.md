# 전지적관람시점 Web App

공연·전시 정보를 자연어로 검색하고 지도와 중앙 정렬 전시 목록으로 탐색하는 모바일 우선 웹앱이다.

## 구성

- 프론트엔드: Nuxt 3, TypeScript, Leaflet
- 백엔드: Go, chi, pgx
- 데이터베이스: PostgreSQL (PostGIS 이미지 사용)
- 프록시·TLS: Caddy
- 실행·배포: Docker Compose
- 제품 기획서: [PROJECT_PLAN.md](./PROJECT_PLAN.md)

## 주요 기능

- 흰 화면과 중앙 정렬 POV 검색
- 인덱스 검색과 자연어 조건 검색
- `mmarrk.svg` 핀과 노란 정보표시로 전시 위치를 보여주는 반응형 지도 뷰
- 서울 열린데이터광장 문화행사 API의 현재 전시를 매일 자동 동기화
- 중앙 정렬된 반응형 전시 목록
- 전시 선택 후 항목별 구분선이 있는 상세 보기
- `+` 버튼을 통한 익명 사용자 전시·사진 제보와 관리자 검토
- 로고를 1.5초 안에 3회 누르는 숨은 관리자 진입
- 관리자 세션 인증
- 고정 텍스트가 들어 있는 단일 본문 편집기
- 대표 이미지 업로드
- TXT, Markdown, CSV, Excel, Word, PDF 자료 업로드와 양식 초안 생성
- 본문 메타데이터 추출과 장소 정보 정리

## 로컬 실행

```bash
cp .env.example .env
docker compose up --build
```

브라우저에서 `http://localhost`를 연다. `.env.example`의 기본 문구형 비밀번호는 실제 값으로 변경해야 한다.

공공 전시 데이터는 기본값 `SEOUL_OPEN_DATA_KEY=sample`로 최신 5건을 불러온다. 서울 열린데이터광장에서 발급한 키와 원하는 `SEOUL_OPEN_DATA_LIMIT`를 `.env`에 넣으면 최대 1,000건까지 동기화하며, 종료된 전시는 자동으로 보관 처리한다.

개별 개발 서버는 다음과 같이 실행할 수 있다.

```bash
cd backend
go test ./...

cd ../frontend
npm install
npm run dev
```

## 관리자 진입

1. 첫 화면의 POV 로고를 1.5초 안에 3번 누른다.
2. 운영 기본 계정 `admin` / `admin`으로 로그인한다.
3. 하나의 본문 양식에 내용을 채우거나 자료 파일을 불러온다.
4. 대표 사진을 선택하고 초안 저장 또는 게시하기를 누른다.

운영자 화면 상단의 공공 전시 데이터 영역에서 서울 열린데이터광장 인증키와 수집 건수를 저장하거나 즉시 동기화할 수 있다. 인증키는 세션 비밀키로 암호화해 데이터베이스에 저장하며 화면에는 마스킹된 값만 표시한다.

로고 3회 동작은 진입 주소를 감추는 UI일 뿐 보안 수단이 아니다. 관리자 API는 서명된 HttpOnly 세션 쿠키로 별도 보호한다.

## 검증

```bash
make test
npm --prefix frontend run build
docker compose config --quiet
```

## Vultr 배포

Ubuntu 기반 Vultr 인스턴스에 Docker Engine, Compose 플러그인, Git을 설치한 뒤 실행한다.

```bash
sudo APP_DIR=/opt/pov sh /opt/pov/infra/deploy.sh
```

최초 실행은 `/opt/pov/.env`를 만들고 중단한다. 다음 값을 안전한 운영 값으로 변경한 뒤 스크립트를 다시 실행한다.

- `POSTGRES_PASSWORD`
- `ADMIN_PASSWORD`
- `SESSION_SECRET`
- `PUBLIC_ORIGIN`
- `SITE_ADDRESS`

`www.d2blue.com/pov` 운영값은 `.env.vultr.example`에 준비되어 있다. Compose의 Caddy는 `127.0.0.1:18080`에서 앱을 열고, 도메인의 기존 프록시가 `/pov` 경로를 이 주소로 전달하는 구성이다. 이때 요청 URI의 `/pov` 접두사는 유지해야 한다.

Nginx를 쓰는 기존 서버라면 HTTPS 서버 블록에서 `infra/nginx/pov-location.conf`를 include 한다.

```nginx
include /opt/pov/infra/nginx/pov-location.conf;
```
