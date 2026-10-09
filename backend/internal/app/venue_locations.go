package app

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// These are reviewed venue lookups, not guessed coordinates for each exhibition.
// Keep the OSM object URL so every location can be traced and corrected.
type exhibitionVenue struct {
	Name      string
	Latitude  float64
	Longitude float64
	SourceURL string
	Aliases   []string
}

var exhibitionVenues = []exhibitionVenue{
	{"국립중앙박물관", 37.5239530, 126.9803196, "https://www.openstreetmap.org/way/168364800", nil},
	{"국립경주박물관", 35.8289946, 129.2279662, "https://www.openstreetmap.org/way/573218888", nil},
	{"국립춘천박물관", 37.8637672, 127.7539316, "https://www.openstreetmap.org/way/475122832", nil},
	{"국립김해박물관", 35.242797, 128.873517, "https://gimhae.museum.go.kr/kr/html/sub01/0104.html", nil},
	{"국립대구박물관", 35.8458620, 128.6372143, "https://www.openstreetmap.org/way/1155812218", nil},
	{"국립청주박물관", 36.6492486, 127.5120937, "https://www.openstreetmap.org/node/368734079", nil},
	{"국립익산박물관", 36.0108264, 127.0274816, "https://www.openstreetmap.org/way/915341903", nil},
	{"국립제주박물관", 33.5135053, 126.5486899, "https://www.openstreetmap.org/way/369809426", nil},
	{"국립전주박물관", 35.8010280, 127.0897040, "https://www.openstreetmap.org/node/368733861", []string{"JEONJU NATIONAL MUSEUM"}},
	{"국립진주박물관", 35.1890527, 128.0767991, "https://www.openstreetmap.org/node/368734217", nil},
	{"국립광주박물관", 35.1889834, 126.8830858, "https://www.openstreetmap.org/way/469896846", nil},
	{"국립나주박물관", 34.9148163, 126.6589670, "https://www.openstreetmap.org/way/1181020506", nil},
	{"국립공주박물관", 36.4655287, 127.1122874, "https://www.openstreetmap.org/way/157482714", nil},
	{"국립부여박물관", 36.2763114, 126.9190510, "https://www.openstreetmap.org/way/366966605", nil},
	{"국립한글박물관", 37.5211497, 126.9809372, "https://www.openstreetmap.org/way/307680462", nil},
	{"국립현대미술관 서울", 37.5795533, 126.9804352, "https://www.openstreetmap.org/relation/14956894", []string{"국립현대미술관 서울관"}},
	{"국립현대미술관 과천", 37.4309964, 127.0196745, "https://www.openstreetmap.org/way/40791172", []string{"국립현대미술관 과천관"}},
	{"국립현대미술관 청주", 36.6553200, 127.4900794, "https://www.openstreetmap.org/relation/6676204", []string{"국립현대미술관 청주관"}},
	{"국립현대미술관 덕수궁", 37.5659183, 126.9737636, "https://www.openstreetmap.org/way/181574568", []string{"국립현대미술관 덕수궁관"}},
	{"예술의전당", 37.4797475, 127.0133947, "https://www.openstreetmap.org/way/606624825", nil},
	{"용인포은아트갤러리", 37.3256129, 127.1045887, "https://www.openstreetmap.org/way/364182044", []string{"Yongin Poeun Art Gallery"}},
	{"당진문예의전당", 36.8902344, 126.6378988, "https://www.openstreetmap.org/way/902160451", nil},
	{"김포아트빌리지 아트센터", 37.6465432, 126.6962112, "https://www.openstreetmap.org/way/657884965", []string{"김포아트빌리지"}},
	{"한국영상자료원", 37.5804134, 126.8895886, "https://www.openstreetmap.org/node/6484705968", nil},
	{"국립아시아문화전당", 35.1472744, 126.9208083, "https://www.openstreetmap.org/way/248192696", nil},
	{"대한민국역사박물관", 37.5739553, 126.9778057, "https://www.openstreetmap.org/way/141957171", []string{"National Museum of Korean Contemporary History"}},
	{"국립세계문자박물관", 37.3944310, 126.6382789, "https://www.openstreetmap.org/way/1035289826", nil},
	{"국립어린이청소년도서관", 37.5009614, 127.0296222, "https://www.openstreetmap.org/way/306374820", nil},
	{"국립민속박물관", 37.5808343, 126.9790431, "https://www.openstreetmap.org/relation/5501518", nil},
	{"제주돌문화공원", 33.4495978, 126.6587886, "https://www.openstreetmap.org/way/375490037", nil},
	{"오죽헌", 37.7785596, 128.8777766, "https://www.openstreetmap.org/way/657468634", []string{"오죽헌·시립박물관"}},
	{"박연문화관", 36.4985155, 127.2637806, "https://www.openstreetmap.org/way/1157036980", []string{"Sejong Culture and Tourism Foundation Park Yeon Culture Center"}},
	{"부산시민회관", 35.1388781, 129.0652905, "https://www.openstreetmap.org/way/498993949", nil},
	{"문화역서울284", 37.5559919, 126.9715457, "https://www.openstreetmap.org/way/759569673", []string{"Culture Station Seoul 284"}},
	{"김해시립김영원미술관", 35.2577806, 128.8742819, "https://www.openstreetmap.org/node/12536081097", []string{"Gimhae Kimyoungwon Museum of Art"}},
	{"구미시문화예술회관", 36.1181134, 128.3487276, "https://www.openstreetmap.org/way/1120918710", nil},
	{"베트남국립도서관", 21.0260138, 105.8492217, "https://www.openstreetmap.org/way/701882910", []string{"Vietnam National Library"}},
	{"주일한국문화원", 35.6872681, 139.7172469, "https://www.openstreetmap.org/way/391703259", []string{"도쿄 한국 문화원", "Korean Cultural Center, Gallery MI"}},
	{"주필리핀한국문화원", 14.5264972, 121.0427124, "https://www.openstreetmap.org/node/11891016876", []string{"Korean Cultural Center in the Philippines"}},
	{"주로스앤젤레스한국문화원", 34.0626106, -118.3484706, "https://www.openstreetmap.org/node/368167487", []string{"미국 LA문화원", "Korean Cultural Center Los Angeles"}},
	{"덴버미술관", 39.7373941, -104.9896237, "https://www.openstreetmap.org/way/35205743", []string{"미국 덴버박물관"}},
	{"상하이박물관 동관", 31.2221159, 121.5343570, "https://www.openstreetmap.org/way/1034667864", []string{"중국 상하이 박물관 동관"}},
}

