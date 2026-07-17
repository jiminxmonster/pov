<script setup lang="ts">
import L, { type Map as LeafletMap, type Marker } from 'leaflet'
import type { ExhibitionPost } from '~/types/post'

const props = defineProps<{
  posts: ExhibitionPost[]
  selectedId?: string
}>()

const emit = defineEmits<{
  select: [post: ExhibitionPost]
  boundsChanged: [bbox: string]
}>()

const mapElement = ref<HTMLElement | null>(null)
let map: LeafletMap | null = null
let markers: Marker[] = []

function renderMarkers() {
  const activeMap = map
  if (!activeMap) return
  markers.forEach((marker) => marker.remove())
  markers = []

  props.posts.forEach((post, index) => {
    const selected = post.id === props.selectedId
    const icon = L.divIcon({
      className: 'pov-marker-shell',
      html: `<span class="pov-marker ${selected ? 'is-selected' : ''}" style="--tilt:${index % 2 === 0 ? '-2deg' : '2deg'}">${escapeHtml(post.title)}</span>`,
      iconSize: [116, 54],
      iconAnchor: [58, 54],
    })
    const marker = L.marker([post.latitude, post.longitude], { icon })
      .addTo(activeMap)
      .on('click', () => emit('select', post))
    markers.push(marker)
  })
}

function escapeHtml(value: string) {
  return value.replace(/[&<>'"]/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;',
  })[character] || character)
}

function initializeMap() {
  if (!mapElement.value || map) return
  map = L.map(mapElement.value, {
    zoomControl: false,
    attributionControl: true,
  }).setView([37.5665, 126.978], 12)

  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19,
    attribution: '&copy; OpenStreetMap contributors',
  }).addTo(map)

  L.control.zoom({ position: 'bottomright' }).addTo(map)
  map.on('moveend', () => {
    if (!map) return
    const bounds = map.getBounds()
    emit('boundsChanged', [bounds.getWest(), bounds.getSouth(), bounds.getEast(), bounds.getNorth()].join(','))
  })
  renderMarkers()
}

watch(mapElement, initializeMap, { flush: 'post' })
onMounted(() => nextTick(initializeMap))

watch(() => [props.posts, props.selectedId], renderMarkers, { deep: true })

onBeforeUnmount(() => {
  map?.remove()
  map = null
})
</script>

<template>
  <div ref="mapElement" class="pov-map" aria-label="공연·전시 지도" />
</template>
