package app

import (
	"strings"
	"testing"
	"time"
)

func TestParseTemplate(t *testing.T) {
	body := `전시명:
여름의 표면

관람료:
무료

장소:
서울 성동구 성수동

전시내용:
첫 번째 문장
두 번째 문장`

	metadata, title, address, latitude, longitude := parseTemplate(body)
	if title != "여름의 표면" {
		t.Fatalf("unexpected title: %q", title)
	}
	if address != "서울 성동구 성수동" {
		t.Fatalf("unexpected address: %q", address)
	}
	if latitude != 37.5445 || longitude != 127.0560 {
		t.Fatalf("unexpected coordinates: %f, %f", latitude, longitude)
	}
	if !strings.Contains(metadata["전시내용"], "두 번째 문장") {
		t.Fatalf("multiline body was not preserved: %q", metadata["전시내용"])
	}
}

func TestNormalizeToTemplate(t *testing.T) {
	body := normalizeToTemplate("낯선 전시\n메모 원문", "memo.txt")
	if !strings.Contains(body, "전시명:\n낯선 전시") {
		t.Fatalf("title was not inferred: %q", body)
	}
	for _, label := range templateLabels() {
		if !strings.Contains(body, label+":") {
			t.Fatalf("missing label %q", label)
		}
	}
}

func TestNormalizeDoesNotMistakeEmbeddedLabelForTemplate(t *testing.T) {
	body := normalizeToTemplate("기획 문서\n중간 예시: 전시명:\n설명", "plan.md")
	if !strings.HasPrefix(body, "전시명:\n기획 문서") {
		t.Fatalf("ordinary document was mistaken for a completed template: %q", body)
	}
}

func TestSearchTermsPreferStructuredConditions(t *testing.T) {
	terms := searchTerms("이번 주말에 성수에서 무료 도슨트 전시")
	want := []string{"무료", "도슨트", "성수"}
	if strings.Join(terms, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected search terms: %#v", terms)
	}
}

func TestSessionSignature(t *testing.T) {
	server := Server{config: Config{
		AdminUsername: "admin",
		SessionSecret: "a-long-enough-test-secret",
	}}
	value := server.createSession("admin", time.Now().Add(time.Hour))
	if !server.validSession(value) {
		t.Fatal("expected signed session to validate")
	}
	if server.validSession(value + "tampered") {
		t.Fatal("tampered session must not validate")
	}
}

func TestBasePathHelpers(t *testing.T) {
	if got := normalizeBasePath("pov/"); got != "/pov" {
		t.Fatalf("unexpected normalized base path: %q", got)
	}
	if got := prefixedPath("/pov", "/uploads/poster.png"); got != "/pov/uploads/poster.png" {
		t.Fatalf("unexpected prefixed path: %q", got)
	}
	if got := prefixedPath("/", "/uploads/poster.png"); got != "/uploads/poster.png" {
		t.Fatalf("unexpected root path: %q", got)
	}
}

func TestConvertSeoulEvent(t *testing.T) {
	now := time.Date(2026, time.July, 18, 14, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	event := seoulCulturalEvent{
		CodeName:  "전시/미술",
		GuName:    "중구",
		Title:     "공공데이터 전시",
		Date:      "2026-07-18~2026-08-31",
		Place:     "서울시립미술관",
		Organizer: "서울시립미술관",
		Audience:  "누구나",
		Fee:       "무료",
		Inquiry:   "02-0000-0000",
		StartDate: "2026-07-18 00:00:00.0",
		EndDate:   "2026-08-31 00:00:00.0",
		Longitude: "126.9737",
		Latitude:  "37.5640",
		Homepage:  "https://culture.seoul.go.kr/culture/culture/cultureEvent/view.do?cultcode=158601",
		ImageURL:  "https://example.com/poster.jpg",
	}

	exhibition, ok := convertSeoulEvent(event, now)
	if !ok {
		t.Fatal("expected current exhibition to be converted")
	}
	if exhibition.Slug != "seoul-culture-158601" {
		t.Fatalf("unexpected slug: %q", exhibition.Slug)
	}
	if exhibition.Address != "서울 중구 · 서울시립미술관" {
		t.Fatalf("unexpected address: %q", exhibition.Address)
	}
	if exhibition.Latitude != 37.5640 || exhibition.Longitude != 126.9737 {
		t.Fatalf("unexpected coordinates: %f, %f", exhibition.Latitude, exhibition.Longitude)
	}
	if !strings.Contains(exhibition.BodyMarkdown, "공공데이터 출처: "+seoulOpenDataSource) {
		t.Fatalf("source attribution is missing: %q", exhibition.BodyMarkdown)
	}

	event.EndDate = "2026-07-17 00:00:00.0"
	if _, ok := convertSeoulEvent(event, now); ok {
		t.Fatal("ended exhibition must be ignored")
	}
}
