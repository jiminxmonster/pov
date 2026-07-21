<script setup lang="ts">
import { ChevronDown, ChevronUp, ImageIcon, LoaderCircle, Plus, Trash2, Video, X } from '@lucide/vue'
import { exhibitionLabels, parseExhibitionContent, parseExhibitionFields } from '~/utils/exhibition'

type MediaKind = 'image' | 'video'

interface TextBlock {
  id: string
  type: 'text'
  text: string
}

interface MediaBlock {
  id: string
  type: MediaKind
  url: string
  source: string
  caption: string
  file?: File
  localURL?: boolean
}

type EditorBlock = TextBlock | MediaBlock

interface EditorField {
  label: string
  blocks: EditorBlock[]
  period?: {
    startDate: string
    startTime: string
    endDate: string
    endTime: string
  }
}

interface PendingEditorMedia {
  id: string
  type: MediaKind
  file: File
}

const props = defineProps<{
  modelValue: string
  uploadMedia?: (file: File, type: MediaKind) => Promise<{ url: string }>
  submission?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'pending-media': [media: PendingEditorMedia[]]
  'uploading': [value: boolean]
  'notice': [message: string]
}>()

let nextID = 0
let editorObserver: IntersectionObserver | undefined
const cursorByBlock = new Map<string, number>()
const editorRoot = ref<HTMLElement | null>(null)
const mediaDock = ref<HTMLElement | null>(null)
const activeTextID = ref('')
const menuOpen = ref(false)
const uploading = ref(false)
const dockVisible = ref(false)
const dockLeft = ref(18)

const submissionBaseLabels = ['전시내용']
const submissionCategories = [
  { label: '전시명', title: '전시명' },
  { label: '장소', title: '장소' },
  { label: '전시기간', title: '전시기간 및 시간' },
  { label: '링크', title: '링크 넣기' },
  { label: '작가(작가소개)', title: '작가소개' },
  { label: '관람료', title: '관람료' },
  { label: '맛집', title: '주변맛집' },
  { label: '찾아가는 방법', title: '교통' },
  { label: '주차정보', title: '주차' },
  { label: '도슨트(전시장 가이드) 유무', title: '도슨트 유무' },
]
const fields = ref<EditorField[]>(parseDocument(props.modelValue))

function newID(prefix: string) {
  nextID += 1
  return `${prefix}-${Date.now().toString(36)}-${nextID}`
}

function parseDocument(body: string): EditorField[] {
  const values = new Map(parseExhibitionFields(body).map(field => [field.label, field.value]))
  const labels = props.submission
    ? exhibitionLabels.filter(label => submissionBaseLabels.includes(label) || body.includes(`${label}:`))
    : exhibitionLabels
  return labels.map((label, fieldIndex) => {
    const value = values.get(label) || ''
    const blocks: EditorBlock[] = parseExhibitionContent(value).map((segment, segmentIndex) => {
      if (segment.type === 'image' || segment.type === 'video') {
        return {
          id: `field-${fieldIndex}-${segment.type}-${segmentIndex}`,
          type: segment.type,
          url: segment.url,
          source: segment.url,
          caption: segment.alt,
        }
      }
      return {
        id: `field-${fieldIndex}-text-${segmentIndex}`,
        type: 'text',
        text: segment.value,
      }
    })
    if (!blocks.length || blocks[blocks.length - 1]?.type !== 'text') {
      blocks.push({ id: `field-${fieldIndex}-text-end`, type: 'text', text: '' })
    }
    return {
      label,
      blocks,
      period: label === '전시기간' ? parsePeriod(value) : undefined,
    }
  })
}

function parsePeriod(value: string) {
  const matches = [...value.matchAll(/(\d{4}-\d{2}-\d{2})(?:\s+(\d{2}:\d{2}))?/g)]
  return {
    startDate: matches[0]?.[1] || '',
    startTime: matches[0]?.[2] || '',
    endDate: matches[1]?.[1] || '',
    endTime: matches[1]?.[2] || '',
  }
}

function serializePeriod(field: EditorField) {
  const period = field.period
  if (!period) return ''
  const start = [period.startDate, period.startTime].filter(Boolean).join(' ')
  const end = [period.endDate, period.endTime].filter(Boolean).join(' ')
  return [start, end].filter(Boolean).join(' ~ ')
}

