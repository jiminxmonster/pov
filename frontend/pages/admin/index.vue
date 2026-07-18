<script setup lang="ts">
import { ArrowLeft, Check, FileUp, ImagePlus, KeyRound, LoaderCircle, LogOut, RefreshCw, Save, Send } from '@lucide/vue'
import type { ExhibitionPost, SearchResponse } from '~/types/post'
import { exhibitionTemplate } from '~/utils/exhibition'

const config = useRuntimeConfig()
const router = useRouter()
const body = ref(exhibitionTemplate)
const documentEditor = ref<HTMLTextAreaElement | null>(null)
const insertionCursor = ref<number | null>(null)
const imageUrl = ref('')
const posts = ref<ExhibitionPost[]>([])
const saving = ref(false)
const uploading = ref(false)
const notice = ref('')
const publicDataKey = ref('')
const publicDataLimit = ref(5)
const publicDataMaskedKey = ref('')
const publicDataStorage = ref('environment')
const settingsSaving = ref(false)
const settingsSyncing = ref(false)
const settingsNotice = ref('')
const aiAPIKey = ref('')
const aiModel = ref('nvidia/nemotron-3-nano-30b-a3b')
const aiMaskedKey = ref('')
const aiConfigured = ref(false)
const aiSaving = ref(false)
const aiTesting = ref(false)
const aiNotice = ref('')

interface PublicDataSettingsResponse {
  configured: boolean
  masked_key: string
  limit: number
  storage: 'environment' | 'database'
  synced_count?: number
  message?: string
}

interface AISettingsResponse {
  configured: boolean
  masked_key: string
  model: string
  endpoint: string
  storage: 'default' | 'database'
  message?: string
}

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

function applyPublicDataSettings(result: PublicDataSettingsResponse) {
  publicDataMaskedKey.value = result.masked_key
  publicDataLimit.value = result.limit
  publicDataStorage.value = result.storage
}

function apiErrorMessage(error: unknown, fallback: string) {
  return (error as { data?: { error?: string } })?.data?.error || fallback
}

async function loadPublicDataSettings() {
  try {
    const result = await $fetch<PublicDataSettingsResponse>(`${config.public.apiBase}/admin/settings/public-data`, {
      credentials: 'include',
    })
    applyPublicDataSettings(result)
  } catch (error) {
    settingsNotice.value = apiErrorMessage(error, '공공데이터 설정을 불러오지 못했습니다.')
  }
}

async function savePublicDataSettings() {
  settingsSaving.value = true
  settingsNotice.value = ''
  try {
    const result = await $fetch<PublicDataSettingsResponse>(`${config.public.apiBase}/admin/settings/public-data`, {
      method: 'PUT',
      credentials: 'include',
      body: { api_key: publicDataKey.value, limit: publicDataLimit.value },
    })
    applyPublicDataSettings(result)
    publicDataKey.value = ''
    settingsNotice.value = `${result.message} ${result.synced_count || 0}건을 반영했습니다.`
    await loadAdminPosts()
  } catch (error) {
    settingsNotice.value = apiErrorMessage(error, '인증키를 저장하지 못했습니다.')
  } finally {
    settingsSaving.value = false
  }
}

async function syncPublicData() {
  settingsSyncing.value = true
  settingsNotice.value = ''
  try {
    const result = await $fetch<PublicDataSettingsResponse>(`${config.public.apiBase}/admin/settings/public-data/sync`, {
      method: 'POST',
      credentials: 'include',
    })
    applyPublicDataSettings(result)
    settingsNotice.value = `${result.message} ${result.synced_count || 0}건을 반영했습니다.`
    await loadAdminPosts()
  } catch (error) {
    settingsNotice.value = apiErrorMessage(error, '공공데이터를 동기화하지 못했습니다.')
  } finally {
    settingsSyncing.value = false
  }
}

function applyAISettings(result: AISettingsResponse) {
  aiConfigured.value = result.configured
  aiMaskedKey.value = result.masked_key
  aiModel.value = result.model
}

async function loadAISettings() {
  try {
    const result = await $fetch<AISettingsResponse>(`${config.public.apiBase}/admin/settings/ai`, {
      credentials: 'include',
    })
    applyAISettings(result)
  } catch (error) {
    aiNotice.value = apiErrorMessage(error, 'NVIDIA AI 설정을 불러오지 못했습니다.')
  }
}

async function saveAISettings() {
  aiSaving.value = true
  aiNotice.value = ''
  try {
    const result = await $fetch<AISettingsResponse>(`${config.public.apiBase}/admin/settings/ai`, {
      method: 'PUT',
      credentials: 'include',
      body: { api_key: aiAPIKey.value, model: aiModel.value },
    })
    applyAISettings(result)
    aiAPIKey.value = ''
    aiNotice.value = result.message || 'NVIDIA AI 설정을 저장했습니다.'
  } catch (error) {
    aiNotice.value = apiErrorMessage(error, 'NVIDIA AI 설정을 저장하지 못했습니다.')
  } finally {
    aiSaving.value = false
  }
}

