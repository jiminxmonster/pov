<script setup lang="ts">
import { ArrowLeft, Check, FileUp, ImagePlus, LoaderCircle, LogOut, Send } from '@lucide/vue'
import type { ExhibitionPost, SearchResponse } from '~/types/post'

const template = `전시명:

작가(작가소개):

관람료:

전시기간:

장소:

도슨트(전시장 가이드) 유무:

찾아가는 방법:

주차정보:

전시내용:

굿즈샵정보:

주변에 함께 볼 만한 전시:

주변에 볼거리:

맛집:

감상평:

페르소나 정보입력:
`

const config = useRuntimeConfig()
const router = useRouter()
const body = ref(template)
const imageUrl = ref('')
const posts = ref<ExhibitionPost[]>([])
const saving = ref(false)
const uploading = ref(false)
const notice = ref('')

useSeoMeta({ title: '관리자 · 전지적관람시점', robots: 'noindex, nofollow' })

async function loadAdminPosts() {
  try {
    const result = await $fetch<SearchResponse>(`${config.public.apiBase}/admin/posts`, {
      credentials: 'include',
    })
    posts.value = result.items
  } catch {
    await router.replace('/admin/login')
  }
}

async function uploadImage(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploading.value = true
  notice.value = ''
  try {
    const form = new FormData()
    form.append('file', file)
    const result = await $fetch<{ url: string }>(`${config.public.apiBase}/admin/media`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
    imageUrl.value = result.url
    notice.value = '대표 이미지가 준비됐습니다.'
  } finally {
    uploading.value = false
    input.value = ''
  }
}

async function importDocument(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploading.value = true
  notice.value = ''
  try {
    const form = new FormData()
    form.append('file', file)
    const result = await $fetch<{ body_markdown: string; message: string }>(`${config.public.apiBase}/admin/uploads`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
    body.value = result.body_markdown
    notice.value = result.message
  } finally {
    uploading.value = false
    input.value = ''
  }
}

async function save(publish: boolean) {
  saving.value = true
  notice.value = ''
  try {
    await $fetch(`${config.public.apiBase}/admin/posts`, {
      method: 'POST',
      credentials: 'include',
      body: { body_markdown: body.value, image_url: imageUrl.value, publish },
    })
    notice.value = publish ? '지도에 게시했습니다.' : '초안으로 저장했습니다.'
    body.value = template
    imageUrl.value = ''
    await loadAdminPosts()
  } finally {
    saving.value = false
  }
}

async function logout() {
  await $fetch(`${config.public.apiBase}/admin/auth/logout`, {
    method: 'POST',
    credentials: 'include',
  })
  await router.push('/')
}

onMounted(loadAdminPosts)
</script>

<template>
  <main class="admin-shell">
    <header class="admin-header">
      <NuxtLink to="/" class="quiet-link"><ArrowLeft :size="17" /> 공개 화면</NuxtLink>
      <strong>POV ADMIN</strong>
      <button class="quiet-link" type="button" @click="logout"><LogOut :size="17" /> 로그아웃</button>
    </header>

    <section class="admin-editor" aria-labelledby="editor-title">
      <div class="admin-editor-heading">
        <div>
          <p class="eyebrow">ONE PAGE EDITOR</p>
          <h1 id="editor-title">하나의 글로 기록하기</h1>
          <p>양식 사이에 내용을 채우면 검색 인덱스와 지도 위치를 자동으로 준비합니다.</p>
        </div>
        <div class="upload-actions">
          <label class="tool-button">
            <FileUp :size="17" /> 자료 불러오기
            <input type="file" accept=".txt,.md,.csv,.xlsx,.docx,.pdf" @change="importDocument">
          </label>
          <label class="tool-button">
            <ImagePlus :size="17" /> 대표 사진
            <input type="file" accept="image/*" @change="uploadImage">
          </label>
        </div>
      </div>

      <div v-if="imageUrl" class="image-preview">
        <img :src="imageUrl" alt="업로드한 대표 이미지">
        <span><Check :size="15" /> 대표 이미지</span>
      </div>

      <textarea v-model="body" class="document-editor" aria-label="공연·전시 게시글 본문" spellcheck="true" />

      <div class="editor-footer">
        <p class="editor-notice">
          <LoaderCircle v-if="saving || uploading" :size="16" class="spin" />
          <span v-else>{{ notice || '게시 전 주소와 자동 추출 결과를 확인해 주세요.' }}</span>
        </p>
        <div class="editor-buttons">
          <button class="pill-button secondary" type="button" :disabled="saving" @click="save(false)">초안 저장</button>
          <button class="pill-button" type="button" :disabled="saving" @click="save(true)"><Send :size="17" /> 게시하기</button>
        </div>
      </div>
    </section>

    <section class="admin-list" aria-labelledby="post-list-title">
      <p class="eyebrow">RECENT POSTS</p>
      <h2 id="post-list-title">최근 기록</h2>
      <div class="admin-table">
        <div v-for="post in posts" :key="post.id" class="admin-row">
          <div>
            <strong>{{ post.title }}</strong>
            <small>{{ post.address || '장소 확인 필요' }}</small>
          </div>
          <span class="status-chip">{{ post.status }}</span>
        </div>
      </div>
    </section>
  </main>
</template>
