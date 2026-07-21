<script setup lang="ts">
import { ArrowLeft, Check, LoaderCircle, Send } from '@lucide/vue'
import { submissionTemplate } from '~/utils/exhibition'

const config = useRuntimeConfig()
const body = ref(submissionTemplate)
const pendingMedia = ref<Array<{ id: string, type: 'image' | 'video', file: File }>>([])
const website = ref('')
const submitting = ref(false)
const editorUploading = ref(false)
const editorNotice = ref('')
const errorMessage = ref('')
const completed = ref(false)

useSeoMeta({ title: '전시 제보 · 전지적관람시점', robots: 'noindex, nofollow' })

function updatePendingMedia(media: Array<{ id: string, type: 'image' | 'video', file: File }>) {
  pendingMedia.value = media
}

async function submitExhibition() {
  submitting.value = true
  errorMessage.value = ''
  try {
    const form = new FormData()
    form.append('body_markdown', body.value)
    form.append('website', website.value)
    for (const media of pendingMedia.value) {
      form.append(`inline_${media.type}_${media.id}`, media.file, media.file.name)
    }
    await $fetch(`${config.public.apiBase}/submissions`, {
      method: 'POST',
      body: form,
    })
    completed.value = true
  } catch (error: any) {
    errorMessage.value = error?.data?.error || '제보를 접수하지 못했습니다. 잠시 후 다시 시도해 주세요.'
  } finally {
    submitting.value = false
  }
}

</script>

<template>
  <main class="submission-room">
    <header class="submission-header">
      <NuxtLink to="/" class="quiet-link"><ArrowLeft :size="17" /> 전시 목록</NuxtLink>
      <span>POV</span>
    </header>

    <section v-if="completed" class="submission-complete" aria-live="polite">
      <span class="complete-mark"><Check :size="24" /></span>
      <p class="eyebrow">THANK YOU</p>
      <h1>전시 정보를 남겨주셨습니다.</h1>
      <p>운영자가 내용을 확인한 뒤 전시 목록에 조용히 더하겠습니다.</p>
      <NuxtLink to="/" class="pill-button secondary">목록으로 돌아가기</NuxtLink>
    </section>

    <form v-else class="submission-form" @submit.prevent="submitExhibition">
      <div class="submission-intro">
        <p class="eyebrow">ADD A SCENE</p>
        <h1>당신의 관람시점</h1>
      </div>

      <ExhibitionBlockEditor
        v-model="body"
        submission
        @pending-media="updatePendingMedia"
        @uploading="editorUploading = $event"
        @notice="editorNotice = $event"
      />

      <label class="submission-honeypot" aria-hidden="true">
        홈페이지
        <input v-model="website" tabindex="-1" autocomplete="off">
      </label>

      <p class="submission-note">{{ editorNotice || '본문의 첫 이미지가 대표로 사용됩니다. 제보는 운영자 확인 후 공개됩니다.' }}</p>
      <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

      <button class="pill-button submission-button" type="submit" :disabled="submitting || editorUploading">
        <LoaderCircle v-if="submitting" :size="18" class="spin" />
        <template v-else><Send :size="17" /> 당신의 관람시점 올리기</template>
      </button>
    </form>
  </main>
</template>
