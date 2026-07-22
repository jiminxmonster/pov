package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const seoulOpenDataSource = "서울특별시 서울 열린데이터광장 · 서울문화포털"
const kcisaOpenDataSource = "문화체육관광부 · 한국문화정보원 문화공공데이터광장"

type seoulCulturalEventResponse struct {
	Result struct {
		Code    string `json:"CODE"`
		Message string `json:"MESSAGE"`
	} `json:"RESULT"`
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

type kcisaExhibition struct {
	Title           string `json:"TITLE" xml:"TITLE"`
	Institution     string `json:"CNTC_INSTT_NM" xml:"CNTC_INSTT_NM"`
	CollectedDate   string `json:"COLLECTED_DATE" xml:"COLLECTED_DATE"`
	IssuedDate      string `json:"ISSUED_DATE" xml:"ISSUED_DATE"`
	Description     string `json:"DESCRIPTION" xml:"DESCRIPTION"`
	ImageObject     string `json:"IMAGE_OBJECT" xml:"IMAGE_OBJECT"`
	LocalID         string `json:"LOCAL_ID" xml:"LOCAL_ID"`
	Homepage        string `json:"URL" xml:"URL"`
	ViewCount       string `json:"VIEW_COUNT" xml:"VIEW_COUNT"`
	SubDescription  string `json:"SUB_DESCRIPTION" xml:"SUB_DESCRIPTION"`
	BookingGuide    string `json:"SPATIAL_COVERAGE" xml:"SPATIAL_COVERAGE"`
	EventSite       string `json:"EVENT_SITE" xml:"EVENT_SITE"`
	Genre           string `json:"GENRE" xml:"GENRE"`
	Duration        string `json:"DURATION" xml:"DURATION"`
	NumberPages     string `json:"NUMBER_PAGES" xml:"NUMBER_PAGES"`
	TableOfContents string `json:"TABLE_OF_CONTENTS" xml:"TABLE_OF_CONTENTS"`
	Author          string `json:"AUTHOR" xml:"AUTHOR"`
	ContactPoint    string `json:"CONTACT_POINT" xml:"CONTACT_POINT"`
	Actor           string `json:"ACTOR" xml:"ACTOR"`
	Contributor     string `json:"CONTRIBUTOR" xml:"CONTRIBUTOR"`
	Audience        string `json:"AUDIENCE" xml:"AUDIENCE"`
	Charge          string `json:"CHARGE" xml:"CHARGE"`
	Period          string `json:"PERIOD" xml:"PERIOD"`
	ExhibitionHours string `json:"EVENT_PERIOD" xml:"EVENT_PERIOD"`
}

type kcisaExhibitionItems struct {
	Item []kcisaExhibition `json:"item" xml:"item"`
}

func (items *kcisaExhibitionItems) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '[' {
		return json.Unmarshal(trimmed, &items.Item)
	}
	var wrapped struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err != nil {
		return err
	}
	item := bytes.TrimSpace(wrapped.Item)
	if len(item) == 0 || bytes.Equal(item, []byte("null")) {
		return nil
	}
	if item[0] == '[' {
		return json.Unmarshal(item, &items.Item)
	}
	var single kcisaExhibition
	if err := json.Unmarshal(item, &single); err != nil {
		return err
	}
	items.Item = []kcisaExhibition{single}
	return nil
}

type kcisaAPIResponse struct {
	Header struct {
		ResultCode string `json:"resultCode" xml:"resultCode"`
		ResultMsg  string `json:"resultMsg" xml:"resultMsg"`
	} `json:"header" xml:"header"`
	Body struct {
		Items kcisaExhibitionItems `json:"items" xml:"items"`
	} `json:"body" xml:"body"`
}

