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
  '링크',
  '굿즈샵정보',
  '주변에 함께 볼 만한 전시',
  '주변에 볼거리',
  '맛집',
  '감상평',
  '페르소나 정보입력',
] as const

export const exhibitionTemplate = `${exhibitionLabels.map(label => `${label}:\n`).join('\n')}\n`

export const submissionTemplate = `전시명:\n\n장소:\n\n전시내용:\n`

export interface ExhibitionField {
  label: string
  value: string
}

interface ExhibitionDateSource {
  metadata: Record<string, string>
  body_markdown: string
}

export function isExhibitionExpired(post: ExhibitionDateSource, now = new Date()) {
  const period = post.metadata['전시기간'] ||
    parseExhibitionFields(post.body_markdown).find(field => field.label === '전시기간')?.value || ''
  const periodDates = period.match(/\d{4}\s*[./-]\s*\d{1,2}\s*[./-]\s*\d{1,2}/g) || []
  const rawEndDate = post.metadata['전시종료일'] || periodDates[periodDates.length - 1] || ''
  const parts = rawEndDate.replace(/\s/g, '').replace(/[./]/g, '-').split('-').map(Number)
  if (parts.length !== 3 || parts.some(part => !Number.isFinite(part))) return false

  const [year, month, day] = parts
  const endOfDay = new Date(year, month - 1, day, 23, 59, 59, 999)
  if (endOfDay.getFullYear() !== year || endOfDay.getMonth() !== month - 1 || endOfDay.getDate() !== day) return false
  return endOfDay.getTime() < now.getTime()
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
