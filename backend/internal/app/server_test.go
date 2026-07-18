package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestEncryptedSettingRoundTrip(t *testing.T) {
	server := Server{config: Config{SessionSecret: "test-session-secret-that-is-long-enough"}}
	plaintext := []byte(`{"api_key":"secret-key-1234","limit":100}`)
	encrypted, err := server.sealSetting(plaintext)
	if err != nil {
		t.Fatalf("seal setting: %v", err)
	}
	if strings.Contains(string(encrypted), "secret-key-1234") {
		t.Fatal("encrypted setting contains plaintext key")
	}
	decrypted, err := server.openSetting(encrypted)
	if err != nil {
		t.Fatalf("open setting: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("unexpected decrypted value: %q", decrypted)
	}
}

func TestPublicDataSettingsHelpers(t *testing.T) {
	settings := normalizePublicDataSettings(publicDataSettings{APIKey: "sample", Limit: 500})
	if settings.Limit != 5 {
		t.Fatalf("sample key must be capped at 5, got %d", settings.Limit)
	}
	if got := maskSecret("1234567890abcdef"); got != "1234••••••cdef" {
		t.Fatalf("unexpected masked key: %q", got)
	}
	if !validPublicDataKey("valid_key-1234") || validPublicDataKey("invalid/key") {
		t.Fatal("public data key validation is incorrect")
	}
}

func TestNVIDIASettingsAndCurationHelpers(t *testing.T) {
	settings := normalizeNVIDIAAISettings(nvidiaAISettings{})
	if settings.Model != defaultNVIDIAModel {
		t.Fatalf("unexpected default NVIDIA model: %q", settings.Model)
	}
	if !validNVIDIAAPIKey("nvapi-test-key-123456") || validNVIDIAAPIKey("key with spaces") {
		t.Fatal("NVIDIA API key validation is incorrect")
	}
	if !validNVIDIAModel("nvidia/nemotron-3-nano-30b-a3b") || validNVIDIAModel("bad model name") {
		t.Fatal("NVIDIA model validation is incorrect")
	}

	curation, err := parseNVIDIACuration("```json\n{\"answer\":\"여름 데이트 전시입니다.\",\"recommended_ids\":[\"post-2\",\"post-1\"]}\n```")
	if err != nil {
		t.Fatalf("parse NVIDIA curation: %v", err)
	}
	posts := []Post{{ID: "post-1", Title: "첫 전시"}, {ID: "post-2", Title: "둘째 전시"}}
	ids := validRecommendedIDs(append(curation.RecommendedIDs, "post-2", "missing"), posts, 12)
	ordered := postsByRecommendedIDs(posts, ids)
	if len(ordered) != 2 || ordered[0].ID != "post-2" || ordered[1].ID != "post-1" {
		t.Fatalf("unexpected curated post order: %#v", ordered)
	}
}

func TestNamedSettingEncryptionUsesSeparateContext(t *testing.T) {
	server := Server{config: Config{SessionSecret: "test-session-secret-that-is-long-enough"}}
	encrypted, err := server.sealNamedSetting(nvidiaAISettingName, []byte("secret"))
	if err != nil {
		t.Fatalf("seal named setting: %v", err)
	}
	plaintext, err := server.openNamedSetting(nvidiaAISettingName, encrypted)
	if err != nil || string(plaintext) != "secret" {
		t.Fatalf("open named setting: %q, %v", plaintext, err)
	}
	if _, err := server.openNamedSetting(publicDataSettingName, encrypted); err == nil {
		t.Fatal("setting encrypted for NVIDIA must not decrypt in public-data context")
	}
}

func TestNVIDIAChatClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected NVIDIA API path: %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer nvapi-test-key-123456" {
			t.Fatal("NVIDIA authorization header is missing")
		}
		var request nvidiaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode NVIDIA request: %v", err)
		}
		if request.Model != defaultNVIDIAModel || request.MaxTokens != 32 {
			t.Fatalf("unexpected NVIDIA request: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"OK"}}]}`))
	}))
	defer server.Close()

	content, err := callNVIDIAChatAtEndpoint(t.Context(), server.URL+"/v1", nvidiaAISettings{
		APIKey: "nvapi-test-key-123456",
	}, []nvidiaChatMessage{{Role: "user", Content: "test"}}, 32)
	if err != nil || content != "OK" {
		t.Fatalf("unexpected NVIDIA response: %q, %v", content, err)
	}
}
