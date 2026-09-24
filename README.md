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
- 서울 열린데이터광장 문화행사 API, 문화공공데이터광장 통합 전시 API, 한국관광공사 국문 관광정보 API의 전시회 분류를 매일 자동 동기화
- 중앙 정렬된 반응형 전시 목록
- 전시 선택 후 항목별 구분선이 있는 상세 보기
- `+` 버튼을 통한 익명 사용자 전시·사진 제보와 관리자 검토
- 로고를 1.5초 안에 3회 누르는 숨은 관리자 진입
- 관리자 세션 인증
- 고정 텍스트가 들어 있는 단일 본문 편집기
- 대표 이미지 업로드
- 편집기 커서 위치에 본문 이미지 업로드·삽입
- TXT, Markdown, CSV, Excel, Word, PDF 자료 업로드와 양식 초안 생성
- 본문 메타데이터 추출과 장소 정보 정리
- OpenAI Responses API 기반 자연어 전시 큐레이션과 추천 이유 생성
- 질문의 명확도에 따라 지도 추천, 한 문장씩 좁히는 위저드, 정보·링크 대화로 이어지는 AI 응답 분기
- 중앙형 POV AI 대화 화면과 `main · map · list` 빠른 이동

## 로컬 실행

```bash
cp .env.example .env
docker compose up --build
```

브라우저에서 `http://localhost`를 연다. `.env.example`의 기본 문구형 비밀번호는 실제 값으로 변경해야 한다.

공공 전시 데이터는 공급자별로 기본 1,000건까지 동기화하고, 공개 목록·지도에는 최대 3,000건까지 보여준다. 서울 열린데이터광장의 `sample` 키는 제공처 제한에 따라 5건으로 제한된다. 문화공공데이터광장의 `한국문화정보원 외_전시정보(통합)` API는 제목·기간·장소·작가·관람료·대표이미지 등 24개 표준 항목을 목록·상세·AI 검색 지식에 반영한다. 이 API에는 위도·경도가 없으므로 좌표가 확인되지 않은 자료는 지도에 임의로 표시하지 않는다. 한국관광공사 `국문 관광정보 서비스_GW`의 `searchFestival2` 전시회 분류는 전국 단위의 기간·주소·좌표·대표이미지를 보완한다. 공공데이터포털에서 해당 API의 활용신청과 서비스키 발급이 필요하며, API 이미지는 공급자의 저작권 유형과 출처표시 조건을 지켜 사용해야 한다. 종료 후 1개월 이내 전시는 공개 목록의 마지막에 만료 상태로 구분하고 지도에서는 제외한다. 더 오래된 기록은 공개하지 않고 AI의 과거 전시 지식으로만 유지한다.

공급원 선정 기준은 최신 일정, 전국 범위, 원문 추적 가능성, 좌표/이미지 품질, API 이용조건이다. 전시 중심인 문화공공데이터광장과 한국관광공사를 우선 사용하고, 서울시는 지역 보완에 쓴다. [KOPIS](https://www.kopis.or.kr/por/cs/openapi/openApiList.do?menuId=MNU_00074&tabId=tab3_2)는 공연 중심이므로 전시 데이터에 섞지 않고 공연 기능을 넓힐 때 별도 검토한다. 박물관·미술관 시설 목록은 개별 전시 일정이 아니므로 전시 게시글로 자동 등록하지 않는다.

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

운영자 화면 상단의 공급자별 공공 전시 데이터 영역에서 서울 열린데이터광장, 문화공공데이터광장, 한국관광공사 서비스키와 수집 건수를 각각 저장하거나 즉시 동기화할 수 있다. 인증키는 세션 비밀키와 공급자별 암호화 문맥으로 데이터베이스에 저장하며 화면에는 마스킹된 값만 표시한다. 관광공사 키는 [국문 관광정보 서비스_GW](https://www.data.go.kr/data/15101578/openapi.do)에 활용신청한 뒤 입력한다.

OpenAI 전시 큐레이터 영역에서는 OpenAI API 키와 모델을 저장하고 연결 상태를 검사할 수 있다. 기본 모델은 `gpt-5.6-luna`다. 키는 서버 데이터베이스에 암호화 저장되고, 공개 검색은 등록된 전시 데이터만 후보로 전달해 지도 추천·역질문·채팅 답변을 만든다. OpenAI 응답은 JSON Schema로 구조화하며 `store: false`로 요청한다. 외부 AI가 응답하지 않으면 기존 인덱스 검색으로 자동 전환한다. 기존 NVIDIA 키는 OpenAI 키로 자동 전환되지 않으므로 관리자 화면에서 새 키를 저장해야 한다. OpenAI API 사용량은 별도로 과금된다.

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
