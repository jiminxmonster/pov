package app

import (
	"strings"
	"testing"
	"time"
)

func TestVenueResolutionUsesActualExhibitionLocation(t *testing.T) {
	tests := []struct {
		name, place, institution, link, want string
	}{
		{"regional supplier", "국립중앙박물관", "국립중앙박물관", "https://gyeongju.museum.go.kr/kor/html/sub02/0202.html", "국립경주박물관"},
		{"regional room", "특별전시실", "국립대구박물관", "https://daegu.museum.go.kr/exhibition", "국립대구박물관"},
		{"travelling venue overrides organiser", "국립세계문자박물관 1층 특별전시실", "국립한글박물관", "https://www.hangeul.go.kr/exhibition/123", "국립세계문자박물관"},
		{"gallery branch", "과천", "국립현대미술관", "https://www.mmca.go.kr/exhibitions/1", "국립현대미술관 과천"},
		{"English venue", "Yongin Poeun Art Gallery", "국립한글박물관", "https://www.hangeul.go.kr/exhibition/456", "용인포은아트갤러리"},
		{"overseas exhibition not in organising museum", "중국 상하이 박물관 동관 특별전시실 2", "국립경주박물관", "https://gyeongju.museum.go.kr/exhibition", "상하이박물관 동관"},
		{"unspecified multiple venues", "고흥분청문화박물관 등 8개 지역 공립박물관", "국립중앙박물관", "https://www.museum.go.kr/exhibition", ""},
		{"unknown venue", "알 수 없는 미술관 기획전시실", "국립한글박물관", "https://www.hangeul.go.kr/exhibition", ""},
		{"invalid location", "test", "국립한글박물관", "https://www.hangeul.go.kr/exhibition", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			venue, ok := resolveExhibitionVenue(tt.place, map[string]string{"연계기관": tt.institution, "원문 링크": tt.link})
			if venue.Name != tt.want || ok != (tt.want != "") {
				t.Fatalf("got venue=%q resolved=%v; want %q", venue.Name, ok, tt.want)
			}
		})
	}
}

func TestKCISAVenueEnrichmentSurvivesFutureSync(t *testing.T) {
	event := kcisaExhibition{Title: "특별전", Institution: "국립중앙박물관", Homepage: "https://chuncheon.museum.go.kr/exhibition/123"}
	exhibition, ok := convertKCISAEvent(event)
	if !ok || exhibition.Latitude == 0 || exhibition.Longitude == 0 || exhibition.Metadata["지도표시"] != "예" {
		t.Fatalf("new imports must have verified venue coordinates: %#v", exhibition)
	}
	if exhibition.Address != "국립춘천박물관" || exhibition.Metadata["원문 장소"] != "국립중앙박물관" || !strings.Contains(exhibition.BodyMarkdown, "장소:\n국립춘천박물관") {
		t.Fatalf("location must identify the branch and preserve original source: %#v", exhibition)
	}
	if enrichExhibitionLocation(&exhibition) {
		t.Fatal("already verified coordinates must be preserved")
	}
}

func TestPublicMapIncludesAllIndexLocationsButAIExcludesEnded(t *testing.T) {
	now := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	posts := []Post{
		{ID: "active", Latitude: 37.5, Longitude: 127, Metadata: map[string]string{"전시종료일": "2026-12-01"}},
		{ID: "recent-ended", Latitude: 37.5, Longitude: 127, Metadata: map[string]string{"전시종료일": "2026-09-20"}},
		{ID: "past-knowledge", Latitude: 37.5, Longitude: 127, Metadata: map[string]string{"전시종료일": "2026-01-01"}},
		{ID: "unresolved", Metadata: map[string]string{"지도표시": "아니오"}},
	}
	visible := publicMapExhibitions(posts, now, 100)
	if len(visible) != 2 || visible[0].ID != "active" || visible[1].ID != "recent-ended" {
		t.Fatalf("map must include all geolocated public index posts: %#v", visible)
	}
	recommendations := currentMapExhibitions(posts, now, 100)
	if len(recommendations) != 1 || recommendations[0].ID != "active" {
		t.Fatalf("AI must not recommend ended exhibitions for a visit: %#v", recommendations)
	}
}