function serializeDocument() {
  return `${fields.value.map((field) => {
    const value = field.label === '전시기간' && props.submission ? serializePeriod(field) : field.blocks
      .map((block) => {
        if (block.type === 'text') return block.text.trim()
        const marker = block.type === 'image' ? '!' : '@'
        return `${marker}[${sanitizeCaption(block.caption, block.type)}](${block.source})`
      })
      .filter(Boolean)
      .join('\n\n')
    return `${field.label}:\n${value}`
  }).join('\n\n')}\n`
}

function sanitizeCaption(value: string, type: MediaKind) {
  return value.replace(/[\[\]\r\n]/g, ' ').trim() || (type === 'image' ? '전시 본문 이미지' : '전시 본문 영상')
}

function syncDocument() {
  emit('update:modelValue', serializeDocument())
  emitPendingMedia()
}

function emitPendingMedia() {
  const media = fields.value.flatMap(field => field.blocks
    .filter((block): block is MediaBlock => block.type !== 'text' && Boolean(block.file))
    .map(block => ({ id: block.id, type: block.type, file: block.file! })))
  emit('pending-media', media)
}

function rememberCursor(block: TextBlock, event: Event) {
  activeTextID.value = block.id
  cursorByBlock.set(block.id, (event.target as HTMLTextAreaElement | HTMLInputElement).selectionStart || 0)
}

function locateInsertionTarget() {
  if (props.submission) {
    const fieldIndex = fields.value.findIndex(field => field.label === '전시내용')
    const blocks = fields.value[fieldIndex]?.blocks || []
    const blockIndex = Math.max(0, blocks.findLastIndex(block => block.type === 'text'))
    const block = blocks[blockIndex]
    if (block?.type === 'text') activeTextID.value = block.id
    return { fieldIndex: Math.max(0, fieldIndex), blockIndex }
  }

  for (let fieldIndex = 0; fieldIndex < fields.value.length; fieldIndex++) {
    const blockIndex = fields.value[fieldIndex]?.blocks.findIndex(block => block.id === activeTextID.value) ?? -1
    if (blockIndex >= 0) return { fieldIndex, blockIndex }
  }

  const fieldIndex = Math.max(0, exhibitionLabels.indexOf('전시내용'))
  const blocks = fields.value[fieldIndex]?.blocks || []
  const blockIndex = Math.max(0, blocks.findLastIndex(block => block.type === 'text'))
  const block = blocks[blockIndex]
  if (block?.type === 'text') activeTextID.value = block.id
  return { fieldIndex, blockIndex }
}

const activeFieldLabel = computed(() => {
  if (props.submission) return '본문'
  const { fieldIndex } = locateInsertionTarget()
  return fields.value[fieldIndex]?.label || '전시내용'
})

function fieldTitle(label: string) {
  if (label === '전시내용' && props.submission) return '본문'
  return submissionCategories.find(category => category.label === label)?.title || label
}

function addCategory(label: string) {
  menuOpen.value = false
  if (fields.value.some(field => field.label === label)) {
    emit('notice', `${fieldTitle(label)} 항목은 이미 추가되어 있습니다.`)
    return
  }
  fields.value.push({
    label,
    blocks: [{ id: newID('text'), type: 'text', text: '' }],
    period: label === '전시기간' ? parsePeriod('') : undefined,
  })
  syncDocument()
  emit('notice', `${fieldTitle(label)} 항목을 추가했습니다.`)
}

function removeField(fieldIndex: number) {
  const field = fields.value[fieldIndex]
  if (!field || submissionBaseLabels.includes(field.label)) return
  for (const block of field.blocks) {
    if (block.type !== 'text' && block.localURL) URL.revokeObjectURL(block.url)
  }
  fields.value.splice(fieldIndex, 1)
  syncDocument()
  emit('notice', `${fieldTitle(field.label)} 항목을 삭제했습니다.`)
}

function mediaCount(type?: MediaKind) {
  return fields.value.reduce((count, field) => count + field.blocks.filter(block => block.type !== 'text' && (!type || block.type === type)).length, 0)
}

