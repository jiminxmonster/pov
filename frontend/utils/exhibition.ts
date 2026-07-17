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
