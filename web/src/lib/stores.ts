import { writable } from 'svelte/store'
import type { Device } from './types'

export const deviceColumns = [
  { key: 'device_num', label: '#' },
  { key: 'name', label: 'Name' },
  { key: 'ip', label: 'IP' },
  { key: 'mac', label: 'MAC' },
  { key: 'gen', label: 'Support' },
  { key: 'model', label: 'Model' },
  { key: 'fw', label: 'Firmware' },
  { key: 'online', label: 'Online' },
  { key: 'wifi_ssid', label: 'WiFi' },
  { key: 'mqtt_enabled', label: 'MQTT' },
  { key: 'mqtt_server', label: 'MQTT Server' },
  { key: 'mqtt_client_id', label: 'MQTT Client ID' },
  { key: 'mqtt_topic_prefix', label: 'MQTT Topic' },
  { key: 'cloud_connected', label: 'Cloud' },
  { key: 'ws_connected', label: 'WebSocket' },
  { key: 'tz', label: 'Timezone' },
  { key: 'time_format', label: 'Time Format' },
  { key: 'sntp_server', label: 'SNTP' },
  { key: 'serial', label: 'Serial' },
  { key: 'matter_enabled', label: 'Matter' },
  { key: 'ble_gw_enabled', label: 'BLE GW' },
  { key: 'coords', label: 'Coords' },
  { key: 'eco_mode', label: 'Eco' },
  { key: 'discoverable', label: 'Discoverable' },
  { key: 'first_seen', label: 'First Seen' },
  { key: 'last_seen', label: 'Last Seen' },
  { key: 'compliance', label: 'Compliance' },
] as const

const defaultCols: Record<string, boolean> = Object.fromEntries(deviceColumns.map((column) => [
  column.key,
  ['device_num', 'name', 'ip', 'mac', 'gen', 'model', 'fw', 'online', 'wifi_ssid', 'mqtt_enabled', 'cloud_connected', 'tz', 'compliance'].includes(column.key),
]))

function persisted<T>(key: string, fallback: T) {
  const initial = typeof localStorage === 'undefined'
    ? fallback
    : JSON.parse(localStorage.getItem(key) ?? JSON.stringify(fallback))
  const store = writable<T>(initial)
  store.subscribe((value) => {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(key, JSON.stringify(value))
    }
  })
  return store
}

export const devices = writable<Device[]>([])
export const colVis = persisted<Record<string, boolean>>('colVis', defaultCols)
export const refreshInterval = persisted<number>('refreshInterval', 0)
export const currentPath = writable<string>(window.location.pathname)

export function navigate(path: string): void {
  if (window.location.pathname !== path) {
    history.pushState({}, '', path)
  }
  currentPath.set(path)
}

window.addEventListener('popstate', () => currentPath.set(window.location.pathname))