var museumHosts = map[string]string{
	"www.museum.go.kr": "국립중앙박물관", "museum.go.kr": "국립중앙박물관",
	"gyeongju.museum.go.kr": "국립경주박물관", "chuncheon.museum.go.kr": "국립춘천박물관",
	"daegu.museum.go.kr": "국립대구박물관", "cheongju.museum.go.kr": "국립청주박물관",
	"gimhae.museum.go.kr": "국립김해박물관", "iksan.museum.go.kr": "국립익산박물관",
	"jeju.museum.go.kr": "국립제주박물관", "jeonju.museum.go.kr": "국립전주박물관",
	"jinju.museum.go.kr": "국립진주박물관", "gwangju.museum.go.kr": "국립광주박물관",
	"naju.museum.go.kr": "국립나주박물관", "gongju.museum.go.kr": "국립공주박물관",
	"buyeo.museum.go.kr": "국립부여박물관", "www.hangeul.go.kr": "국립한글박물관",
	"www.koreafilm.or.kr": "한국영상자료원", "www.acc.go.kr": "국립아시아문화전당",
	"www.much.go.kr": "대한민국역사박물관", "www.nlcy.go.kr": "국립어린이청소년도서관",
	"www.sac.or.kr": "예술의전당",
}

func venueByName(name string) (exhibitionVenue, bool) {
	for _, venue := range exhibitionVenues {
		if venue.Name == name {
			return venue, true
		}
	}
	return exhibitionVenue{}, false
}

func normalizeVenueText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}

