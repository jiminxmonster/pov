package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestSaveSubmissionInlineMedia(t *testing.T) {
	uploadDir := t.TempDir()
	server := Server{config: Config{BasePath: "/pov", UploadDir: uploadDir}}
	body := "전시명:\n여름의 표면\n\n장소:\n서울 성동구\n\n전시내용:\n이미지 앞\n\n![장면](pov-inline://inline-test-1)\n\n@[영상](pov-video://video-test-1)\n\n영상 뒤"

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile("inline_image_inline-test-1", "scene.png")
	if err != nil {
		t.Fatalf("create image part: %v", err)
	}
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode test image: %v", err)
	}
	if _, err := part.Write(png); err != nil {
		t.Fatalf("write image part: %v", err)
	}
	videoPart, err := writer.CreateFormFile("inline_video_video-test-1", "scene.mp4")
	if err != nil {
		t.Fatalf("create video part: %v", err)
	}
	if _, err := videoPart.Write([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}); err != nil {
		t.Fatalf("write video part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/pov/api/submissions", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(8 << 20); err != nil {
		t.Fatalf("parse multipart form: %v", err)
	}
	defer request.MultipartForm.RemoveAll()

	resolved, urls, names, err := server.saveSubmissionInlineMedia(request, body)
	if err != nil {
		t.Fatalf("save inline media: %v", err)
	}
	if len(names) != 2 || urls["inline-test-1"] == "" {
		t.Fatalf("unexpected saved media: names=%#v urls=%#v", names, urls)
	}
	if strings.Contains(resolved, "pov-inline://") || strings.Contains(resolved, "pov-video://") || !strings.Contains(resolved, urls["inline-test-1"]) {
		t.Fatalf("inline placeholder was not resolved: %q", resolved)
	}
	if firstInlineImageURL(body, urls) != urls["inline-test-1"] {
		t.Fatal("first inline image was not selected as the default cover")
	}
	if _, err := os.Stat(uploadDir + "/" + names[0]); err != nil {
		t.Fatalf("saved image is missing: %v", err)
	}
}

func TestSubmissionRejectsMissingInlineImage(t *testing.T) {
	server := Server{config: Config{BasePath: "/pov", UploadDir: t.TempDir()}}
	request := httptest.NewRequest(http.MethodPost, "/pov/api/submissions", nil)
	request.MultipartForm = &multipart.Form{File: map[string][]*multipart.FileHeader{}}
	if _, _, _, err := server.saveSubmissionInlineMedia(request, "전시내용:\n![장면](pov-inline://missing)"); err == nil {
		t.Fatal("missing inline upload must be rejected")
	}
}

func TestConvertSeoulEvent(t *testing.T) {
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

	exhibition, ok := convertSeoulEvent(event)
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
	ended, ok := convertSeoulEvent(event)
	if !ok || ended.Metadata["전시종료일"] != "2026-07-17" {
		t.Fatal("ended exhibition must remain available with its end date")
	}
}

func TestDecodeAndConvertKCISAEvent(t *testing.T) {
	response := `{
		"response": {
			"header": {"resultCode": "0000", "resultMsg": "정상 처리"},
			"body": {
				"items": {"item": [{
					"TITLE": "<b>통합 전시</b>",
					"CNTC_INSTT_NM": "국립현대미술관",
					"DESCRIPTION": "전시 소개 &amp; 안내",
					"IMAGE_OBJECT": "https://example.com/cover.jpg",
					"LOCAL_ID": "exhibition-42",
					"URL": "https://example.com/exhibitions/42",
					"EVENT_SITE": "국립현대미술관 서울",
					"AUTHOR": "홍길동",
					"CHARGE": "무료",
					"PERIOD": "2026.07.01 ~ 2026.09.20",
					"EVENT_PERIOD": "10:00 ~ 18:00"
				}]},
				"numOfRows": "1",
				"pageNo": "1",
				"totalCount": "1"
			}
		}
	}`
	payload, err := decodeKCISAResponse(strings.NewReader(response))
	if err != nil || len(payload.Body.Items.Item) != 1 {
		t.Fatalf("decode KCISA response: %#v, %v", payload, err)
	}
	exhibition, ok := convertKCISAEvent(payload.Body.Items.Item[0])
	if !ok {
		t.Fatal("expected KCISA exhibition to be converted")
	}
	if exhibition.Title != "통합 전시" || exhibition.ImageURL != "https://example.com/cover.jpg" {
		t.Fatalf("unexpected KCISA exhibition: %#v", exhibition)
	}
	if exhibition.Metadata["전시시작일"] != "2026-07-01" || exhibition.Metadata["전시종료일"] != "2026-09-20" {
		t.Fatalf("unexpected KCISA dates: %#v", exhibition.Metadata)
	}
	if exhibition.Metadata["지도표시"] != "아니오" || exhibition.Latitude != 0 || exhibition.Longitude != 0 {
		t.Fatal("KCISA data without coordinates must not create an inaccurate map marker")
	}
	if !strings.Contains(exhibition.BodyMarkdown, kcisaOpenDataSource) || !strings.Contains(exhibition.BodyMarkdown, "전시 소개 & 안내") {
		t.Fatalf("KCISA source or description is missing: %q", exhibition.BodyMarkdown)
	}
}

func TestKCISAProviderErrorIsActionableAndSafe(t *testing.T) {
	err := decodeKCISAHTTPError(http.StatusForbidden, strings.NewReader(`{
		"message":"API Key is not valid or is expired / revoked.",
		"http_status_code":403
	}`))
	if !strings.Contains(err.Error(), "API Key is not valid") {
		t.Fatalf("provider error detail was lost: %v", err)
	}
	message := kcisaDataSyncErrorMessage(err)
	if !strings.Contains(message, "유효하지 않거나 아직 활성화되지 않았습니다") {
		t.Fatalf("unexpected operator message: %q", message)
	}
	if strings.Contains(message, "invalid-debug-key") {
		t.Fatal("operator error must never expose a service key")
	}
}

func TestExhibitionLifecycleVisibility(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	post := func(id, endDate string) Post {
		return Post{ID: id, Metadata: map[string]string{"전시종료일": endDate}}
	}
	posts := []Post{
		post("active", "2026-08-01"),
		post("recent-ended", "2026-07-19"),
		post("one-month-boundary", "2026-06-20"),
		post("knowledge-only", "2026-06-19"),
		{ID: "no-date", Metadata: map[string]string{}},
		{ID: "period-fallback", Metadata: map[string]string{"전시기간": "2026. 01. 01 ~ 2026. 03. 15"}},
	}

	visible := publicIndexExhibitions(posts, now, 20)
	visibleIDs := make([]string, 0, len(visible))
	for _, item := range visible {
		visibleIDs = append(visibleIDs, item.ID)
	}
	if len(visible) != 4 || strings.Join(visibleIDs, ",") != "active,recent-ended,one-month-boundary,no-date" {
		t.Fatalf("unexpected public index exhibitions: %#v", visible)
	}
	current := currentExhibitions(posts, now, 20)
	if len(current) != 2 || current[0].ID != "active" || current[1].ID != "no-date" {
		t.Fatalf("unexpected map exhibitions: %#v", current)
	}
	mapPosts := currentMapExhibitions([]Post{
		{ID: "mapped-undated", Latitude: 37.5, Longitude: 127.0, Metadata: map[string]string{}},
		{ID: "mapped-scheduled", Latitude: 37.5, Longitude: 127.0, Metadata: map[string]string{"전시종료일": "2026-08-01"}},
		{ID: "unmapped", Latitude: 0, Longitude: 0, Metadata: map[string]string{"지도표시": "아니오"}},
	}, now, 20)
	if len(mapPosts) != 2 || mapPosts[0].ID != "mapped-scheduled" || mapPosts[1].ID != "mapped-undated" {
		t.Fatalf("map must only include verified coordinates: %#v", mapPosts)
	}
	if !isExhibitionExpiredAt(posts[5], now) || isPublicIndexExhibitionAt(posts[5], now) {
		t.Fatal("period fallback must be retained as knowledge only")
	}
	knowledge := historicalKnowledgeExhibitions(posts, now, 3)
	if len(knowledge) != 3 || knowledge[0].ID != "recent-ended" || knowledge[1].ID != "one-month-boundary" || knowledge[2].ID != "knowledge-only" {
		t.Fatalf("historical knowledge must prioritize ended exhibitions: %#v", knowledge)
	}
}

func TestSubmissionDraftAllowsBodyWithoutTitleOrPlace(t *testing.T) {
	title, ok := submissionDraftTitle("전시내용:\n오늘 본 전시에서 긴 여운을 느꼈다.")
	if !ok || title != "사용자 관람시점" {
		t.Fatalf("body-only submission must enter review: title=%q ok=%v", title, ok)
	}
	if _, ok := submissionDraftTitle("전시내용:\n짧음"); ok {
		t.Fatal("submission body shorter than five runes must be rejected")
	}
	title, ok = submissionDraftTitle("전시명:\n빛의 방\n\n전시내용:\n본문이 충분합니다.")
	if !ok || title != "빛의 방" {
		t.Fatalf("provided title must be preserved: title=%q ok=%v", title, ok)
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
	kcisa := normalizeKCISADataSettings(publicDataSettings{APIKey: "encoded%2Bkey%2Fvalue", Limit: 5000})
	if kcisa.APIKey != "encoded+key/value" || kcisa.Limit != 1000 || !validKCISADataKey(kcisa.APIKey) {
		t.Fatalf("unexpected KCISA settings: %#v", kcisa)
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

	curation, err := parseNVIDIACuration("```json\n{\"mode\":\"map\",\"answer\":\"여름 데이트 전시입니다.\",\"recommended_ids\":[\"post-2\",\"post-1\"]}\n```")
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

func TestParseNVIDIACurationFindsValidObjectAfterModelPreamble(t *testing.T) {
	content := "검토 형식: {not-json}\n최종 답변:\n```json\n{\"mode\":\"chat\",\"answer\":\"관람료는 무료입니다.\",\"recommended_ids\":[\"post-1\"]}\n```"
	curation, err := parseNVIDIACuration(content)
	if err != nil || curation.Mode != "chat" || curation.Answer == "" || len(curation.RecommendedIDs) != 1 {
		t.Fatalf("expected valid trailing object, got %#v, %v", curation, err)
	}
}

func TestAIRouteNormalizationAndFallback(t *testing.T) {
	options := normalizedAIOptions([]string{" 연인과 ", "연인과", "가족과", "친구와", "혼자", "추가"})
	if len(options) != 4 || options[0] != "연인과" {
		t.Fatalf("unexpected normalized options: %#v", options)
	}
	if normalizedAIMode("unknown") != "map" || normalizedAIMode("CHAT") != "chat" {
		t.Fatal("AI mode normalization failed")
	}

	wizard := fallbackAIDecision("전시 추천해줘", nil, nil)
	if wizard.Mode != "wizard" || wizard.Question == "" || len(wizard.Options) < 2 {
		t.Fatalf("expected wizard fallback, got %#v", wizard)
	}
	posts := []Post{{ID: "post-1", Title: "첫 전시"}}
	chat := fallbackAIDecision("첫 전시 관람료 알려줘", nil, posts)
	if chat.Mode != "chat" || len(chat.RecommendedIDs) != 1 {
		t.Fatalf("expected chat fallback, got %#v", chat)
	}
	for _, query := range []string{"이 전시는 어떤 내용이야?", "이 작가는 누구야?", "아이와 가도 돼?"} {
		if result := fallbackAIDecision(query, nil, posts); result.Mode != "chat" {
			t.Fatalf("expected conversational question %q to open chat, got %#v", query, result)
		}
	}
	mapResult := fallbackAIDecision("성수에서 연인과 볼 무료 전시", nil, posts)
	if mapResult.Mode != "map" || len(mapResult.RecommendedIDs) != 1 {
		t.Fatalf("expected map fallback, got %#v", mapResult)
	}
	recommendationQuestion := fallbackAIDecision("여름에 연인과 볼만한 전시 있어?", nil, posts)
	if recommendationQuestion.Mode != "map" {
		t.Fatalf("expected recommendation question to stay on map, got %#v", recommendationQuestion)
	}
}

func TestSourceLinksOnlyUseRegisteredHTTPURLs(t *testing.T) {
	posts := []Post{
		{Title: "공식 전시", Metadata: map[string]string{"원문 링크": "https://example.com/exhibition"}},
		{Title: "위험한 전시", Metadata: map[string]string{"원문 링크": "javascript:alert(1)"}},
	}
	links := sourceLinksForPosts(posts)
	if len(links) != 1 || links[0].URL != "https://example.com/exhibition" {
		t.Fatalf("unexpected source links: %#v", links)
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
	kcisaEncrypted, err := server.sealNamedSetting(kcisaPublicDataSettingName, []byte("kcisa-secret"))
	if err != nil {
		t.Fatalf("seal KCISA setting: %v", err)
	}
	if _, err := server.openNamedSetting(publicDataSettingName, kcisaEncrypted); err == nil {
		t.Fatal("KCISA setting must use a separate encryption context")
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
		if request.Model != defaultNVIDIAModel || request.MaxTokens != 32 || request.ReasoningBudget != 0 || request.ChatTemplateKwargs == nil || request.ChatTemplateKwargs.EnableThinking {
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

func TestNVIDIAReasoningBudgetLeavesRoomForStructuredAnswer(t *testing.T) {
	if got := nvidiaReasoningBudget(defaultNVIDIAModel, 900); got != 225 {
		t.Fatalf("expected bounded reasoning budget, got %d", got)
	}
	if got := nvidiaReasoningBudget("qwen/qwen3.5-122b-a10b", 900); got != 0 {
		t.Fatalf("reasoning budget should be omitted for other model families, got %d", got)
	}
}

func TestInitialWizardAndInformationRouting(t *testing.T) {
	decision, ok := initialWizardDecision("전시 추천해줘", nil)
	if !ok || decision.Mode != "wizard" || len(decision.Options) < 2 {
		t.Fatalf("expected initial wizard, got %#v", decision)
	}
	if _, ok := initialWizardDecision("성수에서 연인과 볼 전시 추천해줘", nil); ok {
		t.Fatal("clear recommendation should not open the wizard")
	}
	if !isInformationQuery("덕수궁 전시 관람료와 주차 정보를 알려줘") {
		t.Fatal("explicit exhibition information request should stay in chat")
	}
	if isInformationQuery("주차 가능한 전시를 추천해줘") {
		t.Fatal("recommendation request should remain eligible for the map")
	}
	for _, query := range []string{"끝난 전시 중 사진 전시가 있었어?", "전시의 종류를 알려줘", "예전에 열렸던 전시"} {
		if !isHistoricalKnowledgeQuery(query) {
			t.Fatalf("historical knowledge query was not detected: %q", query)
		}
		if decision, ok := initialWizardDecision(query+" 추천해줘", nil); ok || decision.Mode != "" {
			t.Fatalf("historical knowledge query must not open the recommendation wizard: %#v", decision)
		}
	}
}

func TestConversationSearchKeepsRecommendationConstraints(t *testing.T) {
	history := []aiConversationTurn{
		{Role: "user", Content: "전시 추천해줘"},
		{Role: "assistant", Content: "누구와 함께하시나요?"},
	}
	query := aiConversationQuery("연인과 함께", history)
	if !conversationRequestsRecommendation("연인과 함께", history) {
		t.Fatal("wizard follow-up should remain a recommendation request")
	}
	terms := knownSearchTerms(query)
	if len(terms) != 1 || terms[0] != "연인" {
		t.Fatalf("expected companion constraint, got %#v", terms)
	}
	directTerms := recommendationCandidateTerms("성수에서 연인과 함께 볼 무료 전시")
	if len(directTerms) != 2 || directTerms[0] != "무료" || directTerms[1] != "성수" {
		t.Fatalf("expected hard recommendation terms only, got %#v", directTerms)
	}
}

func TestSanitizeAITextRemovesReasoningTags(t *testing.T) {
	if got := sanitizeAIText("<think>내부 추론</think>최종 답변</think>", 100); got != "최종 답변" {
		t.Fatalf("unexpected sanitized answer: %q", got)
	}
}
