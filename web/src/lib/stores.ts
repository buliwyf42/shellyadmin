import { writable } from 'svelte/store'
import type { Device } from './types'

const defaultCols = {
  serial: false,
  coords: false,
  matter: false,
  config: false,
}

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
  history.pushState({}, '', path)
  currentPath.set(path)
}

window.addEventListener('popstate', () => currentPath.set(window.location.pathname))
