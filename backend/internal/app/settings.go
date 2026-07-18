package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

const publicDataSettingName = "seoul_open_data"

var publicDataKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type publicDataSettings struct {
	APIKey string `json:"api_key"`
	Limit  int    `json:"limit"`
}

type publicDataSettingsResponse struct {
	Configured  bool   `json:"configured"`
	MaskedKey   string `json:"masked_key"`
	Limit       int    `json:"limit"`
	Storage     string `json:"storage"`
	SyncedCount int    `json:"synced_count,omitempty"`
	Message     string `json:"message,omitempty"`
}

func (s *Server) getPublicDataSettings(w http.ResponseWriter, r *http.Request) {
	settings, stored, err := s.loadPublicDataSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "인증키 설정을 불러오지 못했습니다")
		return
	}
	writeJSON(w, http.StatusOK, publicDataSettingsPayload(settings, stored))
}

func (s *Server) updatePublicDataSettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		APIKey string `json:"api_key"`
		Limit  int    `json:"limit"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}

	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		current, _, err := s.loadPublicDataSettings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "기존 인증키를 불러오지 못했습니다")
			return
		}
		apiKey = current.APIKey
	}
	if !validPublicDataKey(apiKey) {
		writeError(w, http.StatusBadRequest, "서울 열린데이터광장 인증키 형식을 확인해 주세요")
		return
	}
	settings := normalizePublicDataSettings(publicDataSettings{APIKey: apiKey, Limit: input.Limit})

	count, err := s.syncSeoulExhibitionsWithSettings(r.Context(), settings)
	if err != nil {
		writeError(w, http.StatusBadGateway, "인증키 또는 서울시 API 연결 상태를 확인해 주세요")
		return
	}
	if err := s.storePublicDataSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, "인증키를 저장하지 못했습니다")
		return
	}

	payload := publicDataSettingsPayload(settings, true)
	payload.SyncedCount = count
	payload.Message = "인증키를 저장하고 전시 데이터를 동기화했습니다."
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) syncPublicDataNow(w http.ResponseWriter, r *http.Request) {
	settings, stored, err := s.loadPublicDataSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "인증키 설정을 불러오지 못했습니다")
		return
	}
	count, err := s.syncSeoulExhibitionsWithSettings(r.Context(), settings)
	if err != nil {
		writeError(w, http.StatusBadGateway, "인증키 또는 서울시 API 연결 상태를 확인해 주세요")
		return
	}
	payload := publicDataSettingsPayload(settings, stored)
	payload.SyncedCount = count
	payload.Message = "공공 전시 데이터를 지금 동기화했습니다."
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) loadPublicDataSettings(ctx context.Context) (publicDataSettings, bool, error) {
	fallback := normalizePublicDataSettings(publicDataSettings{
		APIKey: strings.TrimSpace(s.config.SeoulOpenDataKey),
		Limit:  s.config.SeoulOpenDataLimit,
	})

	var encrypted []byte
	err := s.db.QueryRow(ctx, `SELECT value_encrypted FROM app_settings WHERE name = $1`, publicDataSettingName).Scan(&encrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		return fallback, false, nil
	}
	if err != nil {
		return publicDataSettings{}, false, err
	}
	plaintext, err := s.openSetting(encrypted)
	if err != nil {
		return publicDataSettings{}, true, err
	}
	var settings publicDataSettings
	if err := json.Unmarshal(plaintext, &settings); err != nil {
		return publicDataSettings{}, true, err
	}
	settings = normalizePublicDataSettings(settings)
	if settings.APIKey == "" {
		return fallback, false, nil
	}
	return settings, true, nil
}

func (s *Server) storePublicDataSettings(ctx context.Context, settings publicDataSettings) error {
	payload, err := json.Marshal(normalizePublicDataSettings(settings))
	if err != nil {
		return err
	}
	encrypted, err := s.sealSetting(payload)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO app_settings (name, value_encrypted)
		VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET value_encrypted = EXCLUDED.value_encrypted, updated_at = NOW()
	`, publicDataSettingName, encrypted)
	return err
}

func (s *Server) sealSetting(plaintext []byte) ([]byte, error) {
	return s.sealNamedSetting(publicDataSettingName, plaintext)
}

func (s *Server) sealNamedSetting(name string, plaintext []byte) ([]byte, error) {
	gcm, err := s.settingCipher()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, []byte(name)), nil
}

func (s *Server) openSetting(encrypted []byte) ([]byte, error) {
	return s.openNamedSetting(publicDataSettingName, encrypted)
}

func (s *Server) openNamedSetting(name string, encrypted []byte) ([]byte, error) {
	gcm, err := s.settingCipher()
	if err != nil {
		return nil, err
	}
	if len(encrypted) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted setting")
	}
	nonce, ciphertext := encrypted[:gcm.NonceSize()], encrypted[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, []byte(name))
}

func (s *Server) settingCipher() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(s.config.SessionSecret + "|pov-settings-v1"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func normalizePublicDataSettings(settings publicDataSettings) publicDataSettings {
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	if settings.Limit < 1 {
		settings.Limit = 5
	}
	if strings.EqualFold(settings.APIKey, "sample") && settings.Limit > 5 {
		settings.Limit = 5
	}
	if settings.Limit > 1000 {
		settings.Limit = 1000
	}
	return settings
}

func validPublicDataKey(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 4 && len(value) <= 128 && publicDataKeyPattern.MatchString(value)
}

func publicDataSettingsPayload(settings publicDataSettings, stored bool) publicDataSettingsResponse {
	storage := "environment"
	if stored {
		storage = "database"
	}
	return publicDataSettingsResponse{
		Configured: settings.APIKey != "",
		MaskedKey:  maskSecret(settings.APIKey),
		Limit:      settings.Limit,
		Storage:    storage,
	}
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	if len(runes) <= 4 {
		return strings.Repeat("•", len(runes))
	}
	if len(runes) <= 10 {
		return string(runes[:1]) + strings.Repeat("•", len(runes)-2) + string(runes[len(runes)-1:])
	}
	return string(runes[:4]) + strings.Repeat("•", 6) + string(runes[len(runes)-4:])
}
