<script setup lang="ts">
import L, { type Map as LeafletMap, type Marker } from 'leaflet'
import 'leaflet.markercluster'
import 'leaflet.markercluster/dist/MarkerCluster.css'
import type { ExhibitionPost } from '~/types/post'

const config = useRuntimeConfig()
const markerUrl = `${config.app.baseURL}mmarrk.svg`

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
let markerClusterGroup: L.MarkerClusterGroup | null = null

function renderMarkers() {
  const activeMap = map
  const activeClusterGroup = markerClusterGroup
  if (!activeMap || !activeClusterGroup) return
  activeClusterGroup.clearLayers()
  markers = []

  props.posts.forEach((post, index) => {
    const selected = post.id === props.selectedId
    const icon = L.divIcon({
      className: 'pov-marker-shell',
      html: `<span class="pov-marker ${selected ? 'is-selected' : ''}" style="--tilt:${index % 2 === 0 ? '-1.5deg' : '1.5deg'}"><span class="pov-marker-card">${escapeHtml(post.title)}</span><img class="pov-marker-symbol" src="${markerUrl}" alt=""></span>`,
      iconSize: [150, 112],
      iconAnchor: [75, 108],
    })
    const marker = L.marker([post.latitude, post.longitude], { icon })
      .on('click', () => emit('select', post))
    markers.push(marker)
  })

  activeClusterGroup.addLayers(markers)
}

function fitPosts() {
  if (!map || props.posts.length === 0) return
  if (props.posts.length === 1) {
    map.setView([props.posts[0].latitude, props.posts[0].longitude], 13)
    return
  }
  const bounds = L.latLngBounds(props.posts.map(post => [post.latitude, post.longitude]))
  map.fitBounds(bounds, { padding: [80, 70], maxZoom: 12 })
}

function escapeHtml(value: string) {
  return value.replace(/[&<>'"]/g, character => ({
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
  markerClusterGroup = L.markerClusterGroup({
    // 넓게 볼 때는 가까운 전시를 한 숫자로 정리하고, 확대할수록
    // 클러스터 반경을 줄여 작은 묶음과 개별 핀이 차례로 드러나게 한다.
    maxClusterRadius: zoom => Math.max(18, 84 - Math.max(0, zoom - 9) * 12),
    disableClusteringAtZoom: 17,
    animate: true,
    animateAddingMarkers: true,
    showCoverageOnHover: false,
    spiderfyOnMaxZoom: true,
    spiderfyDistanceMultiplier: 2.4,
    zoomToBoundsOnClick: true,
    removeOutsideVisibleBounds: true,
    chunkedLoading: true,
    iconCreateFunction: cluster => L.divIcon({
      className: 'pov-cluster-shell',
      html: `<span class="pov-cluster-count" aria-label="겹친 전시 ${cluster.getChildCount()}개">${cluster.getChildCount()}</span>`,
      iconSize: [48, 48],
      iconAnchor: [24, 24],
    }),
  }).addTo(map)
  map.on('moveend', () => {
    if (!map) return
    const bounds = map.getBounds()
    emit('boundsChanged', [bounds.getWest(), bounds.getSouth(), bounds.getEast(), bounds.getNorth()].join(','))
  })
  renderMarkers()
  fitPosts()
}

watch(mapElement, initializeMap, { flush: 'post' })
onMounted(() => nextTick(initializeMap))
watch(() => props.posts, () => {
  renderMarkers()
  fitPosts()
}, { deep: true })
watch(() => props.selectedId, renderMarkers)

onBeforeUnmount(() => {
  map?.remove()
  map = null
  markerClusterGroup = null
})
</script>

<template>
  <div ref="mapElement" class="pov-map" aria-label="공연·전시 지도" />
</template>
