<script setup lang="ts">
import { ArrowDown, ArrowRight, House, List, LoaderCircle, MapPin, Plus, Search, X } from '@lucide/vue'
import type { ExhibitionPost, SearchResponse } from '~/types/post'
import { isExhibitionExpired, parseExhibitionContent, parseExhibitionFields } from '~/utils/exhibition'

const config = useRuntimeConfig()
const router = useRouter()
const route = useRoute()
const logoUrl = `${config.app.baseURL}logo.png`
const expiredStampUrl = `${config.app.baseURL}expired-stamp.png`
const chatInitialKey = 'pov-chat-initial-v1'
const searchResultKey = 'pov-search-result-v1'
const query = ref('')
const posts = ref<ExhibitionPost[]>([])
const selected = ref<ExhibitionPost | null>(null)
const loading = ref(false)
const interpretation = ref('')
const aiPowered = ref(false)
const views = [
  { id: 'main', label: '메인', icon: House },
  { id: 'map', label: '지도', icon: MapPin },
  { id: 'list', label: '목록', icon: List },
] as const
type View = typeof views[number]['id']
const screens = ref<HTMLElement | null>(null)
const activeView = ref<View>('main')
const mobileLayout = ref(false)
let mobileMedia: MediaQueryList | undefined
const mapSection = ref<HTMLElement | null>(null)
const listSection = ref<HTMLElement | null>(null)
const mapLayout = ref<HTMLElement | null>(null)
const mapResults = ref<HTMLElement | null>(null)
const mapSelection = ref<Set<string> | null>(null)
const sheetStage = ref(0)
const sheetDragHeight = ref<number>()
const sheetLabels = ['접힘', '중간', '펼침']
let sheetGesture: { id: number; startY: number; startHeight: number; maxHeight: number } | undefined
let sheetDragged = false
const currentBbox = ref('')
const tapHistory = ref<number[]>([])
const selectedFields = computed(() => selected.value ? parseExhibitionFields(selected.value.body_markdown) : [])
const selectedExpired = computed(() => selected.value ? isExhibitionExpired(selected.value) : false)
const mapPosts = computed(() => posts.value.filter(post => (
  post.metadata['지도표시'] !== '아니오'
  && Number.isFinite(post.latitude) && Number.isFinite(post.longitude)
  && post.latitude >= -85 && post.latitude <= 85
  && post.longitude >= -180 && post.longitude <= 180
  && (post.latitude !== 0 || post.longitude !== 0)
)))
const pendingLocationCount = computed(() => posts.value.length - mapPosts.value.length)
const mappedPostIds = computed(() => new Set(mapPosts.value.map(post => post.id)))
const mapResultPosts = computed(() => {
  const ids = mapSelection.value
  return ids ? posts.value.filter(post => ids.has(post.id)) : posts.value
})

useSeoMeta({
  title: '전지적관람시점',
  ogTitle: '전지적관람시점',
  description: '지도와 정돈된 목록에서 지금 보고 싶은 공연과 전시를 발견하세요.',
})

async function loadPosts() {
  loading.value = true
  try {
    const result = await $fetch<SearchResponse>(`${config.public.apiBase}/posts`)
    posts.value = result.items
    mapSelection.value = null
    interpretation.value = ''
    aiPowered.value = false
  } finally {
    loading.value = false
  }
}

function applySearchResult(result: SearchResponse) {
  posts.value = result.items
  mapSelection.value = null
  interpretation.value = result.interpretation || ''
  aiPowered.value = Boolean(result.ai_powered)
  selected.value = selected.value && posts.value.some(post => post.id === selected.value?.id) ? selected.value : null
}

function shouldOpenConversation(result: SearchResponse) {
  return result.mode === 'wizard' || result.mode === 'chat' || Boolean(result.question?.trim())
}

async function showSearchResult(result: SearchResponse, target: 'map' | 'list') {
  applySearchResult(result)
  await nextTick()
  navigateTo(target)
}

