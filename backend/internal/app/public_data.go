package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const seoulOpenDataSource = "서울특별시 서울 열린데이터광장 · 서울문화포털"

type seoulCulturalEventResponse struct {
	CulturalEventInfo struct {
		Total  int `json:"list_total_count"`
		Result struct {
			Code    string `json:"CODE"`
			Message string `json:"MESSAGE"`
		} `json:"RESULT"`
		Rows []seoulCulturalEvent `json:"row"`
	} `json:"culturalEventInfo"`
}

type seoulCulturalEvent struct {
	CodeName  string `json:"CODENAME"`
	GuName    string `json:"GUNAME"`
	Title     string `json:"TITLE"`
	Date      string `json:"DATE"`
	Place     string `json:"PLACE"`
	Organizer string `json:"ORG_NAME"`
	Audience  string `json:"USE_TRGT"`
	Fee       string `json:"USE_FEE"`
	Inquiry   string `json:"INQUIRY"`
	Program   string `json:"PROGRAM"`
	Extra     string `json:"ETC_DESC"`
	ImageURL  string `json:"MAIN_IMG"`
	StartDate string `json:"STRTDATE"`
	EndDate   string `json:"END_DATE"`
	Longitude string `json:"LOT"`
	Latitude  string `json:"LAT"`
	IsFree    string `json:"IS_FREE"`
	Homepage  string `json:"HMPG_ADDR"`
	ShowTime  string `json:"PRO_TIME"`
}

type publicExhibition struct {
	Slug         string
	Title        string
	BodyMarkdown string
	Metadata     map[string]string
	Address      string
	Latitude     float64
	Longitude    float64
	ImageURL     string
}

func (s *Server) StartPublicDataSync(ctx context.Context) {
	if strings.TrimSpace(s.config.SeoulOpenDataKey) == "" {
		return
	}
	go func() {
		syncNow := func() {
			syncContext, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			count, err := s.syncSeoulExhibitions(syncContext)
			if err != nil {
				log.Printf("서울시 공공 전시 데이터 동기화 실패: %v", err)
				return
			}
			log.Printf("서울시 공공 전시 데이터 %d건 동기화", count)
		}

		syncNow()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncNow()
			}
		}
	}()
}

