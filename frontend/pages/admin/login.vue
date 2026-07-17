<script setup lang="ts">
import { ArrowRight, LoaderCircle } from '@lucide/vue'

const config = useRuntimeConfig()
const router = useRouter()
const username = ref('')
const password = ref('')
const loading = ref(false)
const errorMessage = ref('')

useSeoMeta({ title: '운영자 로그인 · 전지적관람시점', robots: 'noindex, nofollow' })

async function login() {
  loading.value = true
  errorMessage.value = ''
  try {
    await $fetch(`${config.public.apiBase}/admin/auth/login`, {
      method: 'POST',
      credentials: 'include',
      body: { username: username.value, password: password.value },
    })
    await router.push('/admin')
  } catch {
    errorMessage.value = '로그인 정보를 다시 확인해 주세요.'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="admin-login-room">
    <NuxtLink to="/" class="admin-brand" aria-label="첫 화면으로">
      <img src="/logo.png" alt="" class="admin-brand-image">
      <span>전지적관람시점</span>
    </NuxtLink>

    <form class="admin-login-form" @submit.prevent="login">
      <div>
        <p class="eyebrow">PRIVATE ROOM</p>
        <h1>운영자 로그인</h1>
      </div>

      <label>
        <span>아이디</span>
        <input v-model="username" autocomplete="username" required>
      </label>
      <label>
        <span>비밀번호</span>
        <input v-model="password" type="password" autocomplete="current-password" required>
      </label>

      <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

      <button class="pill-button" type="submit" :disabled="loading">
        <LoaderCircle v-if="loading" :size="18" class="spin" />
        <template v-else>로그인 <ArrowRight :size="18" /></template>
      </button>
    </form>
  </main>
</template>
