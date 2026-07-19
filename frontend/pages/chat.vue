<script setup lang="ts">
import { ArrowUp, ExternalLink, LoaderCircle, MapPin } from '@lucide/vue'
import type { AIConversationTurn, SearchResponse } from '~/types/post'

interface ChatMessage extends AIConversationTurn {
  id: string
}

const config = useRuntimeConfig()
const route = useRoute()
const router = useRouter()
const logoUrl = `${config.app.baseURL}logo.png`
const chatInitialKey = 'pov-chat-initial-v1'
const searchResultKey = 'pov-search-result-v1'
const initialQuery = computed(() => typeof route.query.q === 'string' ? route.query.q.trim() : '')
const messages = ref<ChatMessage[]>([])
const history = ref<AIConversationTurn[]>([])
const latest = ref<SearchResponse | null>(null)
const draft = ref('')
const loading = ref(false)
const notice = ref('')
const transcript = ref<HTMLElement | null>(null)

useSeoMeta({
  title: 'POV AI · 전지적관람시점',
  description: '대화로 관람 조건을 좁히고 전시 정보와 지도를 이어서 확인하세요.',
})

function messageID(role: string) {
  return `${role}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function responseText(result: SearchResponse) {
  return [result.interpretation, result.question].filter(Boolean).join('\n')
}

function addMessage(role: 'user' | 'assistant', content: string) {
  const clean = content.trim()
  if (!clean) return
  messages.value.push({ id: messageID(role), role, content: clean })
}

async function scrollToLatest() {
  await nextTick()
  transcript.value?.scrollTo({ top: transcript.value.scrollHeight, behavior: 'smooth' })
}

async function ask(value: string) {
  const text = value.trim()
  if (!text || loading.value) return

  const priorHistory = [...history.value]
  addMessage('user', text)
  history.value.push({ role: 'user', content: text })
  draft.value = ''
  latest.value = null
  notice.value = ''
  loading.value = true
  await scrollToLatest()

  try {
    const result = await $fetch<SearchResponse>(`${config.public.apiBase}/search/ai`, {
      method: 'POST',
      body: { query: text, history: priorHistory },
    })
    latest.value = result
    const assistantText = responseText(result) || '조금 다른 말로 다시 질문해 주세요.'
    addMessage('assistant', assistantText)
    history.value.push({ role: 'assistant', content: assistantText })
  } catch {
    notice.value = '지금은 답변을 이어가기 어렵습니다. 잠시 뒤 다시 이야기해 주세요.'
  } finally {
    loading.value = false
    await scrollToLatest()
  }
}

function chooseOption(option: string) {
  ask(option)
}

async function openView(target: 'map' | 'list') {
  if (latest.value?.items?.length) {
    sessionStorage.setItem(searchResultKey, JSON.stringify({
      query: history.value.filter(turn => turn.role === 'user').map(turn => turn.content).join(' · '),
      result: latest.value,
      target,
    }))
  }
  await router.push({ path: '/', query: { view: target }, hash: `#${target}` })
}

async function initializeChat() {
  const stored = sessionStorage.getItem(chatInitialKey)
  if (stored) {
    sessionStorage.removeItem(chatInitialKey)
    try {
      const payload = JSON.parse(stored) as { query: string; result: SearchResponse }
      if (payload.query && payload.result) {
        latest.value = payload.result
        addMessage('user', payload.query)
        const assistantText = responseText(payload.result)
        addMessage('assistant', assistantText)
        history.value = [
          { role: 'user', content: payload.query },
          { role: 'assistant', content: assistantText },
        ]
        await scrollToLatest()
        return
      }
    } catch {
      // Ignore stale browser state and ask the query again.
    }
  }
  if (initialQuery.value) {
    await ask(initialQuery.value)
  }
}

onMounted(initializeChat)
</script>

<template>
  <main class="ai-room">
    <header class="ai-room-header">
      <NuxtLink to="/" class="ai-room-brand" aria-label="전지적관람시점 메인">
        <img :src="logoUrl" alt="POV">
      </NuxtLink>
      <p>POV AI</p>
    </header>

    <section ref="transcript" class="ai-transcript" aria-live="polite" aria-label="POV AI 대화">
      <div v-if="messages.length === 0 && !loading" class="ai-empty-state">
        <p class="eyebrow">A QUIET GUIDE</p>
        <h1>어떤 관람을<br>찾고 있나요?</h1>
        <p>한 문장으로 시작해도 충분합니다.</p>
      </div>

      <article
        v-for="message in messages"
        :key="message.id"
        class="ai-message"
        :class="`is-${message.role}`"
      >
        <span>{{ message.role === 'assistant' ? 'pov' : 'you' }}</span>
        <p>{{ message.content }}</p>
      </article>

      <div v-if="latest?.mode === 'wizard' && latest.options?.length" class="ai-wizard-options" aria-label="답변 선택">
        <button v-for="option in latest.options" :key="option" type="button" @click="chooseOption(option)">
          {{ option }}
        </button>
      </div>

      <div v-if="latest?.items?.length" class="ai-related" aria-label="관련 전시">
        <p class="eyebrow">RELATED EXHIBITIONS</p>
        <button v-for="post in latest.items.slice(0, 4)" :key="post.id" type="button" @click="openView('map')">
          <span>{{ post.title }}</span>
          <small><MapPin :size="13" /> {{ post.address || '장소 확인 중' }}</small>
        </button>
      </div>

      <div v-if="latest?.links?.length" class="ai-source-links" aria-label="관련 링크">
        <a v-for="link in latest.links" :key="link.url" :href="link.url" target="_blank" rel="noopener noreferrer">
          {{ link.label }} <ExternalLink :size="13" />
        </a>
      </div>

      <button
        v-if="latest?.mode === 'map' && latest.items.length"
        class="ai-map-action"
        type="button"
        @click="openView('map')"
      >
        지도에서 추천 보기
      </button>

      <p v-if="loading" class="ai-thinking"><LoaderCircle :size="17" class="spin" /> 장면을 고르는 중</p>
      <p v-if="notice" class="ai-notice">{{ notice }}</p>
    </section>

    <footer class="ai-dock">
      <nav class="ai-tabs" aria-label="주요 화면">
        <NuxtLink to="/">main</NuxtLink>
        <button type="button" @click="openView('map')">map</button>
        <button type="button" @click="openView('list')">list</button>
      </nav>

      <form class="ai-composer" @submit.prevent="ask(draft)">
        <input v-model="draft" autocomplete="off" aria-label="AI에게 질문하기" placeholder="계속 이야기해 주세요">
        <button type="submit" :disabled="loading || !draft.trim()" aria-label="보내기">
          <LoaderCircle v-if="loading" :size="18" class="spin" />
          <ArrowUp v-else :size="19" />
        </button>
      </form>
    </footer>
  </main>
</template>