function validateMedia(file: File, type: MediaKind) {
  if (type === 'image') {
    if (!['image/jpeg', 'image/png', 'image/webp', 'image/gif'].includes(file.type)) {
      return 'JPG, PNG, WebP 또는 GIF 이미지만 넣을 수 있습니다.'
    }
    if (file.size > 8 * 1024 * 1024) return '이미지는 장당 8MB 이하로 선택해 주세요.'
  } else {
    if (!['video/mp4', 'video/webm', 'video/quicktime'].includes(file.type)) {
      return 'MP4, WebM 또는 MOV 영상만 넣을 수 있습니다.'
    }
    if (file.size > 30 * 1024 * 1024) return '영상은 한 개당 30MB 이하로 선택해 주세요.'
    if (mediaCount('video') >= 3) return '영상은 게시글당 최대 3개까지 넣을 수 있습니다.'
  }
  if (mediaCount() >= 6) return '이미지와 영상은 합해서 최대 6개까지 넣을 수 있습니다.'
  return ''
}

async function insertMedia(type: MediaKind, event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  menuOpen.value = false
  if (!file) return

  const validationMessage = validateMedia(file, type)
  if (validationMessage) {
    emit('notice', validationMessage)
    return
  }

  const { fieldIndex, blockIndex } = locateInsertionTarget()
  const field = fields.value[fieldIndex]
  const textBlock = field?.blocks[blockIndex]
  if (!field || !textBlock || textBlock.type !== 'text') return

  uploading.value = true
  emit('uploading', true)
  try {
    const cursor = Math.min(cursorByBlock.get(textBlock.id) ?? textBlock.text.length, textBlock.text.length)
    const mediaID = newID(type === 'image' ? 'inline' : 'video')
    let mediaBlock: MediaBlock
    if (props.uploadMedia) {
      const uploaded = await props.uploadMedia(file, type)
      mediaBlock = { id: mediaID, type, url: uploaded.url, source: uploaded.url, caption: '' }
    } else {
      const localURL = URL.createObjectURL(file)
      mediaBlock = {
        id: mediaID,
        type,
        url: localURL,
        source: type === 'image' ? `pov-inline://${mediaID}` : `pov-video://${mediaID}`,
        caption: '',
        file,
        localURL: true,
      }
    }

    const before: TextBlock = { ...textBlock, text: textBlock.text.slice(0, cursor) }
    const after: TextBlock = { id: newID('text'), type: 'text', text: textBlock.text.slice(cursor) }
    field.blocks.splice(blockIndex, 1, before, mediaBlock, after)
    activeTextID.value = after.id
    cursorByBlock.set(after.id, 0)
    syncDocument()
    emit('notice', `선택한 글 위치에 ${type === 'image' ? '이미지' : '영상'}를 넣었습니다.`)
  } catch {
    emit('notice', `${type === 'image' ? '이미지' : '영상'}를 올리지 못했습니다. 잠시 후 다시 시도해 주세요.`)
  } finally {
    uploading.value = false
    emit('uploading', false)
  }
}

function moveMedia(fieldIndex: number, blockIndex: number, direction: -1 | 1) {
  const blocks = fields.value[fieldIndex]?.blocks
  const target = blockIndex + direction
  if (!blocks || target < 0 || target >= blocks.length) return
  const [block] = blocks.splice(blockIndex, 1)
  if (!block) return
  blocks.splice(target, 0, block)
  syncDocument()
}

function removeMedia(fieldIndex: number, blockIndex: number) {
  const blocks = fields.value[fieldIndex]?.blocks
  const block = blocks?.[blockIndex]
  if (!blocks || !block || block.type === 'text') return
  if (block.localURL) URL.revokeObjectURL(block.url)
  blocks.splice(blockIndex, 1)
  if (!blocks.some(item => item.type === 'text')) {
    blocks.push({ id: newID('text'), type: 'text', text: '' })
  }
  syncDocument()
  emit('notice', `본문 ${block.type === 'image' ? '이미지' : '영상'}를 삭제했습니다.`)
}

function releaseLocalMedia() {
  for (const field of fields.value) {
    for (const block of field.blocks) {
      if (block.type !== 'text' && block.localURL) URL.revokeObjectURL(block.url)
    }
  }
}

function updateDockPosition() {
  if (!editorRoot.value || !import.meta.client) return
  const rect = editorRoot.value.getBoundingClientRect()
  dockLeft.value = Math.max(18, Math.min(window.innerWidth - 70, rect.right - 58))
}

function closeMenuFromOutside(event: PointerEvent) {
  if (!mediaDock.value?.contains(event.target as Node)) menuOpen.value = false
}

function closeMenuFromEscape(event: KeyboardEvent) {
  if (event.key === 'Escape') menuOpen.value = false
}

