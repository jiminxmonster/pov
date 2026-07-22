<script setup lang="ts">
import { ArrowDown, ArrowRight, LoaderCircle, MapPin, Plus, Search, X } from '@lucide/vue'
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
const mapSection = ref<HTMLElement | null>(null)
const listSection = ref<HTMLElement | null>(null)
const currentBbox = ref('')
const tapHistory = ref<number[]>([])
const selectedFields = computed(() => selected.value ? parseExhibitionFields(selected.value.body_markdown) : [])
const selectedExpired = computed(() => selected.value ? isExhibitionExpired(selected.value) : false)
const mapPosts = computed(() => posts.value.filter(post => (
  !isExhibitionExpired(post)
  && post.metadata['지도표시'] !== '아니오'
  && post.latitude >= 33
  && post.latitude <= 39
  && post.longitude >= 124
  && post.longitude <= 132
)))

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
    interpretation.value = ''
    aiPowered.value = false
  } finally {
    loading.value = false
  }
}

function applySearchResult(result: SearchResponse) {
  posts.value = result.items
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
  const section = target === 'map' ? mapSection : listSection
  section.value?.scrollIntoView({ behavior: 'smooth' })
}

async function search(target: 'map' | 'list' = 'map') {
  const text = query.value.trim()
  if (!text) {
    await loadPosts()
    const section = target === 'map' ? mapSection : listSection
    section.value?.scrollIntoView({ behavior: 'smooth' })
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
  const view = route.query.view === 'list' ? 'list' : route.query.view === 'map' ? 'map' : ''
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
  await loadPosts()
  if (view) {
    await nextTick()
    const section = view === 'map' ? mapSection : listSection
    section.value?.scrollIntoView({ behavior: 'smooth' })
  }
}

function handleLogoTap() {
  const now = Date.now()
  tapHistory.value = [...tapHistory.value.filter(time => now - time < 1500), now]
  if (tapHistory.value.length >= 3) {
    tapHistory.value = []
    router.push('/admin/login')
  }
}

function goToMain() {
  if (import.meta.client) window.scrollTo({ top: 0, behavior: 'smooth' })
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

onMounted(initializePage)
</script>

<template>
  <main>
    <section class="landing-room" aria-labelledby="brand-title">
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

      <button class="down-cue" type="button" aria-label="지도로 이동" @click="mapSection?.scrollIntoView({ behavior: 'smooth' })">
        <ArrowDown :size="24" />
      </button>
    </section>

    <section id="map" ref="mapSection" class="discovery" aria-label="지도 위의 장면들">
      <header class="map-toolbar">
        <button class="index-emblem" type="button" aria-label="메인 화면으로 이동" @click="goToMain">
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

      <div class="discovery-layout">
        <aside class="post-list" aria-label="지도에 표시된 공연·전시">
          <button
            v-for="(post, index) in mapPosts"
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
            </span>
            <img v-if="isExhibitionExpired(post)" :src="expiredStampUrl" alt="종료된 전시" class="expired-row-stamp is-compact">
          </button>
          <p v-if="loading" class="empty-copy"><LoaderCircle :size="18" class="spin" /> 전시를 불러오고 있습니다.</p>
          <p v-else-if="mapPosts.length === 0" class="empty-copy">현재 관람할 수 있는 장면이 아직 없습니다.</p>
        </aside>

        <div class="map-frame">
          <PovMap
            :posts="mapPosts"
            :selected-id="selected?.id"
            @select="selectPost"
            @bounds-changed="currentBbox = $event"
          />
        </div>
      </div>

      <button class="section-down-cue" type="button" aria-label="전시 목록으로 이동" @click="listSection?.scrollIntoView({ behavior: 'smooth' })">
        <ArrowDown :size="22" />
      </button>
    </section>

    <section id="list" ref="listSection" class="exhibition-index" aria-labelledby="exhibition-index-title">
      <header class="index-toolbar">
        <button class="index-emblem" type="button" aria-label="메인 화면으로 이동" @click="goToMain">
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