async function search(target: 'map' | 'list' = 'map') {
  const text = query.value.trim()
  if (!text) {
    await loadPosts()
    navigateTo(target)
    return
  }

  loading.value = true
  try {
    const result = await $fetch<SearchResponse>(`${config.public.apiBase}/search/ai`, {
      method: 'POST',
      body: target === 'map' ? { query: text, bbox: currentBbox.value } : { query: text },
    })
    if (shouldOpenConversation(result)) {
      sessionStorage.setItem(chatInitialKey, JSON.stringify({ query: text, result }))
      await router.push({ path: '/chat', query: { q: text } })
      return
    }
    await showSearchResult(result, target)
  } finally {
    loading.value = false
  }
}

async function initializePage() {
  const requestedView = route.query.view || route.hash.slice(1)
  const view = requestedView === 'list' || requestedView === 'map' ? requestedView : undefined
  const stored = sessionStorage.getItem(searchResultKey)
  if (stored) {
    sessionStorage.removeItem(searchResultKey)
    try {
      const payload = JSON.parse(stored) as { result: SearchResponse; target?: 'map' | 'list'; query?: string }
      if (payload.result?.items) {
        query.value = payload.query || query.value
        await showSearchResult(payload.result, payload.target || view || 'map')
        return
      }
    } catch {
      // Ignore expired or malformed browser state and load the current index.
    }
  }
  if (view) navigateTo(view, 'instant')
  await loadPosts()
}

function handleLogoTap() {
  const now = Date.now()
  tapHistory.value = [...tapHistory.value.filter(time => now - time < 1500), now]
  if (tapHistory.value.length >= 3) {
    tapHistory.value = []
    router.push('/admin/login')
  }
}

function navigateTo(view: View, behavior: ScrollBehavior = 'smooth') {
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) behavior = 'instant'
  if (mobileLayout.value && screens.value) {
    const index = views.findIndex(item => item.id === view)
    screens.value.scrollTo({ left: index * screens.value.clientWidth, behavior })
    if (behavior === 'instant') activeView.value = view
  } else if (view === 'main') {
    window.scrollTo({ top: 0, behavior })
  } else {
    const section = view === 'map' ? mapSection : listSection
    section.value?.scrollIntoView({ behavior })
  }
}

function syncActiveView() {
  const track = screens.value
  if (!mobileLayout.value || !track?.clientWidth) return
  const index = Math.round(track.scrollLeft / track.clientWidth)
  activeView.value = views[index]?.id ?? 'main'
}

function updateMobileLayout() {
  mobileLayout.value = Boolean(mobileMedia?.matches)
  if (mobileLayout.value) navigateTo(activeView.value, 'instant')
}

function stepSheet(direction: number) {
  sheetStage.value = Math.min(2, Math.max(0, sheetStage.value + direction))
}

function toggleSheet(event: MouseEvent) {
  if (event.detail && sheetDragged) return
  sheetStage.value = (sheetStage.value + 1) % sheetLabels.length
}

function startSheetDrag(event: PointerEvent) {
  const maxHeight = mapLayout.value?.clientHeight
  if (!event.isPrimary || event.button !== 0 || !maxHeight) return
  const handle = event.currentTarget as HTMLButtonElement
  const startHeight = handle.parentElement?.getBoundingClientRect().height ?? 48
  sheetDragged = false
  sheetGesture = { id: event.pointerId, startY: event.clientY, startHeight, maxHeight }
  handle.setPointerCapture(event.pointerId)
}

function moveSheetDrag(event: PointerEvent) {
  if (sheetGesture?.id !== event.pointerId) return
  const distance = sheetGesture.startY - event.clientY
  if (Math.abs(distance) < 8 && !sheetDragged) return
  sheetDragged = true
  sheetDragHeight.value = Math.max(48, Math.min(sheetGesture.maxHeight, sheetGesture.startHeight + distance))
}