watch(() => props.modelValue, (value) => {
  if (value === serializeDocument()) return
  releaseLocalMedia()
  fields.value = parseDocument(value)
  cursorByBlock.clear()
  activeTextID.value = ''
  emitPendingMedia()
})

onMounted(() => {
  if (!editorRoot.value) return
  if (props.submission) {
    dockVisible.value = true
  } else {
    editorObserver = new IntersectionObserver(([entry]) => {
      dockVisible.value = Boolean(entry?.isIntersecting)
      updateDockPosition()
    })
    editorObserver.observe(editorRoot.value)
    window.addEventListener('resize', updateDockPosition)
    window.addEventListener('scroll', updateDockPosition, { passive: true })
  }
  document.addEventListener('pointerdown', closeMenuFromOutside)
  document.addEventListener('keydown', closeMenuFromEscape)
  updateDockPosition()
})

onBeforeUnmount(() => {
  releaseLocalMedia()
  editorObserver?.disconnect()
  if (import.meta.client) {
    window.removeEventListener('resize', updateDockPosition)
    window.removeEventListener('scroll', updateDockPosition)
    document.removeEventListener('pointerdown', closeMenuFromOutside)
    document.removeEventListener('keydown', closeMenuFromEscape)
  }
})
</script>

<template>
  <div ref="editorRoot" class="block-editor" :class="{ 'is-submission': props.submission }">
    <section
      v-for="(field, fieldIndex) in fields"
      :key="field.label"
      class="block-field"
      :class="{ 'is-body': props.submission && field.label === '전시내용' }"
    >
      <header class="block-field-header">
        <label class="block-field-label">{{ fieldTitle(field.label) }}</label>
        <button
          v-if="props.submission && !submissionBaseLabels.includes(field.label)"
          type="button"
          class="block-field-remove"
          :aria-label="`${fieldTitle(field.label)} 항목 삭제`"
          @click="removeField(fieldIndex)"
        >
          <X :size="15" />
        </button>
      </header>

      <div v-if="props.submission && field.label === '전시기간' && field.period" class="period-grid">
        <label>
          <span>시작일</span>
          <input v-model="field.period.startDate" type="date" @input="syncDocument">
        </label>
        <label>
          <span>시작 시간</span>
          <input v-model="field.period.startTime" type="time" @input="syncDocument">
        </label>
        <label>
          <span>종료일</span>
          <input v-model="field.period.endDate" type="date" @input="syncDocument">
        </label>
        <label>
          <span>종료 시간</span>
          <input v-model="field.period.endTime" type="time" @input="syncDocument">
        </label>
      </div>

      <template v-for="(block, blockIndex) in field.blocks" v-else :key="block.id">
        <input
          v-if="block.type === 'text' && props.submission && field.label !== '전시내용'"
          v-model="block.text"
          class="block-line-input"
          :type="field.label === '링크' ? 'url' : 'text'"
          :placeholder="field.label === '링크' ? 'https://' : `${fieldTitle(field.label)}을 입력하세요.`"
          :aria-label="`${fieldTitle(field.label)} 내용`"
          @input="syncDocument"
          @focus="rememberCursor(block, $event)"
          @click="rememberCursor(block, $event)"
          @keyup="rememberCursor(block, $event)"
          @select="rememberCursor(block, $event)"
        >
        <textarea
          v-else-if="block.type === 'text'"
          v-model="block.text"
          class="block-text"
          :class="{ 'is-free-body': props.submission && field.label === '전시내용' }"
          :aria-label="`${field.label} 내용`"
          :rows="props.submission && field.label === '전시내용' ? 10 : 2"
          spellcheck="true"
          @input="syncDocument"
          @focus="rememberCursor(block, $event)"
          @click="rememberCursor(block, $event)"
          @keyup="rememberCursor(block, $event)"
          @select="rememberCursor(block, $event)"
        />

        <figure v-else class="block-media">
          <img v-if="block.type === 'image'" :src="block.url" :alt="block.caption">
          <video v-else :src="block.url" controls playsinline preload="metadata" />
          <input
            v-model="block.caption"
            class="block-caption"
            :aria-label="`${block.type === 'image' ? '이미지' : '영상'} 설명`"
            :placeholder="`${block.type === 'image' ? '이미지' : '영상'} 설명`"
            @input="syncDocument"
          >
          <div class="block-media-actions">
            <button type="button" :disabled="blockIndex === 0" :aria-label="`${block.type === 'image' ? '이미지' : '영상'} 위로 이동`" @click="moveMedia(fieldIndex, blockIndex, -1)">
              <ChevronUp :size="15" /> 위
            </button>
            <button type="button" :disabled="blockIndex === field.blocks.length - 1" :aria-label="`${block.type === 'image' ? '이미지' : '영상'} 아래로 이동`" @click="moveMedia(fieldIndex, blockIndex, 1)">
              <ChevronDown :size="15" /> 아래
            </button>
            <button type="button" :aria-label="`본문 ${block.type === 'image' ? '이미지' : '영상'} 삭제`" @click="removeMedia(fieldIndex, blockIndex)">
              <Trash2 :size="15" /> 삭제
            </button>
          </div>
        </figure>
      </template>
    </section>

    <Transition name="dock-fade">
      <div
        v-if="dockVisible"
        ref="mediaDock"
        class="media-dock"
        :class="{ 'is-embedded': props.submission }"
        :style="props.submission ? undefined : { left: `${dockLeft}px` }"
        @pointerdown.stop
      >
        <Transition name="menu-fade">
          <div v-if="menuOpen" class="media-menu" role="menu" :aria-label="props.submission ? '내용 추가' : '본문 미디어 넣기'">
            <small>{{ props.submission ? '추가할 내용을 고르세요.' : `${activeFieldLabel}의 현재 커서 위치` }}</small>
            <label role="menuitem">
              <ImageIcon :size="17" />
              <span>사진 넣기</span>
              <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" :disabled="uploading" @change="insertMedia('image', $event)">
            </label>
            <label role="menuitem">
              <Video :size="17" />
              <span>동영상 넣기</span>
              <input type="file" accept="video/mp4,video/webm,video/quicktime,.mov" :disabled="uploading" @change="insertMedia('video', $event)">
            </label>
            <template v-if="props.submission">
              <button
                v-for="category in submissionCategories"
                :key="category.label"
                type="button"
                class="category-option"
                role="menuitem"
                :disabled="fields.some(field => field.label === category.label)"
                @click="addCategory(category.label)"
              >
                <Plus :size="16" />
                <span>{{ category.title }}</span>
                <small v-if="fields.some(field => field.label === category.label)">추가됨</small>
              </button>
            </template>
          </div>
        </Transition>

        <button
          class="media-dock-button"
          type="button"
          :aria-label="menuOpen ? '추가 메뉴 닫기' : (props.submission ? '내용 추가' : '현재 커서 위치에 이미지 또는 영상 넣기')"
          :aria-expanded="menuOpen"
          :disabled="uploading"
          @click="menuOpen = !menuOpen"
        >
          <LoaderCircle v-if="uploading" :size="21" class="spin" />
          <X v-else-if="menuOpen" :size="21" />
          <Plus v-else :size="23" />
        </button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.block-editor {
  border-top: 1px solid var(--line-strong);
  background: var(--paper);
}

