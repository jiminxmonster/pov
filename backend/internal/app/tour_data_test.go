package app

import (
	"strings"
	"testing"
)

func TestDecodeTourAPIResponse(t *testing.T) {
	payload, err := decodeTourAPIResponse(strings.NewReader(`{"response":{"header":{"resultCode":"0000","resultMsg":"OK"},"body":{"totalCount":2,"items":{"item":[{"contentid":"101","title":"전시 1"},{"contentid":"102","title":"전시 2"}]}}}}`))
	if err != nil || len(payload.Response.Body.Items.Item) != 2 || payload.Response.Body.TotalCount != 2 {
		t.Fatalf("unexpected list response: %#v, %v", payload, err)
	}
	payload, err = decodeTourAPIResponse(strings.NewReader(`{"response":{"header":{"resultCode":"0000"},"body":{"totalCount":1,"items":{"item":{"contentid":"101","title":"전시 1"}}}}}`))
	if err != nil || len(payload.Response.Body.Items.Item) != 1 {
		t.Fatalf("unexpected singleton response: %#v, %v", payload, err)
	}
	if _, err := decodeTourAPIResponse(strings.NewReader(`{"OpenAPI_ServiceResponse":{"cmmMsgHeader":{"errMsg":"SERVICE_KEY_IS_NULL"}}}`)); err == nil {
		t.Fatal("missing key must not be accepted as an empty result")
	}
}

func TestConvertTourEvent(t *testing.T) {
	event := tourEvent{
		ContentID: "12345", Title: "미술 전시", Address: "서울 종로구", Category: "A02080500",
		StartDate: "20260901", EndDate: "20261031", Latitude: "37.573", Longitude: "126.979",
		ImageURL: "https://example.com/poster.jpg", CopyrightType: "Type3",
	}
	exhibition, ok := convertTourEvent(event)
	if !ok || exhibition.Slug != "tour-exhibition-12345" || exhibition.Metadata["전시종료일"] != "2026-10-31" || exhibition.Latitude != 37.573 {
		t.Fatalf("unexpected exhibition: %#v, ok=%v", exhibition, ok)
	}
	if exhibition.Metadata["원문 링크"] != "https://data.visitkorea.or.kr/page/12345" || exhibition.Metadata["이미지 이용유형"] != "Type3" {
		t.Fatalf("missing attribution: %#v", exhibition.Metadata)
	}
	event.Category = "A02080200"
	if _, ok := convertTourEvent(event); ok {
		t.Fatal("non-exhibition event must be excluded")
	}
	event.Category = "A02080500"
	event.ProgressType = "취소"
	if _, ok := convertTourEvent(event); ok {
		t.Fatal("cancelled exhibition must be excluded")
	}
	event.ProgressType = ""
	event.Latitude = "unknown"
	exhibition, ok = convertTourEvent(event)
	if !ok || exhibition.Latitude != 0 || exhibition.Longitude != 0 {
		t.Fatalf("unknown coordinates must not be placed on map: %#v", exhibition)
	}
}

func TestTourDataSettings(t *testing.T) {
	settings := normalizeTourDataSettings(publicDataSettings{APIKey: "abc%2Bdef", Limit: 5000})
	if settings.APIKey != "abc+def" || settings.Limit != 1000 {
		t.Fatalf("unexpected settings: %#v", settings)
	}
	server := Server{config: Config{SessionSecret: "test-session-secret-that-is-long-enough"}}
	encrypted, err := server.sealNamedSetting(tourPublicDataSettingName, []byte("tour-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.openNamedSetting(kcisaPublicDataSettingName, encrypted); err == nil {
		t.Fatal("tourism key must use a separate encryption context")
	}
}
