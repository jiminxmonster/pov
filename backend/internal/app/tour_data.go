package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const tourOpenDataSource = "한국관광공사 · 국문 관광정보 서비스"

type tourEvent struct {
	ContentID     string `json:"contentid"`
	Title         string `json:"title"`
	Address       string `json:"addr1"`
	AddressExtra  string `json:"addr2"`
	StartDate     string `json:"eventstartdate"`
	EndDate       string `json:"eventenddate"`
	Longitude     string `json:"mapx"`
	Latitude      string `json:"mapy"`
	ImageURL      string `json:"firstimage"`
	Category      string `json:"cat3"`
	CopyrightType string `json:"cpyrhtDivCd"`
	ProgressType  string `json:"progresstype"`
	Telephone     string `json:"tel"`
}

type tourEventItems struct {
	Item []tourEvent `json:"item"`
}

func (items *tourEventItems) UnmarshalJSON(data []byte) error {
	var wrapped struct {
		Item json.RawMessage `json:"item"`
	}
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte(`""`)) || bytes.Equal(trimmed, []byte("[]")) {
		return nil
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return err
	}
	item := bytes.TrimSpace(wrapped.Item)
	if len(item) == 0 || bytes.Equal(item, []byte("null")) {
		return nil
	}
	if item[0] == '[' {
		return json.Unmarshal(item, &items.Item)
	}
	var single tourEvent
	if err := json.Unmarshal(item, &single); err != nil {
		return err
	}
	items.Item = []tourEvent{single}
	return nil
}

type tourAPIResponse struct {
	Response struct {
		Header struct {
			ResultCode string `json:"resultCode"`
			ResultMsg  string `json:"resultMsg"`
		} `json:"header"`
		Body struct {
			TotalCount int            `json:"totalCount"`
			Items      tourEventItems `json:"items"`
		} `json:"body"`
	} `json:"response"`
	Error struct {
		Header struct {
			Message string `json:"errMsg"`
		} `json:"cmmMsgHeader"`
	} `json:"OpenAPI_ServiceResponse"`
}

func decodeTourAPIResponse(reader io.Reader) (tourAPIResponse, error) {
	var payload tourAPIResponse
	err := json.NewDecoder(io.LimitReader(reader, 8<<20)).Decode(&payload)
	if err != nil {
		return payload, err
	}
	if payload.Error.Header.Message != "" {
		return payload, errors.New("관광공사 API 인증 또는 권한 오류")
	}
	if payload.Response.Header.ResultCode != "0000" && payload.Response.Header.ResultCode != "00" {
		return payload, fmt.Errorf("관광공사 API 응답 오류: %s", cleanKCISAText(payload.Response.Header.ResultMsg))
	}
	return payload, nil
}

func (s *Server) syncTourExhibitions(ctx context.Context) (int, error) {
	settings, _, err := s.loadTourDataSettings(ctx)
	if err != nil {
		return 0, err
	}
	return s.syncTourExhibitionsWithSettings(ctx, settings)
}