async function testAISettings() {
  aiTesting.value = true
  aiNotice.value = ''
  try {
    const result = await $fetch<AISettingsResponse>(`${config.public.apiBase}/admin/settings/ai/test`, {
      method: 'POST',
      credentials: 'include',
    })
    applyAISettings(result)
    aiNotice.value = result.message || 'NVIDIA AI 연결이 정상입니다.'
  } catch (error) {
    aiNotice.value = apiErrorMessage(error, 'NVIDIA AI에 연결하지 못했습니다.')
  } finally {
    aiTesting.value = false
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

function rememberEditorCursor() {
  if (!documentEditor.value) return
  insertionCursor.value = documentEditor.value.selectionStart
}

async function uploadInlineImage(event: Event) {
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

    const cursor = Math.min(insertionCursor.value ?? body.value.length, body.value.length)
    const before = body.value.slice(0, cursor)
    const after = body.value.slice(cursor)
    const leadingBreak = before && !before.endsWith('\n') ? '\n' : ''
    const trailingBreak = after && !after.startsWith('\n') ? '\n' : ''
    const insertion = `${leadingBreak}![전시 본문 이미지](${result.url})\n${trailingBreak}`
    body.value = `${before}${insertion}${after}`
    insertionCursor.value = before.length + insertion.length
    notice.value = '선택한 위치에 본문 이미지를 넣었습니다.'

    await nextTick()
    documentEditor.value?.focus()
    documentEditor.value?.setSelectionRange(insertionCursor.value, insertionCursor.value)
  } catch (error) {
    notice.value = apiErrorMessage(error, '본문 이미지를 올리지 못했습니다.')
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
    insertionCursor.value = null
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
    notice.value = publish ? '전시 목록에 게시했습니다.' : '초안으로 저장했습니다.'
    body.value = exhibitionTemplate
    insertionCursor.value = null
    imageUrl.value = ''
    await loadAdminPosts()
  } finally {
    saving.value = false
  }
}

async function publishPost(post: ExhibitionPost) {
  saving.value = true
  notice.value = ''
  try {
    await $fetch(`${config.public.apiBase}/admin/posts/${post.id}/publish`, {
      method: 'POST',
      credentials: 'include',
    })
    notice.value = `‘${post.title}’을 공개했습니다.`
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

onMounted(() => {
  loadAdminPosts()
  loadPublicDataSettings()
  loadAISettings()
})
</script>

<template>
  <main class="admin-shell">
    <header class="admin-header">
      <NuxtLink to="/" class="quiet-link"><ArrowLeft :size="17" /> 공개 화면</NuxtLink>
      <strong>POV ADMIN</strong>
      <button class="quiet-link" type="button" @click="logout"><LogOut :size="17" /> 로그아웃</button>
    </header>

    <section class="admin-settings" aria-labelledby="public-data-title">
      <div class="admin-settings-copy">
        <p class="eyebrow">DATA CONNECTION</p>
        <h1 id="public-data-title">공공 전시 데이터 인증키</h1>
        <p>서울 열린데이터광장 인증키를 저장하면 전시 정보를 지도와 목록에 바로 반영합니다.</p>
        <span v-if="publicDataMaskedKey" class="settings-key-status">
          <KeyRound :size="15" /> {{ publicDataMaskedKey }} · 최대 {{ publicDataLimit }}건 · {{ publicDataStorage === 'database' ? '운영자 저장' : '서버 기본값' }}
        </span>
      </div>

      <form class="admin-settings-form" @submit.prevent="savePublicDataSettings">
        <label class="settings-key-field">
          <span>서울시 API 인증키</span>
          <input
            v-model="publicDataKey"
            type="password"
            autocomplete="off"
            :placeholder="publicDataMaskedKey ? `새 인증키 입력 · 현재 ${publicDataMaskedKey}` : '인증키 입력'"
          >
        </label>
        <label class="settings-limit-field">
          <span>가져올 전시 수</span>
          <input v-model.number="publicDataLimit" type="number" min="1" max="1000" inputmode="numeric">
        </label>
        <div class="settings-actions">
          <button class="pill-button secondary" type="button" :disabled="settingsSaving || settingsSyncing" @click="syncPublicData">
            <LoaderCircle v-if="settingsSyncing" :size="17" class="spin" />
            <RefreshCw v-else :size="17" /> 지금 동기화
          </button>
          <button class="pill-button" type="submit" :disabled="settingsSaving || settingsSyncing">
            <LoaderCircle v-if="settingsSaving" :size="17" class="spin" />
            <Save v-else :size="17" /> 저장하고 동기화
          </button>
        </div>
        <p class="settings-notice" aria-live="polite">{{ settingsNotice || '입력한 인증키는 암호화되어 저장되며 화면에는 다시 표시되지 않습니다.' }}</p>
      </form>
    </section>

    <section class="admin-settings ai-settings" aria-labelledby="ai-settings-title">
      <div class="admin-settings-copy">
        <p class="eyebrow">AI CURATION</p>
        <h1 id="ai-settings-title">NVIDIA 전시 큐레이터</h1>
        <p>자연어 질문을 해석하고 현재 등록된 전시 안에서 추천 순서와 이유를 만듭니다.</p>
        <span v-if="aiConfigured" class="settings-key-status ai-connected">
          <KeyRound :size="15" /> 연결됨 · {{ aiMaskedKey }} · {{ aiModel }}
        </span>
      </div>

      <form class="admin-settings-form ai-settings-form" @submit.prevent="saveAISettings">
        <label>
          <span>NVIDIA API 키</span>
          <input
            v-model="aiAPIKey"
            type="password"
            autocomplete="off"
            :placeholder="aiMaskedKey ? `새 API 키 입력 · 현재 ${aiMaskedKey}` : 'nvapi-로 시작하는 API 키 입력'"
          >
        </label>
        <label>
          <span>NVIDIA 모델</span>
          <input v-model="aiModel" list="nvidia-models" autocomplete="off" spellcheck="false">
          <datalist id="nvidia-models">
            <option value="nvidia/nemotron-3-nano-30b-a3b">빠른 기본 모델</option>
            <option value="nvidia/nemotron-3-super-120b-a12b">균형형 모델</option>
            <option value="nvidia/nemotron-3-ultra-550b-a55b">고품질 모델</option>
          </datalist>
        </label>
        <div class="settings-actions">
          <button class="pill-button secondary" type="button" :disabled="!aiConfigured || aiSaving || aiTesting" @click="testAISettings">
            <LoaderCircle v-if="aiTesting" :size="17" class="spin" />
            <RefreshCw v-else :size="17" /> 연결 테스트
          </button>
          <button class="pill-button" type="submit" :disabled="aiSaving || aiTesting">
            <LoaderCircle v-if="aiSaving" :size="17" class="spin" />
            <Save v-else :size="17" /> 저장 및 연결
          </button>
        </div>
        <p class="settings-notice" aria-live="polite">{{ aiNotice || 'API 키는 서버에서 암호화 저장되며 공개 화면이나 브라우저 코드에 포함되지 않습니다.' }}</p>
        <p class="settings-notice trial-notice">NVIDIA 무료 엔드포인트는 개발·테스트용입니다. 정식 공개 운영 전 프로덕션 이용 조건을 확인해 주세요.</p>
      </form>
    </section>

    <section class="admin-editor" aria-labelledby="editor-title">
      <div class="admin-editor-heading">
        <div>
          <p class="eyebrow">ONE PAGE EDITOR</p>
          <h1 id="editor-title">하나의 글로 기록하기</h1>
          <p>양식 사이에 내용을 채우면 검색 인덱스와 장소 정보를 자동으로 준비합니다.</p>
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
          <label class="tool-button">
            <ImagePlus :size="17" /> 본문 이미지
            <input type="file" accept="image/*" @change="uploadInlineImage">
          </label>
        </div>
      </div>

      <div v-if="imageUrl" class="image-preview">
        <img :src="imageUrl" alt="업로드한 대표 이미지">
        <span><Check :size="15" /> 대표 이미지</span>
      </div>

      <textarea
        ref="documentEditor"
        v-model="body"
        class="document-editor"
        aria-label="공연·전시 게시글 본문"
        spellcheck="true"
        @click="rememberEditorCursor"
        @keyup="rememberEditorCursor"
        @select="rememberEditorCursor"
      />

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
      <h2 id="post-list-title">최근 기록과 제보</h2>
      <div class="admin-table">
        <details v-for="post in posts" :key="post.id" class="admin-entry">
          <summary class="admin-row">
            <div>
              <strong>{{ post.title }}</strong>
              <small>{{ post.address || '장소 확인 필요' }}</small>
            </div>
            <div class="admin-row-chips">
              <span v-if="post.source_type === 'community'" class="status-chip">사용자 제보</span>
              <span class="status-chip">{{ post.status }}</span>
            </div>
          </summary>
          <div class="admin-review-body">
            <img v-if="post.image_url" :src="post.image_url" :alt="`${post.title} 대표 이미지`">
            <pre>{{ post.body_markdown }}</pre>
            <button
              v-if="post.status !== 'published'"
              class="pill-button"
              type="button"
              :disabled="saving"
              @click="publishPost(post)"
            ><Send :size="16" /> 확인 후 공개</button>
          </div>
        </details>
      </div>
    </section>
  </main>
</template>