function finishSheetDrag(event: PointerEvent) {
  if (sheetGesture?.id !== event.pointerId) return
  const distance = sheetGesture.startY - event.clientY
  if (event.type === 'pointerup' && Math.abs(distance) >= 24) {
    sheetDragged = true
    stepSheet(Math.sign(distance))
  }
  sheetGesture = undefined
  sheetDragHeight.value = undefined
}

function postField(post: ExhibitionPost, label: string) {
  return post.metadata[label] || parseExhibitionFields(post.body_markdown).find(field => field.label === label)?.value || ''
}

function fieldContent(value: string) {
  return parseExhibitionContent(value)
}

function selectPost(post: ExhibitionPost) {
  selected.value = post
}

function showMapSelection(ids: string[] | null) {
  if (!ids && !mapSelection.value) return
  mapSelection.value = ids ? new Set(ids) : null
  if (ids && mobileLayout.value && sheetStage.value === 0) sheetStage.value = 1
  if (mapResults.value) mapResults.value.scrollTop = 0
}

onMounted(() => {
  mobileMedia = window.matchMedia('(max-width: 860px), (max-width: 1024px) and (pointer: coarse)')
  updateMobileLayout()
  mobileMedia.addEventListener('change', updateMobileLayout)
  return initializePage()
})
onBeforeUnmount(() => mobileMedia?.removeEventListener('change', updateMobileLayout))
</script>