func resolveExhibitionVenue(place string, metadata map[string]string) (exhibitionVenue, bool) {
	// Travelling exhibitions must not be pinned at their organising museum.
	for _, marker := range []string{"등 8개", "등 6개", "각 지역", "Local public museums"} {
		if strings.Contains(place, marker) {
			return exhibitionVenue{}, false
		}
	}
	normalized := normalizeVenueText(place)
	original, _ := url.Parse(metadata["원문 링크"])
	host := ""
	if original != nil {
		host = strings.ToLower(original.Hostname())
	}
	// KCISA often puts the central supplier in EVENT_SITE for regional museums.
	if normalized == "국립중앙박물관" {
		if name, ok := museumHosts[host]; ok {
			return venueByName(name)
		}
	}
	for _, venue := range exhibitionVenues {
		for _, alias := range append([]string{venue.Name}, venue.Aliases...) {
			if strings.Contains(normalized, normalizeVenueText(alias)) {
				return venue, true
			}
		}
	}
	if host == "www.mmca.go.kr" || metadata["연계기관"] == "국립현대미술관" {
		if normalized == "어린이미술관" {
			return venueByName("국립현대미술관 과천")
		}
		for _, branch := range []string{"서울", "과천", "청주", "덕수궁"} {
			if normalized == branch || strings.HasPrefix(normalized, branch+"관") {
				return venueByName("국립현대미술관 " + branch)
			}
		}
	}
	for _, external := range []string{"중국", "미국", "베트남", "도쿄", "주필리핀", "주상하이", "vietnam", "koreanculturalcenter", "gallerymi"} {
		if strings.Contains(normalized, external) {
			return exhibitionVenue{}, false
		}
	}
	if (strings.Contains(normalized, "박물관") && !strings.HasPrefix(normalized, "어린이박물관") && !strings.HasPrefix(normalized, "아시아문화박물관")) || strings.Contains(normalized, "미술관") {
		return exhibitionVenue{}, false
	}
	// Only an internal room description can fall back to the source's venue.
	// An unknown external venue must remain unresolved rather than acquire a fake pin.
	for _, room := range []string{"전시실", "전시관", "서화관", "서화실", "기증", "분청사기", "신라천년보고", "잔디마당", "한글나눔마당", "디지털실감영상관", "복합전시", "문화창조원", "아시아문화박물관", "대나무정원", "다목적홀", "별관", "exhibitionhall", "exhibitionshall", "hangeulnanum", "detachedhall", "企画展", "企划展", "常设展", "常設展", "韩古尔共享室", "屋外芝生"} {
		if strings.Contains(normalized, normalizeVenueText(room)) {
			if name, ok := museumHosts[host]; ok {
				return venueByName(name)
			}
		}
	}
	if normalized == "lawn" || normalized == "1階" || normalized == "1楼" {
		return venueByName(museumHosts[host])
	}
	return exhibitionVenue{}, false
}

func enrichExhibitionLocation(exhibition *publicExhibition) bool {
	if validMapCoordinates(exhibition.Latitude, exhibition.Longitude) {
		return false
	}
	venue, ok := resolveExhibitionVenue(exhibition.Address, exhibition.Metadata)
	if !ok {
		return false
	}
	place := exhibition.Address
	exhibition.Latitude, exhibition.Longitude = venue.Latitude, venue.Longitude
	if exhibition.Metadata == nil {
		exhibition.Metadata = make(map[string]string)
	}
	exhibition.Metadata["원문 장소"] = place
	exhibition.Metadata["지도표시"] = "예"
	exhibition.Metadata["좌표 출처"] = venue.SourceURL
	exhibition.Metadata["좌표 확인방법"] = "전시장·원문 기관 대조"
	exhibition.Metadata["전시장"] = venue.Name
	if !strings.Contains(normalizeVenueText(place), normalizeVenueText(venue.Name)) {
		exhibition.Address = venue.Name
		if place != exhibition.Metadata["연계기관"] {
			exhibition.Address += " · " + place
		}
	}
	exhibition.Metadata["장소"] = exhibition.Address
	exhibition.BodyMarkdown = strings.Replace(exhibition.BodyMarkdown, "장소:\n"+place, "장소:\n"+exhibition.Address, 1)
	return true
}

func (s *Server) backfillExhibitionLocations(ctx context.Context) error {
	posts, err := s.queryPosts(ctx, "", "", "", 10000)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, post := range posts {
		if post.SourceType != "kcisa-open-data" {
			continue
		}
		exhibition := publicExhibition{Address: post.Address, Metadata: post.Metadata, Latitude: post.Latitude, Longitude: post.Longitude, BodyMarkdown: post.BodyMarkdown}
		if !enrichExhibitionLocation(&exhibition) {
			continue
		}
		metadata, _ := json.Marshal(exhibition.Metadata)
		if _, err := tx.Exec(ctx, `UPDATE posts SET latitude=$2, longitude=$3, metadata=$4, address=$5, body_markdown=$6, updated_at=NOW() WHERE id=$1`, post.ID, exhibition.Latitude, exhibition.Longitude, metadata, exhibition.Address, exhibition.BodyMarkdown); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
