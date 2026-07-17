<script setup lang="ts">
import { ArrowLeft, Check, ImagePlus, LoaderCircle, Send } from '@lucide/vue'
import { exhibitionTemplate } from '~/utils/exhibition'

const config = useRuntimeConfig()
const body = ref(exhibitionTemplate)
const image = ref<File | null>(null)
const imagePreview = ref('')
const website = ref('')
const submitting = ref(false)
const errorMessage = ref('')
const completed = ref(false)

useSeoMeta({ title: '전시 제보 · 전지적관람시점', robots: 'noindex, nofollow' })

function selectImage(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0] || null
  image.value = file
  if (imagePreview.value) URL.revokeObjectURL(imagePreview.value)
  imagePreview.value = file ? URL.createObjectURL(file) : ''
}

async function submitExhibition() {
  submitting.value = true
  errorMessage.value = ''
  try {
    const form = new FormData()
    form.append('body_markdown', body.value)
    form.append('website', website.value)
    if (image.value) form.append('image', image.value)
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

onBeforeUnmount(() => {
  if (imagePreview.value) URL.revokeObjectURL(imagePreview.value)
})
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
        <h1>알고 있는 전시를<br>남겨주세요.</h1>
        <p>비어 있는 항목은 그대로 두어도 됩니다. 전시명과 장소는 꼭 적어주세요.</p>
      </div>

      <label class="submission-image-picker">
        <ImagePlus :size="18" />
        <span>{{ image ? image.name : '대표 사진 선택 · 선택사항' }}</span>
        <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" @change="selectImage">
      </label>

      <img v-if="imagePreview" :src="imagePreview" alt="선택한 대표 사진 미리보기" class="submission-image-preview">

      <label class="submission-body-label" for="submission-body">전시 정보</label>
      <textarea id="submission-body" v-model="body" class="submission-editor" required spellcheck="true" />

      <label class="submission-honeypot" aria-hidden="true">
        홈페이지
        <input v-model="website" tabindex="-1" autocomplete="off">
      </label>

      <p class="submission-note">제보 내용과 사진은 운영자 확인 후 공개됩니다.</p>
      <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

      <button class="pill-button submission-button" type="submit" :disabled="submitting">
        <LoaderCircle v-if="submitting" :size="18" class="spin" />
        <template v-else><Send :size="17" /> 제보 보내기</template>
      </button>
    </form>
  </main>
</template>
