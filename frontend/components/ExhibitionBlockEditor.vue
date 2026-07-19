<script setup lang="ts">
import { ChevronDown, ChevronUp, LoaderCircle, Plus, Star, Trash2 } from '@lucide/vue'
import { exhibitionLabels, parseExhibitionContent, parseExhibitionFields } from '~/utils/exhibition'

interface TextBlock {
  id: string
  type: 'text'
  text: string
}

interface ImageBlock {
  id: string
  type: 'image'
  url: string
  source: string
  alt: string
  file?: File
  localURL?: boolean
}

type EditorBlock = TextBlock | ImageBlock

interface EditorField {
  label: string
  blocks: EditorBlock[]
}

interface PendingEditorImage {
  id: string
  file: File
}

interface EditorCoverImage {
  id: string
  url: string
  file?: File
}

const props = defineProps<{
  modelValue: string
  uploadImage?: (file: File) => Promise<{ url: string }>
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'pending-images': [images: PendingEditorImage[]]
  'set-cover': [image: EditorCoverImage]
  'uploading': [value: boolean]
  'notice': [message: string]
}>()

let nextID = 0
const cursorByBlock = new Map<string, number>()
const fields = ref<EditorField[]>(parseDocument(props.modelValue))
const uploadingBlockID = ref('')

function newID(prefix: string) {
  nextID += 1
  return `${prefix}-${Date.now().toString(36)}-${nextID}`
}

function parseDocument(body: string): EditorField[] {
  const values = new Map(parseExhibitionFields(body).map(field => [field.label, field.value]))
  return exhibitionLabels.map((label, fieldIndex) => {
    const value = values.get(label) || ''
    const blocks: EditorBlock[] = parseExhibitionContent(value).map((segment, segmentIndex) => {
      if (segment.type === 'image') {
        return {
          id: `field-${fieldIndex}-image-${segmentIndex}`,
          type: 'image',
          url: segment.url,
          source: segment.url,
          alt: segment.alt,
        }
      }
      return {
        id: `field-${fieldIndex}-text-${segmentIndex}`,
        type: 'text',
        text: segment.value,
      }
    })
    if (!blocks.length || blocks[blocks.length - 1]?.type === 'image') {
      blocks.push({ id: `field-${fieldIndex}-text-end`, type: 'text', text: '' })
    }
    return { label, blocks }
  })
}

function serializeDocument() {
  return `${fields.value.map((field) => {
    const value = field.blocks
      .map((block) => block.type === 'text' ? block.text.trim() : `![${sanitizeAlt(block.alt)}](${block.source})`)
      .filter(Boolean)
      .join('\n\n')
    return `${field.label}:\n${value}`
  }).join('\n\n')}\n`
}

function sanitizeAlt(value: string) {
  return value.replace(/[\[\]\r\n]/g, ' ').trim() || '전시 본문 이미지'
}

function syncDocument() {
  emit('update:modelValue', serializeDocument())
  emitPendingImages()
}

function emitPendingImages() {
  const images = fields.value.flatMap(field => field.blocks
    .filter((block): block is ImageBlock => block.type === 'image' && Boolean(block.file))
    .map(block => ({ id: block.id, file: block.file! })))
  emit('pending-images', images)
}

function rememberCursor(block: TextBlock, event: Event) {
  cursorByBlock.set(block.id, (event.target as HTMLTextAreaElement).selectionStart)
}

function imageCount() {
  return fields.value.reduce((count, field) => count + field.blocks.filter(block => block.type === 'image').length, 0)
}

async function insertImage(fieldIndex: number, blockIndex: number, event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!['image/jpeg', 'image/png', 'image/webp', 'image/gif'].includes(file.type)) {
    emit('notice', 'JPG, PNG, WebP 또는 GIF 이미지만 넣을 수 있습니다.')
    return
  }
  if (file.size > 8 * 1024 * 1024) {
    emit('notice', '본문 이미지는 장당 8MB 이하로 선택해 주세요.')
    return
  }
  if (imageCount() >= 6) {
    emit('notice', '본문 이미지는 게시글당 최대 6장까지 넣을 수 있습니다.')
    return
  }

  const field = fields.value[fieldIndex]
  const textBlock = field?.blocks[blockIndex]
  if (!field || !textBlock || textBlock.type !== 'text') return

  uploadingBlockID.value = textBlock.id
  emit('uploading', true)
  try {
    const cursor = Math.min(cursorByBlock.get(textBlock.id) ?? textBlock.text.length, textBlock.text.length)
    const imageID = newID('inline')
    let imageBlock: ImageBlock
    if (props.uploadImage) {
      const uploaded = await props.uploadImage(file)
      imageBlock = {
        id: imageID,
        type: 'image',
        url: uploaded.url,
        source: uploaded.url,
        alt: '',
      }
    } else {
      const localURL = URL.createObjectURL(file)
      imageBlock = {
        id: imageID,
        type: 'image',
        url: localURL,
        source: `pov-inline://${imageID}`,
        alt: '',
        file,
        localURL: true,
      }
    }

    const before: TextBlock = { ...textBlock, text: textBlock.text.slice(0, cursor) }
    const after: TextBlock = { id: newID('text'), type: 'text', text: textBlock.text.slice(cursor) }
    field.blocks.splice(blockIndex, 1, before, imageBlock, after)
    syncDocument()
    emit('notice', '선택한 글 위치에 이미지를 넣었습니다.')
  } catch {
    emit('notice', '본문 이미지를 올리지 못했습니다. 잠시 후 다시 시도해 주세요.')
  } finally {
    uploadingBlockID.value = ''
    emit('uploading', false)
  }
}

