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
	Model       string              `json:"model"`
	Messages    []nvidiaChatMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens"`
	Stream      bool                `json:"stream"`
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

func curateWithNVIDIA(ctx context.Context, settings nvidiaAISettings, query string, posts []Post) (nvidiaCuration, error) {
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
	systemPrompt := `당신은 POV 전시 큐레이터입니다. 반드시 제공된 전시 후보의 사실만 사용하세요.
사용자의 계절, 동행인, 지역, 비용, 분위기, 이동 편의 조건을 해석해 가장 알맞은 전시를 최대 12개 고르세요.
근거가 없는 정보나 전시를 만들지 말고, 정보가 부족하면 그 사실을 짧게 밝히세요.
answer는 자연스러운 한국어 2~4문장으로 추천 기준과 핵심 이유를 설명하세요.
recommended_ids에는 추천 순서대로 후보 id만 넣으세요.
마크다운이나 코드 블록 없이 아래 JSON 객체 하나만 출력하세요.
{"answer":"...","recommended_ids":["..."]}`
	userPrompt := fmt.Sprintf("오늘 날짜: %s\n사용자 질문: %s\n전시 후보 JSON:\n%s", today, query, candidateJSON)
	content, err := callNVIDIAChat(ctx, settings, []nvidiaChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, 700)
	if err != nil {
		return nvidiaCuration{}, err
	}
	curation, err := parseNVIDIACuration(content)
	if err != nil {
		return nvidiaCuration{}, err
	}
	curation.RecommendedIDs = validRecommendedIDs(curation.RecommendedIDs, posts, 12)
	if len(curation.RecommendedIDs) == 0 {
		return nvidiaCuration{}, errors.New("NVIDIA returned no valid exhibition IDs")
	}
	curation.Answer = strings.TrimSpace(curation.Answer)
	if curation.Answer == "" {
		curation.Answer = "질문과 가까운 전시를 추천 순서대로 모았습니다."
	}
	return curation, nil
}

func callNVIDIAChat(ctx context.Context, settings nvidiaAISettings, messages []nvidiaChatMessage, maxTokens int) (string, error) {
	return callNVIDIAChatAtEndpoint(ctx, nvidiaAPIEndpoint, settings, messages, maxTokens)
}

func callNVIDIAChatAtEndpoint(ctx context.Context, endpoint string, settings nvidiaAISettings, messages []nvidiaChatMessage, maxTokens int) (string, error) {
	settings = normalizeNVIDIAAISettings(settings)
	payload, err := json.Marshal(nvidiaChatRequest{
		Model: settings.Model, Messages: messages, Temperature: 0.2, MaxTokens: maxTokens, Stream: false,
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

func parseNVIDIACuration(content string) (nvidiaCuration, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return nvidiaCuration{}, errors.New("NVIDIA response was not JSON")
	}
	var curation nvidiaCuration
	if err := json.Unmarshal([]byte(content[start:end+1]), &curation); err != nil {
		return nvidiaCuration{}, errors.New("NVIDIA curation JSON was invalid")
	}
	return curation, nil
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