<template>
  <main ref="screens" class="pov-screens" @scroll.passive="syncActiveView">
    <section id="main" class="landing-room" aria-labelledby="brand-title" :inert="mobileLayout && activeView !== 'main'">
      <button class="brand-mark-button" type="button" aria-label="POV" @click="handleLogoTap">
        <img class="brand-mark" :src="logoUrl" alt="POV 로고">
      </button>

      <h1 id="brand-title" class="brand-title">전지적관람시점</h1>

      <form class="search-line" role="search" @submit.prevent="search('map')">
        <label for="pov-search" class="search-prefix">pov:</label>
        <input
          id="pov-search"
          v-model="query"
          name="query"
          autocomplete="off"
          placeholder="당신이 원하는 관람은?"
          aria-label="공연과 전시 검색"
        >
        <button type="submit" class="search-submit" :disabled="loading" aria-label="검색">
          <LoaderCircle v-if="loading" :size="20" class="spin" />
          <ArrowRight v-else :size="21" />
        </button>
      </form>

      <button class="down-cue" type="button" aria-label="지도로 이동" @click="navigateTo('map')">
        <ArrowDown :size="24" />
      </button>
    </section>

    <section id="map" ref="mapSection" class="discovery" aria-label="지도 위의 장면들" :inert="mobileLayout && activeView !== 'map'">
      <header class="map-toolbar">
        <button class="index-emblem" type="button" aria-label="메인 화면으로 이동" @click="navigateTo('main')">
          <img :src="logoUrl" alt="POV 엠블럼">
        </button>

        <form class="compact-search" role="search" @submit.prevent="search('map')">
          <Search :size="16" aria-hidden="true" />
          <input v-model="query" aria-label="지도 결과 검색" placeholder="전시 검색">
        </form>
      </header>

      <p v-if="interpretation" class="map-interpretation" :class="{ 'is-ai': aiPowered }">
        <strong v-if="aiPowered">POV AI</strong>
        <span>{{ interpretation }}</span>
      </p>

      <div v-if="!loading" class="map-data-summary">
        <span v-if="mapSelection" role="status">선택한 전시 {{ mapResultPosts.length.toLocaleString() }}개</span>
        <span v-else role="status">
          지도 {{ mapPosts.length.toLocaleString() }}개 · 전체 {{ posts.length.toLocaleString() }}개
          <span v-if="pendingLocationCount"> · 위치 확인 중 {{ pendingLocationCount.toLocaleString() }}개</span>
        </span>
        <button v-if="mapSelection" class="map-results-reset" type="button" @click="showMapSelection(null)">전체 보기</button>
      </div>

      <div ref="mapLayout" class="discovery-layout">
        <aside
          class="map-results-sheet"
          aria-label="지도 공연·전시 결과"
          :data-stage="sheetStage"
          :style="sheetDragHeight === undefined ? undefined : { '--sheet-drag-height': `${sheetDragHeight}px` }"
        >
          <button
            class="map-sheet-handle"
            type="button"
            aria-controls="map-results"
            :aria-expanded="sheetStage !== 0"
            :aria-label="`지도 결과 ${sheetLabels[sheetStage]}. 위아래로 밀어 높이 조절`"
            @click="toggleSheet"
            @keydown.up.prevent="stepSheet(1)"
            @keydown.down.prevent="stepSheet(-1)"
            @pointerdown="startSheetDrag"
            @pointermove="moveSheetDrag"
            @pointerup="finishSheetDrag"
            @pointercancel="finishSheetDrag"
            @lostpointercapture="finishSheetDrag"
          >
            <span class="map-sheet-grip" aria-hidden="true" />
            <span>전시 결과 <strong>{{ mapResultPosts.length.toLocaleString() }}</strong></span>
          </button>
          <div id="map-results" ref="mapResults" class="post-list" :inert="mobileLayout && sheetStage === 0">
            <button
              v-for="(post, index) in mapResultPosts"
              :key="post.id"
              type="button"
              class="post-list-item"
              :class="{ 'is-active': selected?.id === post.id, 'is-expired': isExhibitionExpired(post) }"
              @click="selectPost(post)"
            >
              <span class="post-list-index">{{ String(index + 1).padStart(2, '0') }}</span>
              <span class="post-list-copy">
                <strong>{{ post.title }}</strong>
                <small><MapPin :size="13" /> {{ post.address || '장소 확인 중' }}</small>
                <small v-if="!mappedPostIds.has(post.id)">위치 확인 중</small>
              </span>
              <img v-if="isExhibitionExpired(post)" :src="expiredStampUrl" alt="종료된 전시" class="expired-row-stamp is-compact">
            </button>
            <p v-if="loading" class="empty-copy"><LoaderCircle :size="18" class="spin" /> 전시를 불러오고 있습니다.</p>
            <p v-else-if="mapResultPosts.length === 0" class="empty-copy">전시가 아직 없습니다.</p>
          </div>
        </aside>

        <div class="map-frame">
          <PovMap
            :posts="mapPosts"
            :selected-id="selected?.id"
            @results-selected="showMapSelection"
            @bounds-changed="currentBbox = $event"
          />
        </div>
      </div>

      <button class="section-down-cue" type="button" aria-label="전시 목록으로 이동" @click="navigateTo('list')">
        <ArrowDown :size="22" />
      </button>
    </section>

    <section id="list" ref="listSection" class="exhibition-index" aria-labelledby="exhibition-index-title" :inert="mobileLayout && activeView !== 'list'">
      <header class="index-toolbar">
        <button class="index-emblem" type="button" aria-label="메인 화면으로 이동" @click="navigateTo('main')">
          <img :src="logoUrl" alt="POV 엠블럼">
        </button>

        <div class="index-actions">
          <form class="compact-search" role="search" @submit.prevent="search('list')">
            <Search :size="16" aria-hidden="true" />
            <input v-model="query" aria-label="전시 목록 검색" placeholder="전시 검색">
          </form>
        </div>
      </header>

      <div class="index-heading">
        <p class="eyebrow">EXHIBITIONS</p>
        <h2 id="exhibition-index-title">전시 정보</h2>
        <NuxtLink to="/submit" class="submit-plus heading-submit-plus" aria-label="전시 정보 제보하기">
          <Plus :size="24" />
        </NuxtLink>
        <p v-if="interpretation" class="interpretation" :class="{ 'is-ai': aiPowered }">
          <strong v-if="aiPowered">POV AI</strong>
          <span>{{ interpretation }}</span>
        </p>
      </div>

      <div class="exhibition-list" aria-live="polite" aria-label="공연·전시 검색 결과">
        <button
          v-for="(post, index) in posts"
          :key="post.id"
          type="button"
          class="exhibition-list-item"
          :class="{ 'is-expired': isExhibitionExpired(post) }"
          @click="selected = post"
        >
          <span class="exhibition-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <span class="exhibition-copy">
            <strong>{{ post.title }}</strong>
            <span class="exhibition-period">{{ postField(post, '전시기간') || '일정 확인 중' }}</span>
            <small><MapPin :size="13" /> {{ post.address || '장소 확인 중' }}</small>
          </span>
          <ArrowRight :size="18" class="exhibition-arrow" aria-hidden="true" />
          <img v-if="isExhibitionExpired(post)" :src="expiredStampUrl" alt="종료된 전시" class="expired-row-stamp">
        </button>

        <p v-if="loading" class="empty-copy"><LoaderCircle :size="18" class="spin" /> 전시를 불러오고 있습니다.</p>
        <p v-else-if="posts.length === 0" class="empty-copy">조건에 맞는 전시가 아직 없습니다.</p>
      </div>
    </section>

    <nav class="pov-mobile-nav" aria-label="POV 화면 전환" :inert="Boolean(selected)">
      <button
        v-for="view in views"
        :key="view.id"
        type="button"
        :aria-current="activeView === view.id ? 'page' : undefined"
        :aria-controls="view.id"
        @click="navigateTo(view.id)"
      >
        <component :is="view.icon" :size="17" aria-hidden="true" />
        {{ view.label }}
      </button>
    </nav>

    <Transition name="fade">
      <div v-if="selected" class="detail-backdrop" @click.self="selected = null">
        <button class="icon-button detail-close" type="button" aria-label="닫기" @click="selected = null">
          <X :size="20" />
        </button>
        <article class="detail-sheet" :class="{ 'is-expired': selectedExpired }" aria-modal="true" role="dialog" :aria-label="selected.title">
          <div v-if="selectedExpired" class="detail-expired-stamp-layer" aria-hidden="true">
            <img :src="expiredStampUrl" alt="">
          </div>
          <img v-if="selected.image_url" :src="selected.image_url" :alt="`${selected.title} 대표 이미지`" class="detail-image">
          <p class="eyebrow">EXHIBITION NOTE</p>
          <h2>{{ selected.title }}</h2>
          <p class="detail-address"><MapPin :size="15" /> {{ selected.address }}</p>
          <dl class="detail-fields">
            <div v-for="field in selectedFields" :key="field.label" class="detail-field">
              <dt>{{ field.label }}</dt>
              <dd>
                <template v-for="(segment, segmentIndex) in fieldContent(field.value)" :key="`${field.label}-${segmentIndex}`">
                  <figure v-if="segment.type === 'image'" class="detail-inline-figure">
                    <img
                      :src="segment.url"
                      :alt="segment.alt"
                      class="detail-inline-image"
                      loading="lazy"
                    >
                    <figcaption v-if="segment.alt && segment.alt !== '전시 본문 이미지'">{{ segment.alt }}</figcaption>
                  </figure>
                  <figure v-else-if="segment.type === 'video'" class="detail-inline-figure">
                    <video
                      :src="segment.url"
                      class="detail-inline-video"
                      controls
                      playsinline
                      preload="metadata"
                    />
                    <figcaption v-if="segment.alt && segment.alt !== '전시 본문 영상'">{{ segment.alt }}</figcaption>
                  </figure>
                  <span v-else class="detail-inline-text">{{ segment.value }}</span>
                </template>
              </dd>
            </div>
          </dl>
        </article>
      </div>
    </Transition>
  </main>
</template>