func (s *Server) syncTourExhibitionsWithSettings(ctx context.Context, settings publicDataSettings) (int, error) {
	settings = normalizeTourDataSettings(settings)
	if !validKCISADataKey(settings.APIKey) {
		return 0, errors.New("관광공사 API 인증키가 설정되지 않았습니다")
	}
	base, err := url.Parse(strings.TrimSpace(s.config.TourOpenDataURL))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return 0, errors.New("관광공사 API 주소가 올바르지 않습니다")
	}
	client := &http.Client{Timeout: 12 * time.Second}
	pageSize := min(settings.Limit, 500)
	collected := make([]publicExhibition, 0, settings.Limit)
	seen := make(map[string]bool)
	for page := 1; page <= 10 && len(collected) < settings.Limit; page++ {
		endpoint := *base
		query := endpoint.Query()
		query.Set("serviceKey", settings.APIKey)
		query.Set("MobileOS", "ETC")
		query.Set("MobileApp", "POV")
		query.Set("_type", "json")
		query.Set("arrange", "C")
		query.Set("cat3", "A02080500") // 한국관광공사 행사 세부 분류: 전시회.
		query.Set("eventStartDate", time.Now().In(time.FixedZone("KST", 9*3600)).AddDate(-1, 0, 0).Format("20060102"))
		query.Set("eventEndDate", time.Now().In(time.FixedZone("KST", 9*3600)).AddDate(1, 0, 0).Format("20060102"))
		query.Set("numOfRows", strconv.Itoa(pageSize))
		query.Set("pageNo", strconv.Itoa(page))
		endpoint.RawQuery = query.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return 0, err
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "POV-Exhibition-Map/1.0")
		response, err := client.Do(request)
		if err != nil {
			return 0, errors.New("관광공사 API에 연결하지 못했습니다")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			return 0, fmt.Errorf("관광공사 API 응답 코드 %d", response.StatusCode)
		}
		payload, decodeErr := decodeTourAPIResponse(response.Body)
		response.Body.Close()
		if decodeErr != nil {
			return 0, decodeErr
		}
		for _, event := range payload.Response.Body.Items.Item {
			exhibition, ok := convertTourEvent(event)
			if ok && !seen[exhibition.Slug] {
				collected = append(collected, exhibition)
				seen[exhibition.Slug] = true
				if len(collected) == settings.Limit {
					break
				}
			}
		}
		if len(payload.Response.Body.Items.Item) < pageSize || (payload.Response.Body.TotalCount > 0 && page*pageSize >= payload.Response.Body.TotalCount) {
			break
		}
	}
	if len(collected) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	count := 0
	for _, exhibition := range collected {
		metadataBytes, _ := json.Marshal(exhibition.Metadata)
		result, err := tx.Exec(ctx, `
			INSERT INTO posts (slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type, published_at)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, 'published', 'tour-open-data', NOW()
			WHERE NOT EXISTS (
				SELECT 1 FROM posts
				WHERE slug <> $1 AND title = $2 AND metadata->>'전시시작일' = $9
				  AND abs(latitude - $6) < 0.002 AND abs(longitude - $7) < 0.002
				  AND source_type IN ('seoul-open-data', 'kcisa-open-data')
			)
			ON CONFLICT (slug) DO UPDATE SET
				title = EXCLUDED.title, body_markdown = EXCLUDED.body_markdown,
				metadata = EXCLUDED.metadata, address = EXCLUDED.address,
				latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude,
				image_url = EXCLUDED.image_url, status = 'published',
				updated_at = NOW()
		`, exhibition.Slug, exhibition.Title, exhibition.BodyMarkdown, metadataBytes,
			exhibition.Address, exhibition.Latitude, exhibition.Longitude, exhibition.ImageURL, exhibition.Metadata["전시시작일"])
		if err != nil {
			return 0, err
		}
		count += int(result.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func tourDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 8 {
		return ""
	}
	parsed, err := time.Parse("20060102", value)
	if err != nil {
		return ""
	}
	return parsed.Format("2006-01-02")
}

func convertTourEvent(event tourEvent) (publicExhibition, bool) {
	id := strings.TrimSpace(event.ContentID)
	title := cleanKCISAText(event.Title)
	if id == "" || title == "" || (event.Category != "" && event.Category != "A02080500") || strings.Contains(event.ProgressType, "취소") {
		return publicExhibition{}, false
	}
	start, end := tourDate(event.StartDate), tourDate(event.EndDate)
	if start == "" || end == "" {
		return publicExhibition{}, false
	}
	address := strings.TrimSpace(strings.Join(nonEmpty(cleanKCISAText(event.Address), cleanKCISAText(event.AddressExtra)), " "))
	lat, latErr := strconv.ParseFloat(strings.TrimSpace(event.Latitude), 64)
	lon, lonErr := strconv.ParseFloat(strings.TrimSpace(event.Longitude), 64)
	if latErr != nil || lonErr != nil || lat < 33 || lat > 39 || lon < 124 || lon > 132 {
		lat, lon = 0, 0
	}
	period := start + " ~ " + end
	sourceURL := "https://data.visitkorea.or.kr/page/" + url.PathEscape(id)
	description := strings.Join(nonEmpty(
		prefixedValue("문의", cleanKCISAText(event.Telephone)),
		"공공데이터 출처: "+tourOpenDataSource,
		"상세보기: "+sourceURL,
	), "\n")
	values := map[string]string{"전시명": title, "전시기간": period, "장소": address, "전시내용": description}
	metadata := map[string]string{
		"전시명": title, "전시기간": period, "전시시작일": start, "전시종료일": end,
		"장소": address, "전시내용": description, "공공데이터 출처": tourOpenDataSource,
		"원문 링크": sourceURL, "이미지 이용유형": strings.TrimSpace(event.CopyrightType),
	}
	return publicExhibition{
		Slug: "tour-exhibition-" + id, Title: title, BodyMarkdown: renderExhibitionTemplate(values),
		Metadata: metadata, Address: address, Latitude: lat, Longitude: lon, ImageURL: safeHTTPURL(event.ImageURL),
	}, true
}
