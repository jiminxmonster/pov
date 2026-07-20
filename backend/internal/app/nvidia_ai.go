package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	nvidiaAISettingName = "nvidia_ai"
	nvidiaAPIEndpoint   = "https://integrate.api.nvidia.com/v1"
	defaultNVIDIAModel  = "nvidia/nemotron-3-nano-30b-a3b"
)

var nvidiaModelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{2,127}$`)

type nvidiaAISettings struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

type nvidiaAISettingsResponse struct {
	Configured bool   `json:"configured"`
	MaskedKey  string `json:"masked_key"`
	Model      string `json:"model"`
	Endpoint   string `json:"endpoint"`
	Storage    string `json:"storage"`
	Message    string `json:"message,omitempty"`
}

type nvidiaChatRequest struct {
	Model              string                    `json:"model"`
	Messages           []nvidiaChatMessage       `json:"messages"`
	Temperature        float64                   `json:"temperature"`
	MaxTokens          int                       `json:"max_tokens"`
	ReasoningBudget    int                       `json:"reasoning_budget,omitempty"`
	ChatTemplateKwargs *nvidiaChatTemplateKwargs `json:"chat_template_kwargs,omitempty"`
	Stream             bool                      `json:"stream"`
}

type nvidiaChatTemplateKwargs struct {
	EnableThinking bool `json:"enable_thinking"`
}

type nvidiaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type nvidiaChatResponse struct {
	Choices []struct {
		Message nvidiaChatMessage `json:"message"`
	} `json:"choices"`
}

type nvidiaCuration struct {
	Answer         string   `json:"answer"`
	RecommendedIDs []string `json:"recommended_ids"`
	Mode           string   `json:"mode"`
	Question       string   `json:"question,omitempty"`
	Options        []string `json:"options,omitempty"`
}

type aiConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type curationCandidate struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Address     string `json:"address"`
	Period      string `json:"period,omitempty"`
	Fee         string `json:"fee,omitempty"`
	Artist      string `json:"artist,omitempty"`
	Description string `json:"description,omitempty"`
	Docent      string `json:"docent,omitempty"`
	Parking     string `json:"parking,omitempty"`
	Nearby      string `json:"nearby,omitempty"`
	Food        string `json:"food,omitempty"`
	Review      string `json:"review,omitempty"`
	Persona     string `json:"persona,omitempty"`
}

func (s *Server) getNVIDIAAISettings(w http.ResponseWriter, r *http.Request) {
	settings, stored, err := s.loadNVIDIAAISettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "NVIDIA AI 설정을 불러오지 못했습니다")
		return
	}
	writeJSON(w, http.StatusOK, nvidiaAISettingsPayload(settings, stored))
}

func (s *Server) updateNVIDIAAISettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		APIKey string `json:"api_key"`
		Model  string `json:"model"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}

	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		current, _, err := s.loadNVIDIAAISettings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "기존 NVIDIA API 키를 불러오지 못했습니다")
			return
		}
		apiKey = current.APIKey
	}
	settings := normalizeNVIDIAAISettings(nvidiaAISettings{APIKey: apiKey, Model: input.Model})
	if !validNVIDIAAPIKey(settings.APIKey) {
		writeError(w, http.StatusBadRequest, "NVIDIA API 키 형식을 확인해 주세요")
		return
	}
	if !validNVIDIAModel(settings.Model) {
		writeError(w, http.StatusBadRequest, "NVIDIA 모델명을 확인해 주세요")
		return
	}

	testContext, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	if err := testNVIDIAConnection(testContext, settings); err != nil {
		writeError(w, http.StatusBadGateway, "NVIDIA API 키 또는 모델 연결 상태를 확인해 주세요")
		return
	}
	if err := s.storeNVIDIAAISettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, "NVIDIA AI 설정을 저장하지 못했습니다")
		return
	}

	payload := nvidiaAISettingsPayload(settings, true)
	payload.Message = "NVIDIA AI 연결을 확인하고 설정을 저장했습니다."
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) testNVIDIAAISettings(w http.ResponseWriter, r *http.Request) {
	settings, stored, err := s.loadNVIDIAAISettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "NVIDIA AI 설정을 불러오지 못했습니다")
		return
	}
	if !stored || !validNVIDIAAPIKey(settings.APIKey) {
		writeError(w, http.StatusBadRequest, "먼저 NVIDIA API 키를 저장해 주세요")
		return
	}
	testContext, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	if err := testNVIDIAConnection(testContext, settings); err != nil {
		writeError(w, http.StatusBadGateway, "NVIDIA API에 연결하지 못했습니다")
		return
	}
	payload := nvidiaAISettingsPayload(settings, true)
	payload.Message = "NVIDIA AI 연결이 정상입니다."
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) loadNVIDIAAISettings(ctx context.Context) (nvidiaAISettings, bool, error) {
	fallback := normalizeNVIDIAAISettings(nvidiaAISettings{})
	var encrypted []byte
	err := s.db.QueryRow(ctx, `SELECT value_encrypted FROM app_settings WHERE name = $1`, nvidiaAISettingName).Scan(&encrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		return fallback, false, nil
	}
	if err != nil {
		return nvidiaAISettings{}, false, err
	}
	plaintext, err := s.openNamedSetting(nvidiaAISettingName, encrypted)
	if err != nil {
		return nvidiaAISettings{}, true, err
	}
	var settings nvidiaAISettings
	if err := json.Unmarshal(plaintext, &settings); err != nil {
		return nvidiaAISettings{}, true, err
	}
	settings = normalizeNVIDIAAISettings(settings)
	return settings, settings.APIKey != "", nil
}

