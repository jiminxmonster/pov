export interface ExhibitionPost {
  id: string
  slug: string
  title: string
  body_markdown: string
  metadata: Record<string, string>
  address: string
  latitude: number
  longitude: number
  image_url: string
  status: 'draft' | 'review' | 'published' | 'archived'
  source_type: string
  published_at?: string
  created_at: string
  updated_at: string
}

export interface SearchResponse {
  items: ExhibitionPost[]
  interpretation?: string
  total: number
}