func (s *Server) syncSeoulExhibitions(ctx context.Context) (int, error) {
	limit := s.config.SeoulOpenDataLimit
	if limit < 1 {
		limit = 5
	}
	if strings.EqualFold(s.config.SeoulOpenDataKey, "sample") && limit > 5 {
		limit = 5
	}
	if limit > 1000 {
		limit = 1000
	}

	endpoint := fmt.Sprintf("%s/%s/json/culturalEventInfo/1/%d/%s/",
		strings.TrimRight(s.config.SeoulOpenDataURL, "/"),
		url.PathEscape(s.config.SeoulOpenDataKey),
		limit,
		url.PathEscape("전시"),
	)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("User-Agent", "POV-Exhibition-Map/1.0")

	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return 0, errors.New("공공데이터 API에 연결하지 못했습니다")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("공공데이터 API 응답 코드 %d", response.StatusCode)
	}

	var payload seoulCulturalEventResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(&payload); err != nil {
		return 0, errors.New("공공데이터 응답을 읽지 못했습니다")
	}
	if payload.CulturalEventInfo.Result.Code != "INFO-000" {
		return 0, fmt.Errorf("공공데이터 API 오류: %s", payload.CulturalEventInfo.Result.Message)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE posts
		SET status = 'archived', updated_at = NOW()
		WHERE source_type = 'seoul-open-data'
			AND status = 'published'
			AND metadata->>'전시종료일' ~ '^\d{4}-\d{2}-\d{2}$'
			AND (metadata->>'전시종료일')::date < CURRENT_DATE
	`); err != nil {
		return 0, err
	}

	count := 0
	for _, event := range payload.CulturalEventInfo.Rows {
		exhibition, ok := convertSeoulEvent(event, time.Now())
		if !ok {
			continue
		}
		metadataBytes, _ := json.Marshal(exhibition.Metadata)
		_, err := tx.Exec(ctx, `
			INSERT INTO posts (slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type, published_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'published', 'seoul-open-data', NOW())
			ON CONFLICT (slug) DO UPDATE SET
				title = EXCLUDED.title,
				body_markdown = EXCLUDED.body_markdown,
				metadata = EXCLUDED.metadata,
				address = EXCLUDED.address,
				latitude = EXCLUDED.latitude,
				longitude = EXCLUDED.longitude,
				image_url = EXCLUDED.image_url,
				status = 'published',
				source_type = 'seoul-open-data',
				published_at = COALESCE(posts.published_at, NOW()),
				updated_at = NOW()
		`, exhibition.Slug, exhibition.Title, exhibition.BodyMarkdown, metadataBytes, exhibition.Address,
			exhibition.Latitude, exhibition.Longitude, exhibition.ImageURL)
		if err != nil {
			return 0, err
		}
		count++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func convertSeoulEvent(event seoulCulturalEvent, now time.Time) (publicExhibition, bool) {
	title := strings.TrimSpace(event.Title)
	place := strings.TrimSpace(event.Place)
	latitude, latErr := strconv.ParseFloat(strings.TrimSpace(event.Latitude), 64)
	longitude, lonErr := strconv.ParseFloat(strings.TrimSpace(event.Longitude), 64)
	if title == "" || place == "" || latErr != nil || lonErr != nil || latitude < 33 || latitude > 39 || longitude < 124 || longitude > 132 {
		return publicExhibition{}, false
	}

	endDate := datePart(event.EndDate)
	if parsedEnd, err := time.ParseInLocation("2006-01-02", endDate, now.Location()); err == nil && parsedEnd.Before(dayStart(now)) {
		return publicExhibition{}, false
	}

	fee := strings.TrimSpace(event.Fee)
	if fee == "" {
		fee = strings.TrimSpace(event.IsFree)
	}
	period := strings.ReplaceAll(strings.TrimSpace(event.Date), "~", " ~ ")
	address := strings.TrimSpace(strings.Join(nonEmpty("서울 "+strings.TrimSpace(event.GuName), place), " · "))
	description := strings.Join(nonEmpty(
		strings.TrimSpace(event.Program),
		strings.TrimSpace(event.Extra),
		prefixedValue("이용대상", event.Audience),
		prefixedValue("행사시간", event.ShowTime),
		prefixedValue("문의", event.Inquiry),
		"공공데이터 출처: "+seoulOpenDataSource,
		prefixedValue("상세보기", event.Homepage),
	), "\n")

	values := map[string]string{
		"전시명":      title,
		"작가(작가소개)": strings.TrimSpace(event.Organizer),
		"관람료":      fee,
		"전시기간":     period,
		"장소":       address,
		"전시내용":     description,
	}
	metadata := map[string]string{
		"전시명":      title,
		"작가(작가소개)": strings.TrimSpace(event.Organizer),
		"관람료":      fee,
		"전시기간":     period,
		"전시시작일":    datePart(event.StartDate),
		"전시종료일":    endDate,
		"장소":       address,
		"전시내용":     description,
		"자치구":      strings.TrimSpace(event.GuName),
		"분류":       strings.TrimSpace(event.CodeName),
		"공공데이터 출처": seoulOpenDataSource,
		"원문 링크":    strings.TrimSpace(event.Homepage),
	}

	return publicExhibition{
		Slug:         seoulEventSlug(event),
		Title:        title,
		BodyMarkdown: renderExhibitionTemplate(values),
		Metadata:     metadata,
		Address:      address,
		Latitude:     latitude,
		Longitude:    longitude,
		ImageURL:     safeHTTPURL(event.ImageURL),
	}, true
}

func renderExhibitionTemplate(values map[string]string) string {
	var body strings.Builder
	for _, label := range templateLabels() {
		body.WriteString(label)
		body.WriteString(":\n")
		body.WriteString(strings.TrimSpace(values[label]))
		body.WriteString("\n\n")
	}
	return strings.TrimSpace(body.String()) + "\n"
}

func seoulEventSlug(event seoulCulturalEvent) string {
	if parsed, err := url.Parse(strings.TrimSpace(event.Homepage)); err == nil {
		if code := parsed.Query().Get("cultcode"); code != "" {
			return "seoul-culture-" + code
		}
	}
	sum := sha256.Sum256([]byte(event.Title + "|" + event.Date + "|" + event.Place))
	return "seoul-culture-" + hex.EncodeToString(sum[:8])
}

func datePart(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}

func dayStart(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func prefixedValue(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + ": " + value
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func safeHTTPURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return parsed.String()
}
