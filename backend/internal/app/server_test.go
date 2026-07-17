package app

import (
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
