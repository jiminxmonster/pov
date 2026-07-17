<script setup lang="ts">
import { ArrowDown, ArrowRight, LoaderCircle, MapPin, Plus, Search, X } from '@lucide/vue'
import type { ExhibitionPost, SearchResponse } from '~/types/post'
import { parseExhibitionFields } from '~/utils/exhibition'

const config = useRuntimeConfig()
const router = useRouter()
const logoUrl = `${config.app.baseURL}logo.png`
const query = ref('')
const posts = ref<ExhibitionPost[]>([])
const selected = ref<ExhibitionPost | null>(null)
const loading = ref(false)
const interpretation = ref('')
const listSection = ref<HTMLElement | null>(null)
const tapHistory = ref<number[]>([])
const selectedFields = computed(() => selected.value ? parseExhibitionFields(selected.value.body_markdown) : [])

useSeoMeta({
  title: '전지적관람시점',
  ogTitle: '전지적관람시점',
  description: '지금 보고 싶은 공연과 전시를 발견하세요.',
})

async function loadPosts() {
  loading.value = true
  try {
    const result = await $fetch<SearchResponse>(`${config.public.apiBase}/posts`)
    posts.value = result.items
    interpretation.value = ''
  } finally {
    loading.value = false
  }
}

async function search() {
  const text = query.value.trim()
  if (!text) {
    await loadPosts()
    listSection.value?.scrollIntoView({ behavior: 'smooth' })
    return
  }

  loading.value = true
  try {
    const result = await $fetch<SearchResponse>(`${config.public.apiBase}/search/ai`, {
      method: 'POST',
      body: { query: text },
    })
    posts.value = result.items
    interpretation.value = result.interpretation || ''
    listSection.value?.scrollIntoView({ behavior: 'smooth' })
  } finally {
    loading.value = false
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

function postField(post: ExhibitionPost, label: string) {
  return post.metadata[label] || parseExhibitionFields(post.body_markdown).find(field => field.label === label)?.value || ''
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

      <button class="down-cue" type="button" aria-label="전시 목록으로 이동" @click="listSection?.scrollIntoView({ behavior: 'smooth' })">
        <ArrowDown :size="24" />
      </button>
    </section>

    <section ref="listSection" class="exhibition-index" aria-labelledby="exhibition-index-title">
      <header class="index-toolbar">
        <button class="index-emblem" type="button" aria-label="POV" @click="handleLogoTap">
          <img :src="logoUrl" alt="POV 엠블럼">
        </button>

        <div class="index-actions">
          <form class="compact-search" role="search" @submit.prevent="search">
            <Search :size="16" aria-hidden="true" />
            <input v-model="query" aria-label="전시 목록 검색" placeholder="전시 검색">
          </form>
          <NuxtLink to="/submit" class="submit-plus" aria-label="전시 정보 제보하기">
            <Plus :size="21" />
          </NuxtLink>
        </div>
      </header>

      <div class="index-heading">
        <p class="eyebrow">EXHIBITIONS</p>
        <h2 id="exhibition-index-title">전시 정보</h2>
        <p v-if="interpretation" class="interpretation">{{ interpretation }}</p>
      </div>

      <div class="exhibition-list" aria-live="polite" aria-label="공연·전시 검색 결과">
        <button
          v-for="(post, index) in posts"
          :key="post.id"
          type="button"
          class="exhibition-list-item"
          @click="selected = post"
        >
          <span class="exhibition-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <span class="exhibition-copy">
            <strong>{{ post.title }}</strong>
            <span class="exhibition-period">{{ postField(post, '전시기간') || '일정 확인 중' }}</span>
            <small><MapPin :size="13" /> {{ post.address || '장소 확인 중' }}</small>
          </span>
          <ArrowRight :size="18" class="exhibition-arrow" aria-hidden="true" />
        </button>

        <p v-if="loading" class="empty-copy"><LoaderCircle :size="18" class="spin" /> 전시를 불러오고 있습니다.</p>
        <p v-else-if="posts.length === 0" class="empty-copy">조건에 맞는 전시가 아직 없습니다.</p>
      </div>

      <NuxtLink to="/submit" class="index-submit-link">
        <Plus :size="18" /> 알고 있는 전시 제보하기
      </NuxtLink>
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
          <dl class="detail-fields">
            <div v-for="field in selectedFields" :key="field.label" class="detail-field">
              <dt>{{ field.label }}</dt>
              <dd>{{ field.value }}</dd>
            </div>
          </dl>
        </article>
      </div>
    </Transition>
  </main>
</template>
