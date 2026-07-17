# 전지적관람시점 Web App

공연·전시 정보를 자연어로 검색하고 지도 위 포스트잇 핀으로 탐색하는 모바일 우선 웹앱이다.

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
- 반응형 지도와 포스트잇 형태 공연·전시 핀
- 핀 선택 후 게시글 상세 보기
- 로고를 1.5초 안에 3회 누르는 숨은 관리자 진입
- 관리자 세션 인증
- 고정 텍스트가 들어 있는 단일 본문 편집기
- 대표 이미지 업로드
- TXT, Markdown, CSV, Excel, Word, PDF 자료 업로드와 양식 초안 생성
- 본문 메타데이터 추출과 지역 기반 지도 좌표 후보 생성

## 로컬 실행

```bash
cp .env.example .env
docker compose up --build
```

브라우저에서 `http://localhost`를 연다. `.env.example`의 기본 문구형 비밀번호는 실제 값으로 변경해야 한다.

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
2. `.env`의 `ADMIN_USERNAME`, `ADMIN_PASSWORD`로 로그인한다.
3. 하나의 본문 양식에 내용을 채우거나 자료 파일을 불러온다.
4. 대표 사진을 선택하고 초안 저장 또는 게시하기를 누른다.

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

IP로 먼저 공개할 때는 `SITE_ADDRESS=:80`, `PUBLIC_ORIGIN=http://서버IP`를 사용한다. 도메인 연결 후 `SITE_ADDRESS=pov.example.com`, `PUBLIC_ORIGIN=https://pov.example.com`으로 바꾸면 Caddy가 HTTPS 인증서를 자동 관리한다. 도메인 사용 시 Vultr 방화벽과 서버 방화벽에서 TCP 80·443 및 UDP 443을 허용한다.