function moveImage(fieldIndex: number, blockIndex: number, direction: -1 | 1) {
  const blocks = fields.value[fieldIndex]?.blocks
  const target = blockIndex + direction
  if (!blocks || target < 0 || target >= blocks.length) return
  const [block] = blocks.splice(blockIndex, 1)
  if (!block) return
  blocks.splice(target, 0, block)
  syncDocument()
}

function removeImage(fieldIndex: number, blockIndex: number) {
  const blocks = fields.value[fieldIndex]?.blocks
  const block = blocks?.[blockIndex]
  if (!blocks || !block || block.type !== 'image') return
  if (block.localURL) URL.revokeObjectURL(block.url)
  blocks.splice(blockIndex, 1)
  if (!blocks.some(item => item.type === 'text')) {
    blocks.push({ id: newID('text'), type: 'text', text: '' })
  }
  syncDocument()
  emit('notice', '본문 이미지를 삭제했습니다.')
}

function useAsCover(block: ImageBlock) {
  emit('set-cover', { id: block.id, url: block.url, file: block.file })
  emit('notice', '이 이미지를 대표 사진으로 지정했습니다.')
}

function releaseLocalImages() {
  for (const field of fields.value) {
    for (const block of field.blocks) {
      if (block.type === 'image' && block.localURL) URL.revokeObjectURL(block.url)
    }
  }
}

watch(() => props.modelValue, (value) => {
  if (value === serializeDocument()) return
  releaseLocalImages()
  fields.value = parseDocument(value)
  cursorByBlock.clear()
  emitPendingImages()
})

onBeforeUnmount(releaseLocalImages)
</script>

<template>
  <div class="block-editor">
    <section v-for="(field, fieldIndex) in fields" :key="field.label" class="block-field">
      <label class="block-field-label">{{ field.label }}</label>

      <template v-for="(block, blockIndex) in field.blocks" :key="block.id">
        <div v-if="block.type === 'text'" class="block-text-wrap">
          <textarea
            v-model="block.text"
            class="block-text"
            :aria-label="`${field.label} 내용`"
            rows="2"
            spellcheck="true"
            @input="syncDocument"
            @click="rememberCursor(block, $event)"
            @keyup="rememberCursor(block, $event)"
            @select="rememberCursor(block, $event)"
          />
          <div class="block-insert-line">
            <span />
            <label class="block-add-image" :aria-label="`${field.label} 현재 위치에 이미지 넣기`">
              <LoaderCircle v-if="uploadingBlockID === block.id" :size="16" class="spin" />
              <Plus v-else :size="16" />
              <span>이미지</span>
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp,image/gif"
                :disabled="Boolean(uploadingBlockID)"
                @change="insertImage(fieldIndex, blockIndex, $event)"
              >
            </label>
            <span />
          </div>
        </div>

        <figure v-else class="block-image">
          <img :src="block.url" :alt="block.alt">
          <input
            v-model="block.alt"
            class="block-caption"
            aria-label="이미지 설명"
            placeholder="이미지 설명"
            @input="syncDocument"
          >
          <div class="block-image-actions">
            <button type="button" :disabled="blockIndex === 0" aria-label="이미지 위로 이동" @click="moveImage(fieldIndex, blockIndex, -1)">
              <ChevronUp :size="15" /> 위
            </button>
            <button type="button" :disabled="blockIndex === field.blocks.length - 1" aria-label="이미지 아래로 이동" @click="moveImage(fieldIndex, blockIndex, 1)">
              <ChevronDown :size="15" /> 아래
            </button>
            <button type="button" aria-label="대표 이미지로 지정" @click="useAsCover(block)">
              <Star :size="15" /> 대표
            </button>
            <button type="button" aria-label="본문 이미지 삭제" @click="removeImage(fieldIndex, blockIndex)">
              <Trash2 :size="15" /> 삭제
            </button>
          </div>
        </figure>
      </template>
    </section>
  </div>
</template>

<style scoped>
.block-editor {
  border-top: 1px solid var(--line-strong);
  background: var(--paper);
}

.block-field {
  padding: 22px clamp(20px, 4vw, 42px) 15px;
  border-right: 1px solid var(--line-strong);
  border-bottom: 1px solid var(--line);
  border-left: 1px solid var(--line-strong);
}

.block-field-label {
  display: block;
  margin-bottom: 12px;
  color: var(--muted);
  font-size: 12px;
  font-weight: 800;
}

.block-text {
  width: 100%;
  min-height: 54px;
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

.block-insert-line {
  min-height: 34px;
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 10px;
}

.block-insert-line > span {
  height: 1px;
  background: var(--line);
}

.block-add-image {
  min-height: 32px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 11px;
  border: 1px solid var(--line);
  border-radius: 999px;
  color: var(--muted);
  font-size: 11px;
  font-weight: 750;
  cursor: pointer;
  transition: border-color var(--motion-quick), color var(--motion-quick);
}

.block-add-image:hover {
  border-color: var(--ink);
  color: var(--ink);
}

.block-add-image input {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
}

.block-image {
  margin: 12px 0 20px;
  padding: 14px;
  border: 1px solid var(--line-strong);
  background: var(--canvas);
}

.block-image img {
  width: 100%;
  max-height: 440px;
  display: block;
  object-fit: contain;
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

.block-image-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 12px;
}

.block-image-actions button {
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

.block-image-actions button:hover:not(:disabled) {
  border-color: var(--ink);
  color: var(--ink);
}

.block-image-actions button:disabled {
  opacity: 0.35;
  cursor: default;
}

@media (max-width: 560px) {
  .block-field {
    padding: 20px 18px 13px;
  }

  .block-image {
    margin-right: -4px;
    margin-left: -4px;
    padding: 10px;
  }
}
</style>