.block-editor.is-submission {
  position: relative;
  margin-bottom: 64px;
}

.block-field {
  padding: 22px clamp(20px, 4vw, 42px) 18px;
  border-right: 1px solid var(--line-strong);
  border-bottom: 1px solid var(--line);
  border-left: 1px solid var(--line-strong);
}

.block-field-header {
  min-height: 28px;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 8px;
}

.block-field-label {
  display: block;
  color: var(--muted);
  font-size: 12px;
  font-weight: 800;
}

.block-field-remove {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  margin-top: -7px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
}

.block-field-remove:hover {
  background: var(--canvas);
  color: var(--ink);
}

.block-text {
  width: 100%;
  min-height: 58px;
  display: block;
  resize: vertical;
  padding: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--ink-soft);
  font: inherit;
  font-size: 15px;
  line-height: 1.75;
}

.block-text:focus {
  color: var(--ink);
}

.block-text.is-free-body {
  min-height: 280px;
  color: var(--ink);
  font-size: 16px;
  line-height: 1.9;
}

.block-line-input {
  width: 100%;
  min-height: 46px;
  padding: 0;
  border: 0;
  border-bottom: 1px solid var(--line);
  outline: 0;
  background: transparent;
  color: var(--ink);
  font: inherit;
  font-size: 15px;
}

.block-line-input:focus {
  border-color: var(--ink);
}

.period-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px 18px;
}

.period-grid label {
  display: grid;
  gap: 7px;
  color: var(--muted);
  font-size: 10px;
  font-weight: 800;
}

