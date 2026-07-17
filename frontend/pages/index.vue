<script setup lang="ts">
import { ArrowDown, ArrowRight, LoaderCircle, MapPin, Search, X } from '@lucide/vue'
import type { ExhibitionPost, SearchResponse } from '~/types/post'

const config = useRuntimeConfig()
const router = useRouter()
const logoUrl = `${config.app.baseURL}logo.png`
const query = ref('')
const posts = ref<ExhibitionPost[]>([])
const selected = ref<ExhibitionPost | null>(null)
const loading = ref(false)
const interpretation = ref('')
const mapSection = ref<HTMLElement | null>(null)
const currentBbox = ref('')
const tapHistory = ref<number[]>([])

useSeoMeta({
  title: '전지적관람시점',
  ogTitle: '전지적관람시점',
  description: '지도를 움직이고, 지금 보고 싶은 공연과 전시를 발견하세요.',
})

async function loadPosts() {
  loading.value = true
  try {
    const result = await $fetch<SearchResponse>(`${config.public.apiBase}/posts`)
    posts.value = result.items
  } finally {
    loading.value = false
  }
}

async function search() {
  const text = query.value.trim()
  if (!text) {
    await loadPosts()
    mapSection.value?.scrollIntoView({ behavior: 'smooth' })
    return
  }

  loading.value = true
  try {
    const result = await $fetch<SearchResponse>(`${config.public.apiBase}/search/ai`, {
      method: 'POST',
      body: { query: text, bbox: currentBbox.value },
    })
    posts.value = result.items
    interpretation.value = result.interpretation || ''
    mapSection.value?.scrollIntoView({ behavior: 'smooth' })
  } finally {
    loading.value = false
  }
}

function handleLogoTap() {
  const now = Date.now()
  tapHistory.value = [...tapHistory.value.filter((time) => now - time < 1500), now]
  if (tapHistory.value.length >= 3) {
    tapHistory.value = []
    router.push('/admin/login')
  }
}

function selectPost(post: ExhibitionPost) {
  selected.value = post
}

onMounted(loadPosts)
</script>

<template>
  <main>
    <section class="landing-room" aria-labelledby="brand-title">
      <button class="brand-mark-button" type="button" aria-label="POV" @click="handleLogoTap">
        <img class="brand-mark" :src="logoUrl" alt="POV 로고">
      </button>

      <h1 id="brand-title" class="brand-title">전지적관람시점</h1>

      <form class="search-line" role="search" @submit.prevent="search">
        <label for="pov-search" class="search-prefix">pov:</label>
        <input
          id="pov-search"
          v-model="query"
          name="query"
          autocomplete="off"
          placeholder="어떤 장면을 보고 싶나요?"
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

    <section ref="mapSection" class="discovery" aria-labelledby="discovery-title">
      <header class="discovery-header">
        <div>
          <p class="eyebrow">POV MAP</p>
          <h2 id="discovery-title">지도 위의 장면들</h2>
          <p v-if="interpretation" class="interpretation">{{ interpretation }}</p>
        </div>
        <form class="compact-search" @submit.prevent="search">
          <Search :size="17" aria-hidden="true" />
          <input v-model="query" aria-label="결과 다시 검색" placeholder="다시 검색">
        </form>
      </header>

      <div class="discovery-layout">
        <aside class="post-list" aria-label="공연·전시 검색 결과">
          <button
            v-for="post in posts"
            :key="post.id"
            type="button"
            class="post-list-item"
            :class="{ 'is-active': selected?.id === post.id }"
            @click="selectPost(post)"
          >
            <span class="post-list-index">{{ String(posts.indexOf(post) + 1).padStart(2, '0') }}</span>
            <span class="post-list-copy">
              <strong>{{ post.title }}</strong>
              <small><MapPin :size="13" /> {{ post.address || '장소 확인 중' }}</small>
            </span>
          </button>
          <p v-if="!loading && posts.length === 0" class="empty-copy">조건에 맞는 장면이 아직 없습니다.</p>
        </aside>

        <div class="map-frame">
          <PovMap
            :posts="posts"
            :selected-id="selected?.id"
            @select="selectPost"
            @bounds-changed="currentBbox = $event"
          />
        </div>
      </div>
    </section>

    <Transition name="fade">
      <div v-if="selected" class="detail-backdrop" @click.self="selected = null">
        <article class="detail-sheet" aria-modal="true" role="dialog" :aria-label="selected.title">
          <button class="icon-button detail-close" type="button" aria-label="닫기" @click="selected = null">
            <X :size="20" />
          </button>
          <img v-if="selected.image_url" :src="selected.image_url" :alt="`${selected.title} 대표 이미지`" class="detail-image">
          <p class="eyebrow">EXHIBITION NOTE</p>
          <h2>{{ selected.title }}</h2>
          <p class="detail-address"><MapPin :size="15" /> {{ selected.address }}</p>
          <div class="detail-body">{{ selected.body_markdown }}</div>
        </article>
      </div>
    </Transition>
  </main>
</template>
