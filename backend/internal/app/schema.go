package app

import "context"

func (s *Server) migrate(ctx context.Context) error {
	_, err := s.db.Exec(ctx, schemaSQL)
	return err
}

const schemaSQL = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS posts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  body_markdown TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  address TEXT NOT NULL DEFAULT '',
  latitude DOUBLE PRECISION NOT NULL DEFAULT 37.5665,
  longitude DOUBLE PRECISION NOT NULL DEFAULT 126.9780,
  image_url TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'review' CHECK (status IN ('draft', 'review', 'published', 'archived')),
  source_type TEXT NOT NULL DEFAULT 'manual',
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS posts_status_published_at_idx ON posts (status, published_at DESC);
CREATE INDEX IF NOT EXISTS posts_location_idx ON posts (longitude, latitude);
CREATE INDEX IF NOT EXISTS posts_metadata_idx ON posts USING GIN (metadata);

CREATE TABLE IF NOT EXISTS app_settings (
  name TEXT PRIMARY KEY,
  value_encrypted BYTEA NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO posts (slug, title, body_markdown, metadata, address, latitude, longitude, status, source_type, published_at)
VALUES
  (
    'sample-seongsu-light',
    '빛이 머무는 시간',
    E'전시명:\n빛이 머무는 시간\n\n작가(작가소개):\n빛과 공간을 기록하는 신진 작가 4인\n\n관람료:\n무료\n\n전시기간:\n2026-07-01 ~ 2026-08-31\n\n장소:\n서울 성동구 성수동2가\n\n도슨트(전시장 가이드) 유무:\n주말 14시\n\n찾아가는 방법:\n성수역에서 도보 8분\n\n주차정보:\n인근 공영주차장 이용\n\n전시내용:\n도시의 빛이 머문 장면을 설치와 사진으로 보여주는 전시\n\n굿즈샵정보:\n엽서와 소형 포스터\n\n주변에 함께 볼 만한 전시:\n성수동 디자인 전시\n\n주변에 볼거리:\n서울숲\n\n맛집:\n성수동 카페 거리\n\n감상평:\n천천히 걷고 싶은 전시\n\n페르소나 정보입력:\n혼자 조용히 관람하는 사람',
    '{"전시명":"빛이 머무는 시간","관람료":"무료","장소":"서울 성동구 성수동2가","도슨트(전시장 가이드) 유무":"주말 14시"}'::jsonb,
    '서울 성동구 성수동2가', 37.5445, 127.0560, 'published', 'seed', NOW()
  ),
  (
    'sample-samcheong-paper',
    '종이와 기억의 방',
    E'전시명:\n종이와 기억의 방\n\n작가(작가소개):\n기록 매체를 탐구하는 박서윤\n\n관람료:\n12,000원\n\n전시기간:\n2026-07-10 ~ 2026-09-20\n\n장소:\n서울 종로구 삼청동\n\n도슨트(전시장 가이드) 유무:\n매일 15시\n\n찾아가는 방법:\n안국역에서 도보 12분\n\n주차정보:\n주차 불가\n\n전시내용:\n종이와 손글씨가 기억을 보관하는 방식을 살펴본다.\n\n굿즈샵정보:\n아트북과 노트\n\n주변에 함께 볼 만한 전시:\n국립현대미술관 서울\n\n주변에 볼거리:\n삼청동 골목\n\n맛집:\n북촌 일대\n\n감상평:\n기록하고 싶은 마음이 남는다.\n\n페르소나 정보입력:\n책과 문구를 좋아하는 관람자',
    '{"전시명":"종이와 기억의 방","관람료":"12,000원","장소":"서울 종로구 삼청동","도슨트(전시장 가이드) 유무":"매일 15시"}'::jsonb,
    '서울 종로구 삼청동', 37.5824, 126.9810, 'published', 'seed', NOW()
  )
ON CONFLICT (slug) DO NOTHING;
`
