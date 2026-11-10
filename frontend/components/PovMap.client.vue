<script setup lang="ts">
import L, { type Map as LeafletMap, type Marker } from 'leaflet'
import 'leaflet.markercluster'
import 'leaflet.markercluster/dist/MarkerCluster.css'
import type { ExhibitionPost } from '~/types/post'
import { isExhibitionExpired } from '~/utils/exhibition'
import type { MapPoint } from '~/utils/exhibitionMap'

const config = useRuntimeConfig()
const markerUrl = `${config.app.baseURL}mmarrk.svg`

const props = defineProps<{
  posts: ExhibitionPost[]
  extentPosts: ExhibitionPost[]
  radius: { km: number; center: MapPoint } | null
  userLocation?: MapPoint
  selectedId?: string
}>()

const emit = defineEmits<{
  resultsSelected: [ids: string[] | null]
  boundsChanged: [bbox: string]
  centerChanged: [center: MapPoint]
}>()

const mapElement = ref<HTMLElement | null>(null)
let map: LeafletMap | null = null
let markers: Marker[] = []
let markerClusterGroup: L.MarkerClusterGroup | null = null
let radiusCircle: L.Circle | undefined
let locationDot: L.CircleMarker | undefined
let resizeObserver: ResizeObserver | undefined
const markerPostIds = new WeakMap<Marker, string>()

function renderMarkers() {
  const activeMap = map
  const activeClusterGroup = markerClusterGroup
  if (!activeMap || !activeClusterGroup) return
  activeClusterGroup.clearLayers()
  markers = []

  props.posts.forEach((post, index) => {
    const selected = post.id === props.selectedId
    const expired = isExhibitionExpired(post)
    const icon = L.divIcon({
      className: 'pov-marker-shell',
      html: `<span class="pov-marker ${selected ? 'is-selected' : ''} ${expired ? 'is-expired' : ''}" style="--tilt:${index % 2 === 0 ? '-1.5deg' : '1.5deg'}"><span class="pov-marker-card">${escapeHtml(post.title)}</span><img class="pov-marker-symbol" src="${markerUrl}" alt=""></span>`,
      iconSize: [150, 112],
      iconAnchor: [75, 108],
    })
    const marker = L.marker([post.latitude, post.longitude], { icon })
      .on('click', () => emit('resultsSelected', [post.id]))
    markerPostIds.set(marker, post.id)
    markers.push(marker)
  })

  activeClusterGroup.addLayers(markers)
}

function fitPosts() {
  const posts = props.extentPosts
  if (!map || props.radius || props.userLocation || posts.length === 0) return
  if (posts.length === 1) {
    map.setView([posts[0].latitude, posts[0].longitude], 13, { animate: false })
    return
  }
  const domesticPosts = posts.filter(post => post.latitude >= 33 && post.latitude <= 39 && post.longitude >= 124 && post.longitude <= 132)
  const initialPosts = domesticPosts.length > 0 ? domesticPosts : posts
  const bounds = L.latLngBounds(initialPosts.map(post => [post.latitude, post.longitude]))
  map.fitBounds(bounds, { padding: [80, 70], maxZoom: 12, animate: false })
}

function updateRadius(fit = false) {
  if (!map) return
  const radius = props.radius
  if (!radius) {
    radiusCircle?.remove()
    radiusCircle = undefined
    return
  }
  const center = L.latLng(radius.center.latitude, radius.center.longitude)
  const meters = radius.km * 1000
  if (radiusCircle) radiusCircle.setLatLng(center).setRadius(meters)
  else radiusCircle = L.circle(center, {
    radius: meters, color: '#237b43', weight: 2, fillOpacity: 0.1, interactive: false,
    className: 'pov-radius-circle',
  }).addTo(map)
  if (fit) map.fitBounds(radiusCircle.getBounds(), { padding: [24, 24], maxZoom: 15, animate: false })
}

function showUserLocation() {
  const position = props.userLocation
  if (!map || !position) return
  const point = L.latLng(position.latitude, position.longitude)
  if (locationDot) locationDot.setLatLng(point)
  else locationDot = L.circleMarker(point, {
    radius: 8, color: '#fff', weight: 3, fillColor: '#2563eb', fillOpacity: 1,
    interactive: false, className: 'pov-user-location',
  }).addTo(map)
  if (props.radius) updateRadius(true)
  else map.setView(point, Math.max(map.getZoom(), 13), { animate: false })
  locationDot.bringToFront()
}

function reportViewport() {
  if (!map) return
  const center = map.getCenter()
  const bounds = map.getBounds()
  emit('centerChanged', { latitude: center.lat, longitude: center.lng })
  emit('boundsChanged', [bounds.getWest(), bounds.getSouth(), bounds.getEast(), bounds.getNorth()].join(','))
}

function selectCluster(cluster: L.MarkerCluster) {
  if (!map) return
  const ids = cluster.getAllChildMarkers()
    .map(marker => markerPostIds.get(marker))
    .filter((id): id is string => id !== undefined)
  if (ids.length === 0) return
  emit('resultsSelected', ids)

  const bounds = cluster.getBounds()
  const sameVenue = bounds.getNorthEast().distanceTo(bounds.getSouthWest()) < 30
  if (!sameVenue && (!props.radius || props.userLocation) && map.getZoom() < map.getMaxZoom()) {
    cluster.zoomToBounds({ padding: [45, 45] })
  }
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
    animate: true,
    animateAddingMarkers: true,
    showCoverageOnHover: false,
    spiderfyOnMaxZoom: false,
    spiderfyDistanceMultiplier: 2.4,
    zoomToBoundsOnClick: false,
    removeOutsideVisibleBounds: true,
    iconCreateFunction: cluster => L.divIcon({
      className: 'pov-cluster-shell',
      html: `<span class="pov-cluster-count" aria-label="겹친 전시 ${cluster.getChildCount()}개">${cluster.getChildCount()}</span>`,
      iconSize: [48, 48],
      iconAnchor: [24, 24],
    }),
  }).addTo(map)
  markerClusterGroup.on('clusterclick', (event: L.LeafletEvent) => {
    selectCluster((event as L.LeafletEvent & { layer: L.MarkerCluster }).layer)
  })
  map.on('click', () => emit('resultsSelected', null))
  map.on('moveend', reportViewport)
  resizeObserver = new ResizeObserver(() => map?.invalidateSize({ debounceMoveend: true }))
  resizeObserver.observe(mapElement.value)
  fitPosts()
  updateRadius()
  showUserLocation()
  renderMarkers()
  reportViewport()
}

watch(mapElement, initializeMap, { flush: 'post' })
onMounted(() => nextTick(initializeMap))
watch(() => props.extentPosts, fitPosts)
watch([() => props.posts, () => props.selectedId], renderMarkers)
watch(() => props.radius, (radius, previous) => updateRadius(radius?.km !== previous?.km))
watch(() => props.userLocation, showUserLocation)

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  map?.remove()
  map = null
  markerClusterGroup = null
})
</script>

<template>
  <div ref="mapElement" class="pov-map" aria-label="공연·전시 지도" />
</template>
