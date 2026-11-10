import type { ExhibitionPost } from '~/types/post'

export interface MapPoint {
  latitude: number
  longitude: number
}

export const mapGenres = ['전체', '미술', '공예', '조각', '사진', '미디어', '새로운'] as const
export type MapGenre = typeof mapGenres[number]
type SpecificGenre = Exclude<MapGenre, '전체' | '미술' | '새로운'>

const specificGenres: [SpecificGenre, RegExp][] = [
  ['공예', /공예|도예|도자(?:기|전)|자수|직조|\b(?:crafts?|ceramics?|pottery)\b/i],
  ['조각', /조각|\bsculptures?\b/i],
  ['사진', /사진|포토|\b(?:photography|photos?)\b/i],
  ['미디어', /미디어\s*(?:아트|전시)?|영상\s*(?:전|작품|설치)|비디오\s*아트|디지털\s*아트|\b(?:media|video|digital)\s+art\b/i],
]

function genreFromText(text: string): MapGenre | undefined {
  const matches = specificGenres.filter(([, pattern]) => pattern.test(text))
  if (matches.length > 1) return '새로운'
  if (matches.length === 1) return matches[0][0]
  if (/미술|회화|그림|드로잉|판화|수채화|유화|한국화|서양화|\b(?:paintings?|drawings?|fine art)\b/i.test(text)) return '미술'
}

export function exhibitionMapGenre(post: ExhibitionPost): MapGenre {
  const category = post.metadata['분류']?.trim()
  return genreFromText(post.metadata['장르'] || '')
    ?? genreFromText(category === '전시/미술' ? '' : category || '')
    ?? genreFromText(post.title)
    ?? '새로운'
}

export function distanceKm(from: MapPoint, to: MapPoint) {
  const radians = Math.PI / 180
  const latitude = (to.latitude - from.latitude) * radians
  const longitude = (to.longitude - from.longitude) * radians
  const a = Math.sin(latitude / 2) ** 2
    + Math.cos(from.latitude * radians) * Math.cos(to.latitude * radians) * Math.sin(longitude / 2) ** 2
  return 12742 * Math.asin(Math.min(1, Math.sqrt(a)))
}