func (s *Server) storeNVIDIAAISettings(ctx context.Context, settings nvidiaAISettings) error {
	payload, err := json.Marshal(normalizeNVIDIAAISettings(settings))
	if err != nil {
		return err
	}
	encrypted, err := s.sealNamedSetting(nvidiaAISettingName, payload)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO app_settings (name, value_encrypted)
		VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET value_encrypted = EXCLUDED.value_encrypted, updated_at = NOW()
	`, nvidiaAISettingName, encrypted)
	return err
}

func normalizeNVIDIAAISettings(settings nvidiaAISettings) nvidiaAISettings {
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	settings.Model = strings.TrimSpace(settings.Model)
	if settings.Model == "" {
		settings.Model = defaultNVIDIAModel
	}
	return settings
}

func validNVIDIAAPIKey(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 12 && len(value) <= 512 && !strings.ContainsAny(value, " \t\r\n")
}

func validNVIDIAModel(value string) bool {
	return nvidiaModelPattern.MatchString(strings.TrimSpace(value))
}

func nvidiaAISettingsPayload(settings nvidiaAISettings, stored bool) nvidiaAISettingsResponse {
	storage := "default"
	if stored {
		storage = "database"
	}
	return nvidiaAISettingsResponse{
		Configured: settings.APIKey != "",
		MaskedKey:  maskSecret(settings.APIKey),
		Model:      settings.Model,
		Endpoint:   nvidiaAPIEndpoint,
		Storage:    storage,
	}
}

func testNVIDIAConnection(ctx context.Context, settings nvidiaAISettings) error {
	content, err := callNVIDIAChat(ctx, settings, []nvidiaChatMessage{
		{Role: "system", Content: "Reply with only the word OK."},
		{Role: "user", Content: "POV connection test"},
	}, 16)
	if err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" {
		return errors.New("empty NVIDIA response")
	}
	return nil
}

func curateWithNVIDIA(ctx context.Context, settings nvidiaAISettings, query string, history []aiConversationTurn, posts []Post) (nvidiaCuration, error) {
	candidates := make([]curationCandidate, 0, len(posts))
	for _, post := range posts {
		candidates = append(candidates, curationCandidate{
			ID:          post.ID,
			Title:       post.Title,
			Address:     post.Address,
			Period:      post.Metadata["전시기간"],
			Fee:         post.Metadata["관람료"],
			Artist:      post.Metadata["작가(작가소개)"],
			Description: limitRunes(post.Metadata["전시내용"], 700),
			Docent:      post.Metadata["도슨트(전시장 가이드) 유무"],
			Parking:     post.Metadata["주차정보"],
			Nearby:      firstNonEmpty(post.Metadata["주변에 함께 볼 만한 전시"], post.Metadata["주변에 볼거리"]),
			Food:        post.Metadata["맛집"],
			Review:      post.Metadata["감상평"],
			Persona:     post.Metadata["페르소나 정보입력"],
		})
	}
	candidateJSON, err := json.Marshal(candidates)
	if err != nil {
		return nvidiaCuration{}, err
	}

	today := time.Now().In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02")
	systemPrompt := `당신은 POV 전시 큐레이터입니다. 사용자의 요청을 아래 세 가지 응답 방식 중 하나로 판단하세요.
1. map: 조건이 충분히 명확하고 등록된 전시에서 곧바로 추천할 수 있을 때
2. wizard: 추천 결과를 크게 바꾸는 조건이 부족해 한 번의 짧은 역질문이 필요할 때
3. chat: 전시 정보, 관람 방법, 장소, 비용, 일정, 링크처럼 대화 안에서 직접 설명하는 편이 나을 때

불필요한 역질문은 하지 말고 명확한 질문은 map을 우선하세요. 반드시 제공된 후보의 사실만 사용하고 존재하지 않는 전시나 링크를 만들지 마세요.
map이면 recommended_ids에 추천 순서대로 최대 12개 id를 넣고 answer에 추천 이유를 2~4문장으로 쓰세요.
wizard이면 question에 한 가지 질문만 쓰고 options에는 서로 겹치지 않는 짧은 선택지 2~4개를 넣으세요. answer는 질문이 필요한 이유를 한 문장으로 쓰세요.
chat이면 answer에 자연스러운 한국어 2~5문장으로 직접 답하고 관련 전시가 있으면 recommended_ids에 최대 6개 id를 넣으세요.
마크다운이나 코드 블록 없이 아래 JSON 객체 하나만 출력하세요.
{"mode":"map|wizard|chat","answer":"...","question":"...","options":["..."],"recommended_ids":["..."]}`
	userPrompt := fmt.Sprintf("오늘 날짜: %s\n사용자 질문: %s\n전시 후보 JSON:\n%s", today, query, candidateJSON)
	messages := []nvidiaChatMessage{{Role: "system", Content: systemPrompt}}
	messages = append(messages, normalizedAIHistory(history)...)
	messages = append(messages, nvidiaChatMessage{Role: "user", Content: userPrompt})
	content, err := callNVIDIAChat(ctx, settings, messages, 900)
	if err != nil {
		return nvidiaCuration{}, err
	}
	curation, err := parseNVIDIACuration(content)
	if err != nil {
		return nvidiaCuration{}, err
	}
	curation.Mode = normalizedAIMode(curation.Mode)
	if isInformationQuery(query) {
		curation.Mode = "chat"
	}
	recommendationLimit := 12
	if curation.Mode == "chat" {
		recommendationLimit = 6
	}
	curation.RecommendedIDs = validRecommendedIDs(curation.RecommendedIDs, posts, recommendationLimit)
	if !isInformationQuery(query) && conversationRequestsRecommendation(query, history) && len(curation.RecommendedIDs) > 0 {
		curation.Mode = "map"
	}
	curation.Question = sanitizeAIText(curation.Question, 180)
	curation.Options = normalizedAIOptions(curation.Options)
	if curation.Mode == "map" && len(curation.RecommendedIDs) == 0 {
		return nvidiaCuration{}, errors.New("NVIDIA returned no valid exhibition IDs")
	}
	if curation.Mode == "wizard" && (curation.Question == "" || len(curation.Options) < 2) {
		return nvidiaCuration{}, errors.New("NVIDIA returned an incomplete wizard question")
	}
	curation.Answer = sanitizeAIText(curation.Answer, 700)
	if curation.Answer == "" {
		curation.Answer = "질문과 가까운 전시를 추천 순서대로 모았습니다."
	}
	return curation, nil
}

func normalizedAIHistory(history []aiConversationTurn) []nvidiaChatMessage {
	if len(history) > 10 {
		history = history[len(history)-10:]
	}
	result := make([]nvidiaChatMessage, 0, len(history))
	for _, turn := range history {
		role := strings.ToLower(strings.TrimSpace(turn.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(limitRunes(turn.Content, 800))
		if content != "" {
			result = append(result, nvidiaChatMessage{Role: role, Content: content})
		}
	}
	return result
}

func normalizedAIMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "map", "wizard", "chat":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "map"
	}
}

func normalizedAIOptions(options []string) []string {
	result := make([]string, 0, min(len(options), 4))
	seen := make(map[string]bool, len(options))
	for _, option := range options {
		option = sanitizeAIText(option, 40)
		if option == "" || seen[option] {
			continue
		}
		seen[option] = true
		result = append(result, option)
		if len(result) == 4 {
			break
		}
	}
	return result
}

func fallbackAIDecision(query string, history []aiConversationTurn, posts []Post) nvidiaCuration {
	query = strings.TrimSpace(query)
	lower := strings.ToLower(query)
	if wizard, ok := initialWizardDecision(query, history); ok {
		return wizard
	}

	ids := make([]string, 0, min(len(posts), 12))
	for _, post := range posts {
		ids = append(ids, post.ID)
		if len(ids) == 12 {
			break
		}
	}
	if isInformationQuery(lower) {
		answer := interpretQuery(query)
		if len(posts) == 0 {
			answer = "등록된 전시 정보에서는 바로 확인할 내용을 찾지 못했어요. 전시명이나 지역을 조금 더 구체적으로 알려주세요."
		}
		return nvidiaCuration{Mode: "chat", Answer: answer, RecommendedIDs: ids[:min(len(ids), 6)]}
	}
	return nvidiaCuration{Mode: "map", Answer: interpretQuery(query), RecommendedIDs: ids}
}

func initialWizardDecision(query string, history []aiConversationTurn) (nvidiaCuration, bool) {
	lower := strings.ToLower(strings.TrimSpace(query))
	if len(history) != 0 || !containsAny(lower, "추천해", "추천 해", "뭐 볼", "무엇을 볼", "어디 갈", "볼만한 전시") ||
		containsAny(lower, "연인", "데이트", "가족", "아이", "혼자", "무료", "주차", "성수", "종로", "강남", "홍대", "이번 주", "주말", "오늘") {
		return nvidiaCuration{}, false
	}
	return nvidiaCuration{
		Mode: "wizard", Answer: "조금만 더 알면 지금 마음에 가까운 전시를 고를 수 있어요.",
		Question: "이번 관람은 누구와 함께하시나요?", Options: []string{"혼자 천천히", "연인과 함께", "가족과 함께", "친구와 함께"},
	}, true
}

func isInformationQuery(query string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	if containsAny(lower, "추천", "찾아", "골라", "볼만한", "뭐 볼", "무엇을 볼", "어디 갈") {
		return false
	}
	return containsAny(lower,
		"알려", "어떻게", "언제", "어디", "관람료", "주차", "도슨트", "링크", "홈페이지", "정보", "설명",
		"누구", "무엇", "어떤", "뭐야", "어때", "왜", "가능", "있어", "없어", "가도 돼", "해도 돼",
	)
}

func aiConversationQuery(query string, history []aiConversationTurn) string {
	parts := make([]string, 0, len(history)+1)
	for _, turn := range history {
		if strings.EqualFold(strings.TrimSpace(turn.Role), "user") && strings.TrimSpace(turn.Content) != "" {
			parts = append(parts, strings.TrimSpace(limitRunes(turn.Content, 200)))
		}
	}
	if strings.TrimSpace(query) != "" {
		parts = append(parts, strings.TrimSpace(limitRunes(query, 200)))
	}
	return strings.Join(parts, " ")
}

func conversationRequestsRecommendation(query string, history []aiConversationTurn) bool {
	return containsAny(strings.ToLower(aiConversationQuery(query, history)), "추천", "뭐 볼", "무엇을 볼", "어디 갈", "볼만한")
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func sanitizeAIText(value string, limit int) string {
	value = strings.TrimSpace(value)
	for {
		start := strings.Index(value, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(value[start+len("<think>"):], "</think>")
		if end < 0 {
			value = value[:start]
			break
		}
		end += start + len("<think>")
		value = value[:start] + value[end+len("</think>"):]
	}
	value = strings.ReplaceAll(value, "</think>", "")
	return strings.TrimSpace(limitRunes(value, limit))
}

func sourceLinksForPosts(posts []Post) []searchLink {
	links := make([]searchLink, 0, min(len(posts), 6))
	seen := make(map[string]bool, len(posts))
	for _, post := range posts {
		url := safeHTTPURL(post.Metadata["원문 링크"])
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		links = append(links, searchLink{Label: post.Title + " 원문 보기", URL: url})
		if len(links) == 6 {
			break
		}
	}
	return links
}

func callNVIDIAChat(ctx context.Context, settings nvidiaAISettings, messages []nvidiaChatMessage, maxTokens int) (string, error) {
	return callNVIDIAChatAtEndpoint(ctx, nvidiaAPIEndpoint, settings, messages, maxTokens)
}

func callNVIDIAChatAtEndpoint(ctx context.Context, endpoint string, settings nvidiaAISettings, messages []nvidiaChatMessage, maxTokens int) (string, error) {
	settings = normalizeNVIDIAAISettings(settings)
	payload, err := json.Marshal(nvidiaChatRequest{
		Model: settings.Model, Messages: messages, Temperature: 0.2, MaxTokens: maxTokens,
		ReasoningBudget:    nvidiaReasoningBudget(settings.Model, maxTokens),
		ChatTemplateKwargs: nvidiaChatTemplateSettings(settings.Model), Stream: false,
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+settings.APIKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "POV-Exhibition-Curator/1.0")

	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("NVIDIA request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return "", fmt.Errorf("NVIDIA API status %d", response.StatusCode)
	}
	var result nvidiaChatResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result); err != nil {
		return "", errors.New("NVIDIA response could not be decoded")
	}
	if len(result.Choices) == 0 {
		return "", errors.New("NVIDIA response did not contain a choice")
	}
	return result.Choices[0].Message.Content, nil
}

func nvidiaReasoningBudget(model string, maxTokens int) int {
	if !strings.Contains(strings.ToLower(model), "nemotron-3-") || maxTokens < 64 {
		return 0
	}
	return min(256, maxTokens/4)
}

func nvidiaChatTemplateSettings(model string) *nvidiaChatTemplateKwargs {
	if !strings.Contains(strings.ToLower(model), "nemotron-3-") {
		return nil
	}
	return &nvidiaChatTemplateKwargs{EnableThinking: false}
}

func parseNVIDIACuration(content string) (nvidiaCuration, error) {
	content = strings.TrimSpace(content)
	foundObject := false
	for offset := 0; offset < len(content); {
		relativeStart := strings.Index(content[offset:], "{")
		if relativeStart < 0 {
			break
		}
		start := offset + relativeStart
		foundObject = true
		var curation nvidiaCuration
		decoder := json.NewDecoder(strings.NewReader(content[start:]))
		if err := decoder.Decode(&curation); err == nil &&
			(curation.Mode != "" || curation.Answer != "" || curation.Question != "" || len(curation.RecommendedIDs) > 0) {
			return curation, nil
		}
		offset = start + 1
	}
	if !foundObject {
		return nvidiaCuration{}, errors.New("NVIDIA response was not JSON")
	}
	return nvidiaCuration{}, errors.New("NVIDIA curation JSON was invalid")
}

func validRecommendedIDs(ids []string, posts []Post, limit int) []string {
	available := make(map[string]bool, len(posts))
	for _, post := range posts {
		available[post.ID] = true
	}
	seen := make(map[string]bool, len(ids))
	valid := make([]string, 0, min(len(ids), limit))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || !available[id] || seen[id] {
			continue
		}
		valid = append(valid, id)
		seen[id] = true
		if len(valid) == limit {
			break
		}
	}
	return valid
}

func postsByRecommendedIDs(posts []Post, ids []string) []Post {
	byID := make(map[string]Post, len(posts))
	for _, post := range posts {
		byID[post.ID] = post
	}
	result := make([]Post, 0, len(ids))
	for _, id := range ids {
		if post, ok := byID[id]; ok {
			result = append(result, post)
		}
	}
	return result
}

func limitRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