.period-grid input {
  width: 100%;
  min-height: 42px;
  padding: 0 10px;
  border: 1px solid var(--line);
  border-radius: 0;
  outline: 0;
  background: var(--paper);
  color: var(--ink);
  font: inherit;
  font-size: 13px;
}

.period-grid input:focus {
  border-color: var(--ink);
}

.block-media {
  margin: 12px 0 20px;
  padding: 14px;
  border: 1px solid var(--line-strong);
  background: var(--canvas);
}

.block-media img,
.block-media video {
  width: 100%;
  max-height: 440px;
  display: block;
  object-fit: contain;
  background: #111;
}

.block-media img {
  background: var(--paper);
}

.block-caption {
  width: 100%;
  height: 42px;
  padding: 0 2px;
  border: 0;
  border-bottom: 1px solid var(--line);
  outline: 0;
  background: transparent;
  color: var(--ink-soft);
  font-size: 12px;
}

.block-caption:focus {
  border-color: var(--ink);
}

.block-media-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 12px;
}

.block-media-actions button {
  min-height: 34px;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 0 10px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--paper);
  color: var(--ink-soft);
  font-size: 11px;
  cursor: pointer;
}

.block-media-actions button:hover:not(:disabled) {
  border-color: var(--ink);
  color: var(--ink);
}

.block-media-actions button:disabled {
  opacity: 0.35;
  cursor: default;
}

.media-dock {
  position: fixed;
  bottom: max(22px, calc(env(safe-area-inset-bottom) + 14px));
  z-index: 70;
}

.media-dock.is-embedded {
  position: absolute;
  right: 18px;
  bottom: -27px;
  left: auto !important;
  z-index: 20;
}

.media-dock-button {
  width: 52px;
  height: 52px;
  display: grid;
  place-items: center;
  padding: 0;
  border: 1px solid var(--ink);
  border-radius: 50%;
  background: var(--paper);
  color: var(--ink);
  box-shadow: 0 10px 30px rgba(36, 31, 22, 0.12);
  cursor: pointer;
  transition: transform var(--motion-quick), background var(--motion-quick), color var(--motion-quick);
}

.media-dock-button:hover:not(:disabled),
.media-dock-button[aria-expanded='true'] {
  transform: translateY(-1px);
  background: var(--ink);
  color: var(--paper);
}

.media-menu {
  width: 248px;
  max-height: min(520px, 68vh);
  position: absolute;
  right: 0;
  bottom: 64px;
  overflow-x: hidden;
  overflow-y: auto;
  border: 1px solid var(--line-strong);
  background: var(--paper);
  box-shadow: 0 16px 44px rgba(36, 31, 22, 0.12);
}

.media-menu > small {
  display: block;
  padding: 13px 15px 10px;
  border-bottom: 1px solid var(--line);
  color: var(--muted);
  font-size: 10px;
  line-height: 1.45;
}

.media-menu label,
.category-option {
  min-height: 48px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 15px;
  border-bottom: 1px solid var(--line);
  color: var(--ink-soft);
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.category-option {
  width: 100%;
  border: 0;
  border-bottom: 1px solid var(--line);
  background: var(--paper);
  text-align: left;
}

.category-option small {
  margin-left: auto;
  color: var(--muted);
  font-size: 9px;
  font-weight: 700;
}

.media-menu > :last-child {
  border-bottom: 0;
}

.media-menu label:hover,
.category-option:hover:not(:disabled) {
  background: var(--canvas);
  color: var(--ink);
}

.category-option:disabled {
  opacity: 0.38;
  cursor: default;
}

.media-menu input {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
}

.dock-fade-enter-active,
.dock-fade-leave-active,
.menu-fade-enter-active,
.menu-fade-leave-active {
  transition: opacity var(--motion-quick), transform var(--motion-quick);
}

.dock-fade-enter-from,
.dock-fade-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

.menu-fade-enter-from,
.menu-fade-leave-to {
  opacity: 0;
  transform: translateY(6px);
}

@media (max-width: 560px) {
  .block-field {
    padding: 20px 18px 16px;
  }

  .block-media {
    margin-right: -4px;
    margin-left: -4px;
    padding: 10px;
  }

  .period-grid {
    grid-template-columns: 1fr;
  }

  .block-text.is-free-body {
    min-height: 230px;
  }

  .media-dock-button {
    width: 50px;
    height: 50px;
  }
}
</style>