type kcisaAPIErrorResponse struct {
	Message string `json:"message"`
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
	go func() {
		syncNow := func() {
			seoulContext, cancelSeoul := context.WithTimeout(ctx, 20*time.Second)
			count, err := s.syncSeoulExhibitions(seoulContext)
			cancelSeoul()
			if err != nil {
				log.Printf("서울시 공공 전시 데이터 동기화 실패: %v", err)
			} else {
				log.Printf("서울시 공공 전시 데이터 %d건 동기화", count)
			}

			settings, _, settingsErr := s.loadKCISADataSettings(ctx)
			if settingsErr != nil {
				log.Printf("문화공공데이터 설정 확인 실패: %v", settingsErr)
				return
			}
			if !validKCISADataKey(settings.APIKey) {
				return
			}
			kcisaContext, cancelKCISA := context.WithTimeout(ctx, 30*time.Second)
			count, err = s.syncKCISAExhibitionsWithSettings(kcisaContext, settings)
			cancelKCISA()
			if err != nil {
				log.Printf("문화공공데이터 통합 전시 동기화 실패: %v", err)
				return
			}
			log.Printf("문화공공데이터 통합 전시 %d건 동기화", count)
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

func (s *Server) syncKCISAExhibitions(ctx context.Context) (int, error) {
	settings, _, err := s.loadKCISADataSettings(ctx)
	if err != nil {
		return 0, err
	}
	return s.syncKCISAExhibitionsWithSettings(ctx, settings)
}

func (s *Server) syncKCISAExhibitionsWithSettings(ctx context.Context, settings publicDataSettings) (int, error) {
	settings = normalizeKCISADataSettings(settings)
	if !validKCISADataKey(settings.APIKey) {
		return 0, errors.New("문화공공데이터 서비스키가 설정되지 않았습니다")
	}

	endpoint, err := url.Parse(strings.TrimSpace(s.config.KCISAOpenDataURL))
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return 0, errors.New("문화공공데이터 API 주소가 올바르지 않습니다")
	}
	query := endpoint.Query()
	query.Set("serviceKey", settings.APIKey)
	query.Set("numOfRows", strconv.Itoa(settings.Limit))
	query.Set("pageNo", "1")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "POV-Exhibition-Map/1.0")

	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return 0, errors.New("문화공공데이터 API에 연결하지 못했습니다")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, decodeKCISAHTTPError(response.StatusCode, io.LimitReader(response.Body, 16<<10))
	}

	payload, err := decodeKCISAResponse(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return 0, errors.New("문화공공데이터 API 응답을 읽지 못했습니다")
	}
	if payload.Header.ResultCode != "" && payload.Header.ResultCode != "0000" && payload.Header.ResultCode != "00" {
		return 0, fmt.Errorf("문화공공데이터 API 오류: %s", strings.TrimSpace(payload.Header.ResultMsg))
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	count := 0
	for _, event := range payload.Body.Items.Item {
		exhibition, ok := convertKCISAEvent(event)
		if !ok {
			continue
		}
		metadataBytes, _ := json.Marshal(exhibition.Metadata)
		_, err := tx.Exec(ctx, `
			INSERT INTO posts (slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type, published_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'published', 'kcisa-open-data', NOW())
			ON CONFLICT (slug) DO UPDATE SET
				title = EXCLUDED.title,
				body_markdown = EXCLUDED.body_markdown,
				metadata = EXCLUDED.metadata,
				address = EXCLUDED.address,
				latitude = EXCLUDED.latitude,
				longitude = EXCLUDED.longitude,
				image_url = EXCLUDED.image_url,
				status = 'published',
				source_type = 'kcisa-open-data',
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

func decodeKCISAHTTPError(statusCode int, reader io.Reader) error {
	data, _ := io.ReadAll(reader)
	var payload kcisaAPIErrorResponse
	if json.Unmarshal(data, &payload) == nil {
		if message := cleanKCISAText(payload.Message); message != "" {
			return fmt.Errorf("문화공공데이터 API 응답 코드 %d: %s", statusCode, message)
		}
	}
	return fmt.Errorf("문화공공데이터 API 응답 코드 %d", statusCode)
}

func decodeKCISAResponse(reader io.Reader) (kcisaAPIResponse, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return kcisaAPIResponse{}, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return kcisaAPIResponse{}, errors.New("empty response")
	}
	if trimmed[0] == '<' {
		var payload kcisaAPIResponse
		if err := xml.Unmarshal(trimmed, &payload); err != nil {
			return kcisaAPIResponse{}, err
		}
		return payload, nil
	}
	var wrapped struct {
		Response kcisaAPIResponse `json:"response"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err != nil {
		return kcisaAPIResponse{}, err
	}
	if wrapped.Response.Header.ResultCode != "" || wrapped.Response.Body.Items.Item != nil {
		return wrapped.Response, nil
	}
	var direct kcisaAPIResponse
	if err := json.Unmarshal(trimmed, &direct); err != nil {
		return kcisaAPIResponse{}, err
	}
	return direct, nil
}

func (s *Server) syncSeoulExhibitions(ctx context.Context) (int, error) {
	settings, _, err := s.loadPublicDataSettings(ctx)
	if err != nil {
		return 0, err
	}
	return s.syncSeoulExhibitionsWithSettings(ctx, settings)
}

func (s *Server) syncSeoulExhibitionsWithSettings(ctx context.Context, settings publicDataSettings) (int, error) {
	settings = normalizePublicDataSettings(settings)
	if !validPublicDataKey(settings.APIKey) {
		return 0, errors.New("공공데이터 인증키가 설정되지 않았습니다")
	}

	endpoint := fmt.Sprintf("%s/%s/json/culturalEventInfo/1/%d/%s/",
		strings.TrimRight(s.config.SeoulOpenDataURL, "/"),
		url.PathEscape(settings.APIKey),
		settings.Limit,
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
	decoder := json.NewDecoder(io.LimitReader(response.Body, 32<<20))
	if err := decoder.Decode(&payload); err != nil {
		return 0, errors.New("공공데이터 응답을 읽지 못했습니다")
	}
	if payload.Result.Code != "" && payload.Result.Code != "INFO-000" {
		return 0, fmt.Errorf("공공데이터 API 오류: %s", payload.Result.Message)
	}
	if payload.CulturalEventInfo.Result.Code != "INFO-000" {
		return 0, fmt.Errorf("공공데이터 API 오류: %s", payload.CulturalEventInfo.Result.Message)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	count := 0
	for _, event := range payload.CulturalEventInfo.Rows {
		exhibition, ok := convertSeoulEvent(event)
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

func convertSeoulEvent(event seoulCulturalEvent) (publicExhibition, bool) {
	title := strings.TrimSpace(event.Title)
	place := strings.TrimSpace(event.Place)
	latitude, latErr := strconv.ParseFloat(strings.TrimSpace(event.Latitude), 64)
	longitude, lonErr := strconv.ParseFloat(strings.TrimSpace(event.Longitude), 64)
	if title == "" || place == "" || latErr != nil || lonErr != nil || latitude < 33 || latitude > 39 || longitude < 124 || longitude > 132 {
		return publicExhibition{}, false
	}

	endDate := datePart(event.EndDate)

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

var kcisaHTMLTagPattern = regexp.MustCompile(`<[^>]+>`)
var kcisaDatePattern = regexp.MustCompile(`(?:(\d{4})\s*(?:[./-]|년)\s*(\d{1,2})\s*(?:[./-]|월)\s*(\d{1,2})|\b(\d{4})(\d{2})(\d{2})\b)`)

func convertKCISAEvent(event kcisaExhibition) (publicExhibition, bool) {
	title := cleanKCISAText(event.Title)
	institution := cleanKCISAText(event.Institution)
	place := cleanKCISAText(event.EventSite)
	if place == "" {
		place = institution
	}
	if title == "" || place == "" {
		return publicExhibition{}, false
	}

	period := cleanKCISAText(event.Period)
	startDate, endDate := kcisaPeriodDates(period)
	author := cleanKCISAText(event.Author)
	if author == "" {
		author = cleanKCISAText(event.Actor)
	}
	description := strings.Join(nonEmpty(
		cleanKCISAText(event.Description),
		cleanKCISAText(event.SubDescription),
		prefixedValue("연계기관", cleanKCISAText(event.Institution)),
		prefixedValue("장르", cleanKCISAText(event.Genre)),
		prefixedValue("관람시간", cleanKCISAText(event.Duration)),
		prefixedValue("운영시간", cleanKCISAText(event.ExhibitionHours)),
		prefixedValue("관람대상", cleanKCISAText(event.Audience)),
		prefixedValue("전시품 정보", cleanKCISAText(event.NumberPages)),
		prefixedValue("예매안내", cleanKCISAText(event.BookingGuide)),
		prefixedValue("안내 및 유의사항", cleanKCISAText(event.TableOfContents)),
		prefixedValue("문의", cleanKCISAText(event.ContactPoint)),
		prefixedValue("주최·후원", cleanKCISAText(event.Contributor)),
		"공공데이터 출처: "+kcisaOpenDataSource,
		prefixedValue("상세보기", safeHTTPURL(event.Homepage)),
	), "\n")

	values := map[string]string{
		"전시명":      title,
		"작가(작가소개)": author,
		"관람료":      cleanKCISAText(event.Charge),
		"전시기간":     period,
		"장소":       place,
		"전시내용":     description,
	}
	metadata := map[string]string{
		"전시명":      title,
		"작가(작가소개)": author,
		"관람료":      cleanKCISAText(event.Charge),
		"전시기간":     period,
		"전시시작일":    startDate,
		"전시종료일":    endDate,
		"장소":       place,
		"전시내용":     description,
		"연계기관":     institution,
		"장르":       cleanKCISAText(event.Genre),
		"관람시간":     cleanKCISAText(event.Duration),
		"운영시간":     cleanKCISAText(event.ExhibitionHours),
		"공공데이터 출처": kcisaOpenDataSource,
		"원문 링크":    safeHTTPURL(event.Homepage),
		"지도표시":     "아니오",
	}

	return publicExhibition{
		Slug:         kcisaEventSlug(event),
		Title:        title,
		BodyMarkdown: renderExhibitionTemplate(values),
		Metadata:     metadata,
		Address:      place,
		Latitude:     0,
		Longitude:    0,
		ImageURL:     safeHTTPURL(event.ImageObject),
	}, true
}

func cleanKCISAText(value string) string {
	value = html.UnescapeString(strings.TrimSpace(value))
	value = kcisaHTMLTagPattern.ReplaceAllString(value, " ")
	value = strings.ReplaceAll(value, "\u00a0", " ")
	return strings.Join(strings.Fields(value), " ")
}

func kcisaPeriodDates(value string) (string, string) {
	matches := kcisaDatePattern.FindAllStringSubmatch(value, -1)
	dates := make([]string, 0, len(matches))
	for _, match := range matches {
		year, month, day := match[1], match[2], match[3]
		if year == "" {
			year, month, day = match[4], match[5], match[6]
		}
		monthNumber, monthErr := strconv.Atoi(month)
		dayNumber, dayErr := strconv.Atoi(day)
		if monthErr != nil || dayErr != nil || monthNumber < 1 || monthNumber > 12 || dayNumber < 1 || dayNumber > 31 {
			continue
		}
		dates = append(dates, fmt.Sprintf("%s-%02d-%02d", year, monthNumber, dayNumber))
	}
	if len(dates) == 0 {
		return "", ""
	}
	return dates[0], dates[len(dates)-1]
}

func kcisaEventSlug(event kcisaExhibition) string {
	identity := strings.Join([]string{
		cleanKCISAText(event.Institution),
		cleanKCISAText(event.LocalID),
		cleanKCISAText(event.Title),
		cleanKCISAText(event.Period),
	}, "|")
	sum := sha256.Sum256([]byte(identity))
	return "kcisa-exhibition-" + hex.EncodeToString(sum[:8])
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
