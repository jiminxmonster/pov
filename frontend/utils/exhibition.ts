export const exhibitionLabels = [
  '전시명',
  '작가(작가소개)',
  '관람료',
  '전시기간',
  '장소',
  '도슨트(전시장 가이드) 유무',
  '찾아가는 방법',
  '주차정보',
  '전시내용',
  '굿즈샵정보',
  '주변에 함께 볼 만한 전시',
  '주변에 볼거리',
  '맛집',
  '감상평',
  '페르소나 정보입력',
] as const

export const exhibitionTemplate = `${exhibitionLabels.map(label => `${label}:\n`).join('\n')}\n`

export interface ExhibitionField {
  label: string
  value: string
}

export type ExhibitionContentSegment =
  | { type: 'text', value: string }
  | { type: 'image', url: string, alt: string }
  | { type: 'video', url: string, alt: string }

export function parseExhibitionFields(body: string): ExhibitionField[] {
  const values = new Map<string, string>()
  let currentLabel = ''

  for (const line of body.split('\n')) {
    const trimmed = line.trim()
    const matchedLabel = exhibitionLabels.find(label => trimmed.startsWith(`${label}:`))
    if (matchedLabel) {
      currentLabel = matchedLabel
      values.set(currentLabel, trimmed.slice(matchedLabel.length + 1).trim())
      continue
    }
    if (!trimmed || !currentLabel) continue
    const currentValue = values.get(currentLabel) || ''
    values.set(currentLabel, currentValue ? `${currentValue}\n${trimmed}` : trimmed)
  }

  return exhibitionLabels
    .map(label => ({ label, value: values.get(label)?.trim() || '' }))
    .filter(field => field.value)
}

export function parseExhibitionContent(value: string): ExhibitionContentSegment[] {
  const segments: ExhibitionContentSegment[] = []
  const mediaPattern = /([!@])\[([^\]]*)\]\(([^)\s]+)\)/g
  let cursor = 0

  for (const match of value.matchAll(mediaPattern)) {
    const index = match.index ?? 0
    const url = match[3]?.trim() || ''
    if (!isSafeImageURL(url)) continue

    pushTextSegment(segments, value.slice(cursor, index))
    segments.push({
      type: match[1] === '!' ? 'image' : 'video',
      url,
      alt: match[2]?.trim() || (match[1] === '!' ? '전시 본문 이미지' : '전시 본문 영상'),
    })
    cursor = index + match[0].length
  }

  pushTextSegment(segments, value.slice(cursor))
  return segments.length ? segments : [{ type: 'text', value }]
}

function pushTextSegment(segments: ExhibitionContentSegment[], value: string) {
  const normalized = value.replace(/^\s*\n/, '').replace(/\n\s*$/, '')
  if (normalized.trim()) segments.push({ type: 'text', value: normalized })
}

function isSafeImageURL(value: string) {
  if (value.startsWith('/') && !value.startsWith('//')) return true
  try {
    const url = new URL(value)
    return url.protocol === 'https:' || url.protocol === 'http:'
  } catch {
    return false
  }
}
