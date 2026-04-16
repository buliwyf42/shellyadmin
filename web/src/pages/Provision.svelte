<script lang="ts">
  import { onMount } from 'svelte'
  import { APIError, api } from '../lib/api'
  import type { Credential, CredentialGroup, Device } from '../lib/types'
  import ErrorNotice from '../components/ErrorNotice.svelte'

  type ProvisionResult = { info: unknown; results: unknown[] }
  type TemplateGenPolicy = { hasGen1Only: boolean; hasGen2Only: boolean; hasDual: boolean }
  type PrecheckIssue = { ip: string; label: string; reason: string; category: 'auth' | 'generation' | 'other' }

  let devices: Device[] = []
  let selected = new Set<string>()
  let loading = false
  let running = false
  let error = ''
  let errorDetails = ''
  let results: ProvisionResult[] = []
  let templateNames: string[] = []
  let credentials: Credential[] = []
  let credentialGroups: CredentialGroup[] = []
  let deviceGroupAssignments: Record<string, string> = {}
  let selectedTemplate = ''
  let selectedTemplateCredentialRef = ''
  let autoSelectedCredentialRef = ''
  let templateName = ''
  let viewMode: 'form' | 'json' = 'form'
  let jsonText = '{}'
  let templateLoadNotice = ''

  let sysEnabled = false
  let sysNameEnabled = false
  let sysName = '{device_name}'
  let sysTZEnabled = false
  let sysTZ = 'Europe/Berlin'
  let sysLatEnabled = false
  let sysLat = ''
  let sysLonEnabled = false
  let sysLon = ''
  let sysSNTPEnabled = false
  let sysSNTP = 'time.cloudflare.com'
  let sysTimeFormatEnabled = false
  let sysTimeFormat: '24h' | '12h' = '24h'
  let sysDebugWSEnabled = false
  let sysDebugWS = false
  let sysDebugUDPHostEnabled = false
  let sysDebugUDPHost = ''
  let sysRPCUDPPortEnabled = false
  let sysRPCUDPPort = '0'
  let sysEcoEnabled = false
  let sysEco = false
  let sysDiscoverableEnabled = false
  let sysDiscoverable = true

  let mqttEnabled = false
  let mqttEnableField = false
  let mqttEnable = true
  let mqttServerEnabled = false
  let mqttServer = 'mqtt.home:1883'
  let mqttClientIDEnabled = false
  let mqttClientID = '{device_name}'
  let mqttTopicPrefixEnabled = false
  let mqttTopicPrefix = 'shelly/{device_name}'
  let mqttUserEnabled = false
  let mqttUser = ''
  let mqttPassEnabled = false
  let mqttPass = ''
  let mqttSSLCAEnabled = false
  let mqttSSLCA = ''
  let mqttRPCNtfEnabled = false
  let mqttRPCNtf = true
  let mqttStatusNtfEnabled = false
  let mqttStatusNtf = true
  let mqttEnableRPCEnabled = false
  let mqttEnableRPC = true
  let mqttEnableControlEnabled = false
  let mqttEnableControl = true
  let mqttUseClientCertEnabled = false
  let mqttUseClientCert = false

  let wsEnabled = false
  let wsEnableField = false
  let wsEnable = true
  let wsServerEnabled = false
  let wsServer = 'ws://ha.home:8123/api/shelly/ws'
  let wsTLSModeEnabled = false
  let wsTLSMode: 'no_validation' | 'default' | 'user' = 'default'
  let wsSSLCAEnabled = false
  let wsSSLCA = ''

  let bleEnabled = false
  let bleEnableField = false
  let bleEnable = true
  let bleRPCEnabledField = false
  let bleRPCEnabled = false
  let bleObserverEnabledField = false
  let bleObserverEnabled = false

  let matterEnabled = false
  let matterEnableField = false
  let matterEnable = true

  let cloudEnabled = false
  let cloudEnableField = false
  let cloudEnable = true

  let otaEnabled = false
  let otaStageEnabled = false
  let otaStage: 'stable' | 'beta' = 'stable'
  let otaAutoUpdateEnabled = false
  let otaAutoUpdate: 'off' | 'stable' | 'beta' = 'off'

  let authEnabled = false
  let authPassEnabled = false
  let authPass = ''

  let wifiEnabled = false
  let wifiSTAEnabled = false
  let wifiSSIDEnabled = false
  let wifiSSID = ''
  let wifiPassEnabled = false
  let wifiPass = ''

  let sysOpen = false
  let mqttOpen = false
  let wsOpen = false
  let bleOpen = false
  let matterOpen = false
  let cloudOpen = false
  let otaOpen = false
  let authOpen = false
  let wifiOpen = false
  let copiedSkipped = false

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`
      return
    }
    error = (err as Error).message
    errorDetails = String(err)
  }

  function clearTemplateLoadNotice() {
    templateLoadNotice = ''
  }

  $: sysExpanded = sysEnabled || sysNameEnabled || sysTZEnabled || sysLatEnabled || sysLonEnabled || sysSNTPEnabled || sysTimeFormatEnabled || sysDebugWSEnabled || sysDebugUDPHostEnabled || sysRPCUDPPortEnabled || sysEcoEnabled || sysDiscoverableEnabled
  $: mqttExpanded = mqttEnabled || mqttEnableField || mqttServerEnabled || mqttClientIDEnabled || mqttTopicPrefixEnabled || mqttUserEnabled || mqttPassEnabled || mqttSSLCAEnabled || mqttRPCNtfEnabled || mqttStatusNtfEnabled || mqttEnableRPCEnabled || mqttEnableControlEnabled || mqttUseClientCertEnabled
  $: wsExpanded = wsEnabled || wsEnableField || wsServerEnabled || wsTLSModeEnabled || wsSSLCAEnabled
  $: bleExpanded = bleEnabled || bleEnableField || bleRPCEnabledField || bleObserverEnabledField
  $: matterExpanded = matterEnabled || matterEnableField
  $: cloudExpanded = cloudEnabled || cloudEnableField
  $: otaExpanded = otaEnabled || otaStageEnabled || otaAutoUpdateEnabled
  $: authExpanded = authEnabled || authPassEnabled
  $: wifiExpanded = wifiEnabled || wifiSTAEnabled || wifiSSIDEnabled || wifiPassEnabled
  $: sysVisible = sysExpanded || sysOpen
  $: mqttVisible = mqttExpanded || mqttOpen
  $: wsVisible = wsExpanded || wsOpen
  $: bleVisible = bleExpanded || bleOpen
  $: matterVisible = matterExpanded || matterOpen
  $: cloudVisible = cloudExpanded || cloudOpen
  $: otaVisible = otaExpanded || otaOpen
  $: authVisible = authExpanded || authOpen
  $: wifiVisible = wifiExpanded || wifiOpen

  onMount(async () => {
    loading = true
    error = ''
    try {
      const [loadedDevices, loadedTemplates, loadedCredentialGroups, loadedGroupAssignments] = await Promise.all([
        api.getDevices(),
        api.listTemplates(),
        api.listCredentialGroups(),
        api.getCredentialGroupAssignments(),
      ])
      devices = loadedDevices
      templateNames = loadedTemplates
      credentials = await api.listCredentials()
      credentialGroups = loadedCredentialGroups
      deviceGroupAssignments = loadedGroupAssignments.assignments
    } catch (err) {
      captureError(err)
    } finally {
      loading = false
    }
    jsonText = JSON.stringify(buildTemplate(), null, 2)
  })

  function toggle(mac: string, checked: boolean) {
    if (checked) selected.add(mac)
    else selected.delete(mac)
    selected = new Set(selected)
  }

  function selectAll() {
    selected = new Set(devices.map((d) => d.mac))
  }

  function selectNone() {
    selected = new Set()
  }

  function selectedDevices() {
    return devices.filter((d) => selected.has(d.mac))
  }

  function templateForPrecheck(): Record<string, unknown> | null {
    if (viewMode !== 'json') return buildTemplate()
    try {
      return JSON.parse(jsonText) as Record<string, unknown>
    } catch {
      return null
    }
  }

  function deriveTemplateGenPolicy(template: Record<string, unknown>): TemplateGenPolicy {
    const policy: TemplateGenPolicy = { hasGen1Only: false, hasGen2Only: false, hasDual: false }
    for (const rawSection of Object.keys(template)) {
      const section = rawSection.trim().toLowerCase()
      switch (section) {
        case 'gen1_http':
          policy.hasGen1Only = true
          break
        case 'mqtt':
        case 'sys':
          policy.hasDual = true
          break
        case 'gen2_rpc':
        case 'ws':
        case 'ble':
        case 'matter':
        case 'cloud':
        case 'wifi':
        case 'kvs':
        case 'ota':
        case 'auth':
          policy.hasGen2Only = true
          break
        default:
          policy.hasGen2Only = true
      }
    }
    return policy
  }

  function genIncompatibleReason(device: Device, policy: TemplateGenPolicy): string {
    if (device.gen <= 1) {
      if (policy.hasGen2Only && !policy.hasGen1Only && !policy.hasDual) {
        return 'template targets gen2+ sections while device is gen1'
      }
      return ''
    }
    if (policy.hasGen1Only && !policy.hasGen2Only && !policy.hasDual) {
      return 'template targets gen1-only sections while device is gen2+'
    }
    return ''
  }

  function selectedGenLabel() {
    const picked = selectedDevices()
    if (picked.length === 0) return 'none'
    const hasGen1 = picked.some((d) => d.gen <= 1)
    const hasGen2 = picked.some((d) => d.gen >= 2)
    if (hasGen1 && hasGen2) return 'mixed'
    return hasGen1 ? 'gen1' : 'gen2+'
  }

  function reasonBadgeClass(category: PrecheckIssue['category']): string {
    switch (category) {
      case 'auth':
        return 'bg-warning text-dark'
      case 'generation':
        return 'bg-info text-dark'
      default:
        return 'bg-secondary'
    }
  }

  function reasonBadgeText(category: PrecheckIssue['category']): string {
    switch (category) {
      case 'auth':
        return 'auth'
      case 'generation':
        return 'generation'
      default:
        return 'other'
    }
  }

  function selectOnlyEligible() {
    if (!precheckTemplate || precheckTemplateError) return
    const skippedIPs = new Set(precheckIssues.map((issue) => issue.ip))
    selected = new Set(selectedDevices().filter((device) => !skippedIPs.has(device.ip)).map((device) => device.mac))
  }

  async function copySkippedIPs() {
    const ips = [...new Set(precheckIssues.map((issue) => issue.ip))]
    if (ips.length === 0) return
    try {
      await navigator.clipboard.writeText(ips.join('\n'))
      copiedSkipped = true
      setTimeout(() => {
        copiedSkipped = false
      }, 1500)
    } catch {
      copiedSkipped = false
    }
  }

  $: precheckTemplate = templateForPrecheck()
  $: precheckTemplateError = viewMode === 'json' && precheckTemplate === null ? 'JSON is invalid; precheck is disabled until JSON is valid.' : ''
  $: precheckPolicy = precheckTemplate ? deriveTemplateGenPolicy(precheckTemplate) : { hasGen1Only: false, hasGen2Only: false, hasDual: false }
  $: groupCredentialByName = Object.fromEntries(credentialGroups.map((group) => [group.name, group.name]))
  $: groupResolution = (() => {
    const chosenDevices = selectedDevices()
    let unresolved = 0
    const credentials = new Set<string>()
    for (const device of chosenDevices) {
      const groupName = deviceGroupAssignments[device.mac]
      if (!groupName) {
        unresolved++
        continue
      }
      const credentialRef = groupCredentialByName[groupName]
      if (!credentialRef) {
        unresolved++
        continue
      }
      credentials.add(credentialRef)
    }
    return {
      total: chosenDevices.length,
      unresolved,
      credentialRefs: [...credentials],
    }
  })()
  $: groupCredentialHint = (() => {
    if (groupResolution.total === 0) return ''
    if (groupResolution.credentialRefs.length === 1 && groupResolution.unresolved === 0) {
      return `Credential defaulted from device groups: ${groupResolution.credentialRefs[0]}`
    }
    if (groupResolution.credentialRefs.length > 1) {
      return 'Selected devices resolve to multiple group credentials. Choose a credential manually.'
    }
    if (groupResolution.unresolved > 0) {
      return `${groupResolution.unresolved} selected device(s) have no resolvable credential group.`
    }
    return ''
  })()
  $: resolvedGroupCredentialRef = groupResolution.credentialRefs.length === 1 && groupResolution.unresolved === 0
    ? groupResolution.credentialRefs[0]
    : ''
  $: if (resolvedGroupCredentialRef) {
    if (!selectedTemplateCredentialRef || selectedTemplateCredentialRef === autoSelectedCredentialRef) {
      selectedTemplateCredentialRef = resolvedGroupCredentialRef
      autoSelectedCredentialRef = resolvedGroupCredentialRef
    }
  } else if (autoSelectedCredentialRef && selectedTemplateCredentialRef === autoSelectedCredentialRef) {
    selectedTemplateCredentialRef = ''
    autoSelectedCredentialRef = ''
  }
  $: precheckIssues = selectedDevices().flatMap((device): PrecheckIssue[] => {
    if (!precheckTemplate) return []
    if (device.auth_required && !selectedTemplateCredentialRef.trim()) {
      return [{
        ip: device.ip,
        label: device.name || device.serial || device.mac,
        reason: 'auth required but no credential ref selected',
        category: 'auth',
      }]
    }
    const genReason = genIncompatibleReason(device, precheckPolicy)
    if (genReason) {
      return [{
        ip: device.ip,
        label: device.name || device.serial || device.mac,
        reason: genReason,
        category: 'generation',
      }]
    }
    return []
  })
  $: precheckEligibleCount = Math.max(0, selectedDevices().length - precheckIssues.length)
  $: precheckReasonCounts = precheckIssues.reduce((acc, issue) => {
    acc[issue.category] = (acc[issue.category] || 0) + 1
    return acc
  }, {} as Record<string, number>)

  function maybeNum(raw: string | number): number | undefined {
    if (typeof raw === 'number') return Number.isFinite(raw) ? raw : undefined
    if (raw.trim() === '') return undefined
    const n = Number(raw)
    return Number.isFinite(n) ? n : undefined
  }

  function isTLSServerURL(raw: string): boolean {
    return raw.trim().toLowerCase().startsWith('wss://')
  }

  function inferWSTLSMode(server: string | undefined, sslCA: string | undefined, explicitMode: string | undefined): 'no_validation' | 'default' | 'user' | undefined {
    if (explicitMode === 'no_validation' || explicitMode === 'default' || explicitMode === 'user') return explicitMode
    if (!server || !isTLSServerURL(server)) return undefined
    if (sslCA === '*') return 'no_validation'
    if (sslCA && sslCA.trim() !== '') return 'user'
    return 'default'
  }

  function resetFormState() {
    sysEnabled = false
    sysNameEnabled = false
    sysName = '{device_name}'
    sysTZEnabled = false
    sysTZ = 'Europe/Berlin'
    sysLatEnabled = false
    sysLat = ''
    sysLonEnabled = false
    sysLon = ''
    sysSNTPEnabled = false
    sysSNTP = 'time.cloudflare.com'
    sysTimeFormatEnabled = false
    sysTimeFormat = '24h'
    sysDebugWSEnabled = false
    sysDebugWS = false
    sysDebugUDPHostEnabled = false
    sysDebugUDPHost = ''
    sysRPCUDPPortEnabled = false
    sysRPCUDPPort = '0'
    sysEcoEnabled = false
    sysEco = false
    sysDiscoverableEnabled = false
    sysDiscoverable = true

    mqttEnabled = false
    mqttEnableField = false
    mqttEnable = true
    mqttServerEnabled = false
    mqttServer = 'mqtt.home:1883'
    mqttClientIDEnabled = false
    mqttClientID = '{device_name}'
    mqttTopicPrefixEnabled = false
    mqttTopicPrefix = 'shelly/{device_name}'
    mqttUserEnabled = false
    mqttUser = ''
    mqttPassEnabled = false
    mqttPass = ''
    mqttSSLCAEnabled = false
    mqttSSLCA = ''
    mqttRPCNtfEnabled = false
    mqttRPCNtf = true
    mqttStatusNtfEnabled = false
    mqttStatusNtf = true
    mqttEnableRPCEnabled = false
    mqttEnableRPC = true
    mqttEnableControlEnabled = false
    mqttEnableControl = true
    mqttUseClientCertEnabled = false
    mqttUseClientCert = false

    wsEnabled = false
    wsEnableField = false
    wsEnable = true
    wsServerEnabled = false
    wsServer = 'ws://ha.home:8123/api/shelly/ws'
    wsTLSModeEnabled = false
    wsTLSMode = 'default'
    wsSSLCAEnabled = false
    wsSSLCA = ''

    bleEnabled = false
    bleEnableField = false
    bleEnable = true
    bleRPCEnabledField = false
    bleRPCEnabled = false
    bleObserverEnabledField = false
    bleObserverEnabled = false

    matterEnabled = false
    matterEnableField = false
    matterEnable = true

    cloudEnabled = false
    cloudEnableField = false
    cloudEnable = true

    otaEnabled = false
    otaStageEnabled = false
    otaStage = 'stable'
    otaAutoUpdateEnabled = false
    otaAutoUpdate = 'off'

    authEnabled = false
    authPassEnabled = false
    authPass = ''

    wifiEnabled = false
    wifiSTAEnabled = false
    wifiSSIDEnabled = false
    wifiSSID = ''
    wifiPassEnabled = false
    wifiPass = ''
  }

  function asRecord(value: unknown): Record<string, unknown> | null {
    return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null
  }

  function hasOnlyKeys(record: Record<string, unknown>, keys: string[]): boolean {
    return Object.keys(record).every((key) => keys.includes(key))
  }

  function boolField(record: Record<string, unknown>, key: string): boolean | undefined {
    const value = record[key]
    return typeof value === 'boolean' ? value : undefined
  }

  function stringField(record: Record<string, unknown>, key: string): string | undefined {
    const value = record[key]
    return typeof value === 'string' ? value : undefined
  }

  function numberField(record: Record<string, unknown>, key: string): number | undefined {
    const value = record[key]
    return typeof value === 'number' ? value : undefined
  }

  function hydrateFormFromTemplate(template: Record<string, unknown>): { ok: true } | { ok: false; reason: string } {
    resetFormState()
    for (const [sectionName, rawSection] of Object.entries(template)) {
      const section = sectionName.trim().toLowerCase()
      const record = asRecord(rawSection)
      if (!record) {
        return { ok: false, reason: `Template section "${sectionName}" is not an object.` }
      }
      switch (section) {
        case 'sys': {
          if (!hasOnlyKeys(record, ['name', 'device', 'tz', 'location', 'sntp', 'clock_mode', 'dbg', 'debug', 'rpc_udp', 'lat', 'lng', 'lon', 'profile', 'addon_type'])) {
            return { ok: false, reason: 'Template sys section contains fields the form cannot represent.' }
          }
          const device = record.device ? asRecord(record.device) : null
          const location = record.location ? asRecord(record.location) : null
          const sntp = record.sntp ? asRecord(record.sntp) : null
          const dbg = record.dbg ? asRecord(record.dbg) : null
          const debug = record.debug ? asRecord(record.debug) : null
          const rpcUDP = record.rpc_udp ? asRecord(record.rpc_udp) : null
          if ((record.device && !device) || (record.location && !location) || (record.sntp && !sntp) || (record.dbg && !dbg) || (record.debug && !debug) || (record.rpc_udp && !rpcUDP)) {
            return { ok: false, reason: 'Template sys section contains nested values the form cannot represent.' }
          }
          if (device && !hasOnlyKeys(device, ['name', 'eco_mode', 'discoverable'])) {
            return { ok: false, reason: 'Template sys.device section contains unsupported fields.' }
          }
          if (location && !hasOnlyKeys(location, ['tz', 'lat', 'lon'])) {
            return { ok: false, reason: 'Template sys.location section contains unsupported fields.' }
          }
          if (sntp && !hasOnlyKeys(sntp, ['server'])) {
            return { ok: false, reason: 'Template sys.sntp section contains unsupported fields.' }
          }
          if (dbg && !hasOnlyKeys(dbg, ['websocket_enable', 'udp_addr'])) {
            return { ok: false, reason: 'Template sys.dbg section contains unsupported fields.' }
          }
          const debugWS = debug && debug.websocket ? asRecord(debug.websocket) : null
          const debugUDP = debug && debug.udp ? asRecord(debug.udp) : null
          if ((debug && debug.websocket && !debugWS) || (debug && debug.udp && !debugUDP)) {
            return { ok: false, reason: 'Template sys.debug section contains unsupported nested values.' }
          }
          if (debugWS && !hasOnlyKeys(debugWS, ['enable'])) {
            return { ok: false, reason: 'Template sys.debug.websocket section contains unsupported fields.' }
          }
          if (debugUDP && !hasOnlyKeys(debugUDP, ['addr'])) {
            return { ok: false, reason: 'Template sys.debug.udp section contains unsupported fields.' }
          }
          if (rpcUDP && !hasOnlyKeys(rpcUDP, ['port', 'listen_port'])) {
            return { ok: false, reason: 'Template sys.rpc_udp section contains unsupported fields.' }
          }

          sysEnabled = true
          const topName = stringField(record, 'name')
          const nestedName = device ? stringField(device, 'name') : undefined
          if (topName !== undefined || nestedName !== undefined) {
            if (topName !== undefined && nestedName !== undefined && topName !== nestedName) {
              return { ok: false, reason: 'Template sys name fields disagree and cannot be represented safely in the form.' }
            }
            sysNameEnabled = true
            sysName = topName ?? nestedName ?? sysName
          }
          const topTZ = stringField(record, 'tz')
          const nestedTZ = location ? stringField(location, 'tz') : undefined
          if (topTZ !== undefined || nestedTZ !== undefined) {
            if (topTZ !== undefined && nestedTZ !== undefined && topTZ !== nestedTZ) {
              return { ok: false, reason: 'Template sys timezone fields disagree and cannot be represented safely in the form.' }
            }
            sysTZEnabled = true
            sysTZ = topTZ ?? nestedTZ ?? sysTZ
          }
          const topLat = numberField(record, 'lat')
          const nestedLat = location ? numberField(location, 'lat') : undefined
          if (topLat !== undefined || nestedLat !== undefined) {
            if (topLat !== undefined && nestedLat !== undefined && topLat !== nestedLat) {
              return { ok: false, reason: 'Template sys latitude fields disagree and cannot be represented safely in the form.' }
            }
            sysLatEnabled = true
            sysLat = String(topLat ?? nestedLat ?? '')
          }
          const topLon = numberField(record, 'lng') ?? numberField(record, 'lon')
          const nestedLon = location ? numberField(location, 'lon') : undefined
          if (topLon !== undefined || nestedLon !== undefined) {
            if (topLon !== undefined && nestedLon !== undefined && topLon !== nestedLon) {
              return { ok: false, reason: 'Template sys longitude fields disagree and cannot be represented safely in the form.' }
            }
            sysLonEnabled = true
            sysLon = String(topLon ?? nestedLon ?? '')
          }
          const sntpServer = sntp ? stringField(sntp, 'server') : undefined
          if (sntpServer !== undefined) {
            sysSNTPEnabled = true
            sysSNTP = sntpServer
          }
          const clockMode = numberField(record, 'clock_mode')
          if (clockMode !== undefined) {
            if (clockMode !== 0 && clockMode !== 1) {
              return { ok: false, reason: 'Template sys clock_mode is not representable in the form.' }
            }
            sysTimeFormatEnabled = true
            sysTimeFormat = clockMode === 1 ? '12h' : '24h'
          }
          const legacyDebugWS = dbg ? boolField(dbg, 'websocket_enable') : undefined
          const nestedDebugWebsocket = debugWS ? boolField(debugWS, 'enable') : undefined
          const finalDebugWS = legacyDebugWS !== undefined ? legacyDebugWS : nestedDebugWebsocket
          if (finalDebugWS !== undefined) {
            sysDebugWSEnabled = true
            sysDebugWS = finalDebugWS
          }
          const legacyDebugUDPHost = dbg ? stringField(dbg, 'udp_addr') : undefined
          const nestedDebugUDPHost = debugUDP ? stringField(debugUDP, 'addr') : undefined
          const debugUDPHost = legacyDebugUDPHost ?? nestedDebugUDPHost
          if (debugUDPHost !== undefined) {
            sysDebugUDPHostEnabled = true
            sysDebugUDPHost = debugUDPHost
          }
          const rpcUDPPort = rpcUDP ? (numberField(rpcUDP, 'listen_port') ?? numberField(rpcUDP, 'port')) : undefined
          if (rpcUDPPort !== undefined) {
            sysRPCUDPPortEnabled = true
            sysRPCUDPPort = String(rpcUDPPort)
          }
          const ecoMode = device ? boolField(device, 'eco_mode') : undefined
          if (ecoMode !== undefined) {
            sysEcoEnabled = true
            sysEco = ecoMode
          }
          const discoverable = device ? boolField(device, 'discoverable') : undefined
          if (discoverable !== undefined) {
            sysDiscoverableEnabled = true
            sysDiscoverable = discoverable
          }
          break
        }
        case 'mqtt': {
          if (!hasOnlyKeys(record, ['enable', 'server', 'client_id', 'id', 'topic_prefix', 'user', 'pass', 'ssl_ca', 'rpc_ntf', 'status_ntf', 'enable_rpc', 'enable_control', 'use_client_cert'])) {
            return { ok: false, reason: 'Template mqtt section contains unsupported fields.' }
          }
          const clientID = stringField(record, 'client_id')
          const aliasID = stringField(record, 'id')
          if (clientID !== undefined && aliasID !== undefined && clientID !== aliasID) {
            return { ok: false, reason: 'Template mqtt client identifiers disagree and cannot be represented safely in the form.' }
          }
          mqttEnabled = true
          const mqttEnableValue = boolField(record, 'enable')
          if (mqttEnableValue !== undefined) {
            mqttEnableField = true
            mqttEnable = mqttEnableValue
          }
          const mqttServerValue = stringField(record, 'server')
          if (mqttServerValue !== undefined) {
            mqttServerEnabled = true
            mqttServer = mqttServerValue
          }
          const mqttClientValue = clientID ?? aliasID
          if (mqttClientValue !== undefined) {
            mqttClientIDEnabled = true
            mqttClientID = mqttClientValue
          }
          const mqttTopicValue = stringField(record, 'topic_prefix')
          if (mqttTopicValue !== undefined) {
            mqttTopicPrefixEnabled = true
            mqttTopicPrefix = mqttTopicValue
          }
          const mqttUserValue = stringField(record, 'user')
          if (mqttUserValue !== undefined) {
            mqttUserEnabled = true
            mqttUser = mqttUserValue
          }
          const mqttPassValue = stringField(record, 'pass')
          if (mqttPassValue !== undefined) {
            mqttPassEnabled = true
            mqttPass = mqttPassValue
          }
          const mqttSSLCAValue = stringField(record, 'ssl_ca')
          if (mqttSSLCAValue !== undefined) {
            mqttSSLCAEnabled = true
            mqttSSLCA = mqttSSLCAValue
          }
          const mqttRPCValue = boolField(record, 'rpc_ntf')
          if (mqttRPCValue !== undefined) {
            mqttRPCNtfEnabled = true
            mqttRPCNtf = mqttRPCValue
          }
          const mqttStatusValue = boolField(record, 'status_ntf')
          if (mqttStatusValue !== undefined) {
            mqttStatusNtfEnabled = true
            mqttStatusNtf = mqttStatusValue
          }
          const mqttEnableRPCValue = boolField(record, 'enable_rpc')
          if (mqttEnableRPCValue !== undefined) {
            mqttEnableRPCEnabled = true
            mqttEnableRPC = mqttEnableRPCValue
          }
          const mqttEnableControlValue = boolField(record, 'enable_control')
          if (mqttEnableControlValue !== undefined) {
            mqttEnableControlEnabled = true
            mqttEnableControl = mqttEnableControlValue
          }
          const mqttUseClientCertValue = boolField(record, 'use_client_cert')
          if (mqttUseClientCertValue !== undefined) {
            mqttUseClientCertEnabled = true
            mqttUseClientCert = mqttUseClientCertValue
          }
          break
        }
        case 'ws': {
          if (!hasOnlyKeys(record, ['enable', 'server', 'tls_mode', 'ssl_ca'])) {
            return { ok: false, reason: 'Template ws section contains unsupported fields.' }
          }
          const tlsMode = stringField(record, 'tls_mode')
          if (tlsMode !== undefined && tlsMode !== 'no_validation' && tlsMode !== 'default' && tlsMode !== 'user') {
            return { ok: false, reason: 'Template ws tls_mode is not representable in the form.' }
          }
          wsEnabled = true
          const wsEnableValue = boolField(record, 'enable')
          if (wsEnableValue !== undefined) {
            wsEnableField = true
            wsEnable = wsEnableValue
          }
          const wsServerValue = stringField(record, 'server')
          if (wsServerValue !== undefined) {
            wsServerEnabled = true
            wsServer = wsServerValue
          }
          const wsSSLCAValue = stringField(record, 'ssl_ca')
          const inferredTLSMode = inferWSTLSMode(wsServerValue, wsSSLCAValue, tlsMode)
          if (inferredTLSMode !== undefined) {
            wsTLSModeEnabled = true
            wsTLSMode = inferredTLSMode
          }
          if (wsSSLCAValue !== undefined && wsSSLCAValue !== '*') {
            wsSSLCAEnabled = true
            wsSSLCA = wsSSLCAValue
          }
          break
        }
        case 'ble': {
          if (!hasOnlyKeys(record, ['enable', 'rpc', 'observer'])) {
            return { ok: false, reason: 'Template ble section contains unsupported fields.' }
          }
          const rpc = record.rpc ? asRecord(record.rpc) : null
          const observer = record.observer ? asRecord(record.observer) : null
          if ((record.rpc && !rpc) || (record.observer && !observer)) {
            return { ok: false, reason: 'Template ble section contains nested values the form cannot represent.' }
          }
          if (rpc && !hasOnlyKeys(rpc, ['enable'])) {
            return { ok: false, reason: 'Template ble.rpc section contains unsupported fields.' }
          }
          if (observer && !hasOnlyKeys(observer, ['enable'])) {
            return { ok: false, reason: 'Template ble.observer section contains unsupported fields.' }
          }
          bleEnabled = true
          const bleEnableValue = boolField(record, 'enable')
          if (bleEnableValue !== undefined) {
            bleEnableField = true
            bleEnable = bleEnableValue
          }
          const bleRPCValue = rpc ? boolField(rpc, 'enable') : undefined
          if (bleRPCValue !== undefined) {
            bleRPCEnabledField = true
            bleRPCEnabled = bleRPCValue
          }
          const bleObserverValue = observer ? boolField(observer, 'enable') : undefined
          if (bleObserverValue !== undefined) {
            bleObserverEnabledField = true
            bleObserverEnabled = bleObserverValue
          }
          break
        }
        case 'matter': {
          if (!hasOnlyKeys(record, ['enable'])) {
            return { ok: false, reason: 'Template matter section contains unsupported fields.' }
          }
          matterEnabled = true
          const matterEnableValue = boolField(record, 'enable')
          if (matterEnableValue !== undefined) {
            matterEnableField = true
            matterEnable = matterEnableValue
          }
          break
        }
        case 'cloud': {
          if (!hasOnlyKeys(record, ['enable'])) {
            return { ok: false, reason: 'Template cloud section contains unsupported fields.' }
          }
          cloudEnabled = true
          const cloudEnableValue = boolField(record, 'enable')
          if (cloudEnableValue !== undefined) {
            cloudEnableField = true
            cloudEnable = cloudEnableValue
          }
          break
        }
        case 'ota': {
          if (!hasOnlyKeys(record, ['stage', 'auto_update'])) {
            return { ok: false, reason: 'Template ota section contains unsupported fields.' }
          }
          const stageValue = stringField(record, 'stage')
          if (stageValue !== undefined && stageValue !== 'stable' && stageValue !== 'beta') {
            return { ok: false, reason: 'Template ota stage is not representable in the form.' }
          }
          const autoUpdateValue = stringField(record, 'auto_update')
          if (autoUpdateValue !== undefined && autoUpdateValue !== 'off' && autoUpdateValue !== 'stable' && autoUpdateValue !== 'beta') {
            return { ok: false, reason: 'Template ota auto_update is not representable in the form.' }
          }
          otaEnabled = true
          if (stageValue !== undefined) {
            otaStageEnabled = true
            otaStage = stageValue
          }
          if (autoUpdateValue !== undefined) {
            otaAutoUpdateEnabled = true
            otaAutoUpdate = autoUpdateValue
          }
          break
        }
        case 'auth': {
          if (!hasOnlyKeys(record, ['pass'])) {
            return { ok: false, reason: 'Template auth section contains unsupported fields.' }
          }
          authEnabled = true
          const passValue = stringField(record, 'pass')
          if (passValue !== undefined) {
            authPassEnabled = true
            authPass = passValue
          }
          break
        }
        case 'wifi': {
          if (!hasOnlyKeys(record, ['sta'])) {
            return { ok: false, reason: 'Template wifi section contains unsupported fields.' }
          }
          const sta = record.sta ? asRecord(record.sta) : null
          if (record.sta && !sta) {
            return { ok: false, reason: 'Template wifi.sta section is not representable in the form.' }
          }
          if (sta && !hasOnlyKeys(sta, ['enable', 'ssid', 'pass'])) {
            return { ok: false, reason: 'Template wifi.sta section contains unsupported fields.' }
          }
          wifiEnabled = true
          const staEnabled = sta ? boolField(sta, 'enable') : undefined
          if (staEnabled !== undefined) wifiSTAEnabled = staEnabled
          const ssidValue = sta ? stringField(sta, 'ssid') : undefined
          if (ssidValue !== undefined) {
            wifiSSIDEnabled = true
            wifiSSID = ssidValue
          }
          const passValue = sta ? stringField(sta, 'pass') : undefined
          if (passValue !== undefined) {
            wifiPassEnabled = true
            wifiPass = passValue
          }
          break
        }
        default:
          return { ok: false, reason: `Template section "${sectionName}" is not supported by the form editor.` }
      }
    }
    return { ok: true }
  }

  function buildTemplate() {
    const out: Record<string, unknown> = {}

    if (sysEnabled) {
      const sys: Record<string, unknown> = {}
      const deviceCfg: Record<string, unknown> = {}
      const location: Record<string, unknown> = {}
      const sntp: Record<string, unknown> = {}
      const debug: Record<string, unknown> = {}
      const debugWS: Record<string, unknown> = {}
      const debugUDP: Record<string, unknown> = {}
      const rpcUDP: Record<string, unknown> = {}

      if (sysNameEnabled) deviceCfg.name = sysName
      if (sysEcoEnabled) deviceCfg.eco_mode = sysEco
      if (sysDiscoverableEnabled) deviceCfg.discoverable = sysDiscoverable
      if (sysTZEnabled) location.tz = sysTZ
      if (sysSNTPEnabled) sntp.server = sysSNTP
      if (sysTimeFormatEnabled) sys.clock_mode = sysTimeFormat === '12h' ? 1 : 0
      if (sysDebugWSEnabled) debugWS.enable = sysDebugWS
      if (sysDebugUDPHostEnabled && sysDebugUDPHost.trim()) debugUDP.addr = sysDebugUDPHost.trim()
      if (sysRPCUDPPortEnabled) {
        const port = maybeNum(sysRPCUDPPort)
        rpcUDP.listen_port = port === undefined ? 0 : port
      }
      if (sysLatEnabled) {
        const lat = maybeNum(sysLat)
        if (lat !== undefined) {
          location.lat = lat
        }
      }
      if (sysLonEnabled) {
        const lon = maybeNum(sysLon)
        if (lon !== undefined) {
          location.lon = lon
        }
      }
      if (Object.keys(deviceCfg).length > 0) sys.device = deviceCfg
      if (Object.keys(location).length > 0) sys.location = location
      if (Object.keys(sntp).length > 0) sys.sntp = sntp
      if (Object.keys(debugWS).length > 0) debug.websocket = debugWS
      if (Object.keys(debugUDP).length > 0) debug.udp = debugUDP
      if (Object.keys(debug).length > 0) sys.debug = debug
      if (Object.keys(rpcUDP).length > 0) sys.rpc_udp = rpcUDP
      if (Object.keys(sys).length > 0) out.sys = sys
    }

    if (mqttEnabled) {
      const mqtt: Record<string, unknown> = {}
      if (mqttEnableField) mqtt.enable = mqttEnable
      if (mqttServerEnabled) mqtt.server = mqttServer
      if (mqttClientIDEnabled) {
        mqtt.client_id = mqttClientID
        mqtt.id = mqttClientID
      }
      if (mqttTopicPrefixEnabled) mqtt.topic_prefix = mqttTopicPrefix
      if (mqttUserEnabled) mqtt.user = mqttUser
      if (mqttPassEnabled) mqtt.pass = mqttPass
      if (mqttSSLCAEnabled && mqttSSLCA !== '') mqtt.ssl_ca = mqttSSLCA
      if (mqttRPCNtfEnabled) mqtt.rpc_ntf = mqttRPCNtf
      if (mqttStatusNtfEnabled) mqtt.status_ntf = mqttStatusNtf
      if (mqttEnableRPCEnabled) mqtt.enable_rpc = mqttEnableRPC
      if (mqttEnableControlEnabled) mqtt.enable_control = mqttEnableControl
      if (mqttUseClientCertEnabled) mqtt.use_client_cert = mqttUseClientCert
      if (Object.keys(mqtt).length > 0) out.mqtt = mqtt
    }

    if (wsEnabled) {
      const ws: Record<string, unknown> = {}
      if (wsEnableField) ws.enable = wsEnable
      if (wsServerEnabled) ws.server = wsServer
      if (isTLSServerURL(wsServer)) {
        if (wsTLSModeEnabled) ws.tls_mode = wsTLSMode
        if (wsSSLCAEnabled && wsTLSMode === 'user') ws.ssl_ca = wsSSLCA
      }
      if (Object.keys(ws).length > 0) out.ws = ws
    }

    if (bleEnabled) {
      const ble: Record<string, unknown> = {}
      if (bleEnableField) ble.enable = bleEnable
      if (bleRPCEnabledField) ble.rpc = { enable: bleRPCEnabled }
      if (bleObserverEnabledField) ble.observer = { enable: bleObserverEnabled }
      if (Object.keys(ble).length > 0) out.ble = ble
    }

    if (matterEnabled) {
      const matter: Record<string, unknown> = {}
      if (matterEnableField) matter.enable = matterEnable
      if (Object.keys(matter).length > 0) out.matter = matter
    }

    if (cloudEnabled) {
      const cloud: Record<string, unknown> = {}
      if (cloudEnableField) cloud.enable = cloudEnable
      if (Object.keys(cloud).length > 0) out.cloud = cloud
    }

    if (otaEnabled) {
      const ota: Record<string, unknown> = {}
      if (otaStageEnabled) ota.stage = otaStage
      if (otaAutoUpdateEnabled) ota.auto_update = otaAutoUpdate
      if (Object.keys(ota).length > 0) out.ota = ota
    }

    if (authEnabled) {
      const auth: Record<string, unknown> = {}
      if (authPassEnabled) auth.pass = authPass
      if (Object.keys(auth).length > 0) out.auth = auth
    }

    if (wifiEnabled) {
      const wifi: Record<string, unknown> = {}
      const sta: Record<string, unknown> = {}
      if (wifiSTAEnabled) sta.enable = true
      if (wifiSSIDEnabled) sta.ssid = wifiSSID
      if (wifiPassEnabled) sta.pass = wifiPass
      if (Object.keys(sta).length > 0) wifi.sta = sta
      if (Object.keys(wifi).length > 0) out.wifi = wifi
    }

    return out
  }

  function syncJSONFromForm() {
    jsonText = JSON.stringify(buildTemplate(), null, 2)
  }

  function setView(mode: 'form' | 'json') {
    if (mode === 'json') syncJSONFromForm()
    viewMode = mode
  }

  async function saveCurrentTemplate() {
    if (!templateName.trim()) {
      error = 'Template name is required'
      return
    }
    try {
      const body = viewMode === 'json' ? jsonText : JSON.stringify(buildTemplate(), null, 2)
      await api.saveTemplate(templateName.trim(), body, selectedTemplateCredentialRef)
      templateNames = await api.listTemplates()
      selectedTemplate = templateName.trim()
      error = ''
      errorDetails = ''
    } catch (err) {
      captureError(err)
    }
  }

  async function deleteCurrentTemplate() {
    if (!selectedTemplate) return
    const name = selectedTemplate
    try {
      await api.deleteTemplate(name)
      templateNames = await api.listTemplates()
      selectedTemplate = ''
      templateName = ''
      error = ''
      errorDetails = ''
    } catch (err) {
      captureError(err)
    }
  }

  async function renameCurrentTemplate() {
    const oldName = selectedTemplate
    const newName = templateName.trim()
    if (!oldName || !newName || oldName === newName) return
    try {
      const body = viewMode === 'json' ? jsonText : JSON.stringify(buildTemplate(), null, 2)
      await api.saveTemplate(newName, body, selectedTemplateCredentialRef)
      await api.deleteTemplate(oldName)
      templateNames = await api.listTemplates()
      selectedTemplate = newName
      error = ''
      errorDetails = ''
    } catch (err) {
      captureError(err)
    }
  }

  async function loadCurrentTemplate() {
    if (!selectedTemplate) return
    try {
      const loaded = await api.getTemplate(selectedTemplate)
      jsonText = loaded.content
      selectedTemplateCredentialRef = loaded.credential_ref || ''
      templateName = selectedTemplate
      clearTemplateLoadNotice()
      const parsed = asRecord(JSON.parse(loaded.content))
      const hydrated = parsed
        ? hydrateFormFromTemplate(parsed)
        : { ok: false as const, reason: 'Template root is not an object and cannot be represented in the form.' }
      if (hydrated.ok) {
        viewMode = 'form'
      } else {
        viewMode = 'json'
        templateLoadNotice = `Loaded in JSON mode: ${hydrated.reason}`
      }
      error = ''
      errorDetails = ''
    } catch (err) {
      captureError(err)
    }
  }

  async function runProvision() {
    running = true
    error = ''
    errorDetails = ''
    try {
      const template = viewMode === 'json' ? JSON.parse(jsonText) : buildTemplate()
      results = await api.provision(
        selectedDevices().map((device) => device.ip),
        template,
        selectedTemplateCredentialRef,
      )
    } catch (err) {
      captureError(err)
    } finally {
      running = false
    }
  }
</script>

<ErrorNotice summary={error} details={errorDetails} />

<div class="row g-3">
  <div class="col-lg-6 provision-devices-col">
    {#if selected.size > 0}
      <div class="card bg-dark border-secondary">
        <div class="card-body">
          <h2 class="h6">Precheck Summary</h2>
          {#if precheckTemplateError}
            <div class="alert alert-warning py-2 mb-2">{precheckTemplateError}</div>
          {/if}
          <p class="mb-2"><span class="badge bg-success me-2">{precheckEligibleCount}</span> eligible</p>
          <p class="mb-2"><span class="badge bg-warning text-dark me-2">{precheckIssues.length}</span> predicted to be skipped</p>
          <div class="d-flex gap-2 flex-wrap mb-2">
            <button class="btn btn-sm btn-outline-light" on:click={selectOnlyEligible} disabled={precheckIssues.length === 0 || Boolean(precheckTemplateError)}>Select Only Eligible</button>
            <button class="btn btn-sm btn-outline-light" on:click={copySkippedIPs} disabled={precheckIssues.length === 0}>Copy Skipped IPs</button>
            {#if copiedSkipped}
              <span class="badge bg-success">copied</span>
            {/if}
            {#if precheckReasonCounts.auth}
              <span class="badge bg-warning text-dark">auth: {precheckReasonCounts.auth}</span>
            {/if}
            {#if precheckReasonCounts.generation}
              <span class="badge bg-info text-dark">generation: {precheckReasonCounts.generation}</span>
            {/if}
          </div>
          {#if precheckIssues.length > 0}
            <div class="table-responsive">
              <table class="table table-dark table-striped table-sm mb-0">
                <thead>
                  <tr><th>IP</th><th>Device</th><th>Type</th><th>Reason</th></tr>
                </thead>
                <tbody>
                  {#each precheckIssues as issue}
                    <tr>
                      <td>{issue.ip}</td>
                      <td>{issue.label}</td>
                      <td><span class={`badge ${reasonBadgeClass(issue.category)}`}>{reasonBadgeText(issue.category)}</span></td>
                      <td>{issue.reason}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      </div>
    {/if}

    <div class="card bg-dark border-secondary">
      <div class="card-header d-flex justify-content-between align-items-center">
        <span>Select Devices</span>
        <div class="d-flex gap-2">
          <button class="btn btn-sm btn-outline-light" on:click={selectAll}>All</button>
          <button class="btn btn-sm btn-outline-light" on:click={selectNone}>None</button>
        </div>
      </div>
      <div class="card-body p-0">
        {#if loading}
          <div class="p-2 text-secondary">Loading devices...</div>
        {:else if devices.length === 0}
          <div class="p-2 text-secondary">No devices enrolled yet.</div>
        {:else}
          <div class="table-responsive device-list-scroll">
            <table class="table table-dark table-striped align-middle table-nowrap mb-0">
              <thead>
                <tr>
                  <th></th>
                  <th>IP</th>
                  <th>Name</th>
                  <th>Gen</th>
                </tr>
              </thead>
              <tbody>
                {#each devices as device}
                  <tr>
                    <td><input type="checkbox" class="form-check-input" checked={selected.has(device.mac)} on:change={(e) => toggle(device.mac, (e.currentTarget as HTMLInputElement).checked)} /></td>
                    <td>{device.ip}</td>
                    <td>{device.name || device.serial || device.mac}</td>
                    <td><span class={`badge ${device.gen <= 1 ? 'bg-danger' : 'bg-success'}`}>{device.gen <= 1 ? 'Gen1' : `Gen${device.gen}`}</span></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
      <div class="card-footer p-2 text-secondary">{selected.size} of {devices.length} selected ({selectedGenLabel()})</div>
    </div>
  </div>

  <div class="col-lg-6 provision-settings-col">
    <div class="card bg-dark border-secondary">
      <div class="card-header d-flex justify-content-between align-items-center gap-2 flex-wrap">
        <div class="d-flex gap-2 align-items-center flex-wrap">
          <select class="form-select toolbar-select-lg" bind:value={selectedTemplate}>
            <option value="">load template</option>
            {#each templateNames as name}
              <option value={name}>{name}</option>
            {/each}
          </select>
          <button class="btn btn-sm btn-outline-light" on:click={loadCurrentTemplate} disabled={!selectedTemplate}>Load</button>
          <button class="btn btn-sm btn-outline-danger" on:click={deleteCurrentTemplate} disabled={!selectedTemplate}>Delete</button>
          <input class="form-control toolbar-input-md" placeholder="template name" bind:value={templateName} />
          <select class="form-select toolbar-select-md" bind:value={selectedTemplateCredentialRef}>
            <option value="">credential: none</option>
            {#each credentials as credential}
              <option value={credential.name}>{credential.name}</option>
            {/each}
          </select>
          <button class="btn btn-sm btn-outline-light" on:click={saveCurrentTemplate}>Save</button>
          <button class="btn btn-sm btn-outline-secondary" on:click={renameCurrentTemplate} disabled={!selectedTemplate || !templateName.trim() || selectedTemplate === templateName.trim()}>Rename</button>
        </div>
        {#if groupCredentialHint}
          <span class="text-secondary">{groupCredentialHint}</span>
        {/if}
        <div class="d-flex gap-2">
          <button class={`btn btn-sm ${viewMode === 'form' ? 'btn-warning text-dark' : 'btn-outline-light'}`} on:click={() => setView('form')}>Form</button>
          <button class={`btn btn-sm ${viewMode === 'json' ? 'btn-warning text-dark' : 'btn-outline-light'}`} on:click={() => setView('json')}>JSON</button>
        </div>
      </div>

      <div class="card-body">
        {#if templateLoadNotice}
          <div class="alert alert-info py-2">{templateLoadNotice}</div>
        {/if}
        {#if viewMode === 'json'}
          <textarea class="form-control font-monospace" rows="36" bind:value={jsonText}></textarea>
        {:else}
          <div class="d-flex flex-column gap-3">
            <div class="card bg-black border-secondary">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (sysOpen = !sysOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (sysOpen = !sysOpen)} style="cursor: pointer">
                  <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer"><input type="checkbox" class="form-check-input" bind:checked={sysEnabled} on:click|stopPropagation /> <strong>sys</strong> - System & Location</label>
                  <span class="text-secondary">{sysVisible ? '▾' : '▸'}</span>
                </div>
                {#if sysVisible}
                  <div class="row g-2">
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysNameEnabled} disabled={!sysEnabled} />Device Name</label><input class="form-control" bind:value={sysName} disabled={!sysEnabled || !sysNameEnabled} /></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysTZEnabled} disabled={!sysEnabled} />Timezone</label><input class="form-control" bind:value={sysTZ} disabled={!sysEnabled || !sysTZEnabled} /></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysSNTPEnabled} disabled={!sysEnabled} />SNTP Server</label><input class="form-control" bind:value={sysSNTP} disabled={!sysEnabled || !sysSNTPEnabled} /></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysTimeFormatEnabled} disabled={!sysEnabled} />Time Format</label><select class="form-select" bind:value={sysTimeFormat} disabled={!sysEnabled || !sysTimeFormatEnabled}><option value="24h">24h</option><option value="12h">12h</option></select></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysDebugWSEnabled} disabled={!sysEnabled} />Debug WebSocket (stream logs)</label><select class="form-select" bind:value={sysDebugWS} disabled={!sysEnabled || !sysDebugWSEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysDebugUDPHostEnabled} disabled={!sysEnabled} />Debug UDP Host</label><input class="form-control" placeholder="host:port" bind:value={sysDebugUDPHost} disabled={!sysEnabled || !sysDebugUDPHostEnabled} /></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysRPCUDPPortEnabled} disabled={!sysEnabled} />RPC UDP Port (0=off)</label><input class="form-control" type="number" min="0" bind:value={sysRPCUDPPort} disabled={!sysEnabled || !sysRPCUDPPortEnabled} /></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysLatEnabled} disabled={!sysEnabled} />Latitude</label><input class="form-control" type="number" step="0.0001" bind:value={sysLat} disabled={!sysEnabled || !sysLatEnabled} /></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysLonEnabled} disabled={!sysEnabled} />Longitude</label><input class="form-control" type="number" step="0.0001" bind:value={sysLon} disabled={!sysEnabled || !sysLonEnabled} /></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysEcoEnabled} disabled={!sysEnabled} />Eco Mode</label><select class="form-select" bind:value={sysEco} disabled={!sysEnabled || !sysEcoEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysDiscoverableEnabled} disabled={!sysEnabled} />Discoverable</label><select class="form-select" bind:value={sysDiscoverable} disabled={!sysEnabled || !sysDiscoverableEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  </div>
                {/if}
              </div>
            </div>

            <div class="card bg-black border-secondary">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (mqttOpen = !mqttOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (mqttOpen = !mqttOpen)} style="cursor: pointer">
                  <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer"><input type="checkbox" class="form-check-input" bind:checked={mqttEnabled} on:click|stopPropagation /> <strong>mqtt</strong> - MQTT Broker</label>
                  <span class="text-secondary">{mqttVisible ? '▾' : '▸'}</span>
                </div>
                {#if mqttVisible}
                  <div class="row g-2">
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnableField} disabled={!mqttEnabled} />Enable MQTT</label><select class="form-select" bind:value={mqttEnable} disabled={!mqttEnabled || !mqttEnableField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttServerEnabled} disabled={!mqttEnabled} />Broker</label><input class="form-control" bind:value={mqttServer} disabled={!mqttEnabled || !mqttServerEnabled} /></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttClientIDEnabled} disabled={!mqttEnabled} />Client ID</label><input class="form-control" bind:value={mqttClientID} disabled={!mqttEnabled || !mqttClientIDEnabled} /></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttTopicPrefixEnabled} disabled={!mqttEnabled} />Topic Prefix</label><input class="form-control" bind:value={mqttTopicPrefix} disabled={!mqttEnabled || !mqttTopicPrefixEnabled} /></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttUserEnabled} disabled={!mqttEnabled} />Username</label><input class="form-control" bind:value={mqttUser} disabled={!mqttEnabled || !mqttUserEnabled} /></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttPassEnabled} disabled={!mqttEnabled} />Password</label><input class="form-control" type="password" bind:value={mqttPass} disabled={!mqttEnabled || !mqttPassEnabled} /></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttSSLCAEnabled} disabled={!mqttEnabled} />SSL CA</label><select class="form-select" bind:value={mqttSSLCA} disabled={!mqttEnabled || !mqttSSLCAEnabled}><option value="">— none (no TLS) —</option><option value="*">* (disable cert validation)</option><option value="ca.pem">ca.pem (built-in CA)</option><option value="user_ca.pem">user_ca.pem (user CA)</option></select></div>
                    <div class="col-md-3"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttRPCNtfEnabled} disabled={!mqttEnabled} />RPC notifications</label><select class="form-select" bind:value={mqttRPCNtf} disabled={!mqttEnabled || !mqttRPCNtfEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-3"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttStatusNtfEnabled} disabled={!mqttEnabled} />Status updates</label><select class="form-select" bind:value={mqttStatusNtf} disabled={!mqttEnabled || !mqttStatusNtfEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-3"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnableRPCEnabled} disabled={!mqttEnabled} />Enable RPC</label><select class="form-select" bind:value={mqttEnableRPC} disabled={!mqttEnabled || !mqttEnableRPCEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-3"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnableControlEnabled} disabled={!mqttEnabled} />Enable control</label><select class="form-select" bind:value={mqttEnableControl} disabled={!mqttEnabled || !mqttEnableControlEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttUseClientCertEnabled} disabled={!mqttEnabled} />Use Client Certificate</label><select class="form-select" bind:value={mqttUseClientCert} disabled={!mqttEnabled || !mqttUseClientCertEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  </div>
                {/if}
              </div>
            </div>

            <div class="card bg-black border-secondary">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (wsOpen = !wsOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (wsOpen = !wsOpen)} style="cursor: pointer">
                  <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer"><input type="checkbox" class="form-check-input" bind:checked={wsEnabled} on:click|stopPropagation /> <strong>ws</strong> - WebSocket (Gen 2+)</label>
                  <span class="text-secondary">{wsVisible ? '▾' : '▸'}</span>
                </div>
                {#if wsVisible}
                  <div class="row g-2">
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wsEnableField} disabled={!wsEnabled} />Enable WebSocket</label><select class="form-select" bind:value={wsEnable} disabled={!wsEnabled || !wsEnableField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wsServerEnabled} disabled={!wsEnabled} />Server URL</label><input class="form-control" bind:value={wsServer} disabled={!wsEnabled || !wsServerEnabled} /></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wsTLSModeEnabled} disabled={!wsEnabled} />Connection type</label><select class="form-select" bind:value={wsTLSMode} disabled={!wsEnabled || !wsTLSModeEnabled}><option value="no_validation">TLS no validation</option><option value="default">Default TLS</option><option value="user">User TLS</option></select></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wsSSLCAEnabled} disabled={!wsEnabled} />SSL CA</label><input class="form-control" placeholder="* or ca.pem" bind:value={wsSSLCA} disabled={!wsEnabled || !wsSSLCAEnabled || wsTLSMode !== 'user'} /></div>
                  </div>
                  {#if wsServerEnabled && wsServer && !isTLSServerURL(wsServer)}
                    <div class="form-text mt-2">TLS settings are ignored for plain <code>ws://</code> endpoints. Use <code>wss://</code> for TLS.</div>
                  {/if}
                {/if}
              </div>
            </div>

            <div class="card bg-black border-secondary">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (bleOpen = !bleOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (bleOpen = !bleOpen)} style="cursor: pointer">
                  <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer"><input type="checkbox" class="form-check-input" bind:checked={bleEnabled} on:click|stopPropagation /> <strong>ble</strong> - Bluetooth (Gen 2+)</label>
                  <span class="text-secondary">{bleVisible ? '▾' : '▸'}</span>
                </div>
                {#if bleVisible}
                  <div class="row g-2">
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={bleEnableField} disabled={!bleEnabled} />Enable BLE</label><select class="form-select" bind:value={bleEnable} disabled={!bleEnabled || !bleEnableField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={bleRPCEnabledField} disabled={!bleEnabled} />Enable RPC over BLE</label><select class="form-select" bind:value={bleRPCEnabled} disabled={!bleEnabled || !bleRPCEnabledField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={bleObserverEnabledField} disabled={!bleEnabled} />Observer Mode</label><select class="form-select" bind:value={bleObserverEnabled} disabled={!bleEnabled || !bleObserverEnabledField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  </div>
                {/if}
              </div>
            </div>

            <div class="card bg-black border-secondary">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (matterOpen = !matterOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (matterOpen = !matterOpen)} style="cursor: pointer">
                  <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer"><input type="checkbox" class="form-check-input" bind:checked={matterEnabled} on:click|stopPropagation /> <strong>matter</strong> - Matter (Gen 2+)</label>
                  <span class="text-secondary">{matterVisible ? '▾' : '▸'}</span>
                </div>
                {#if matterVisible}
                  <div class="row g-2">
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={matterEnableField} disabled={!matterEnabled} />Enable Matter</label><select class="form-select" bind:value={matterEnable} disabled={!matterEnabled || !matterEnableField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  </div>
                {/if}
              </div>
            </div>

            <div class="card bg-black border-secondary">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (cloudOpen = !cloudOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (cloudOpen = !cloudOpen)} style="cursor: pointer">
                  <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer"><input type="checkbox" class="form-check-input" bind:checked={cloudEnabled} on:click|stopPropagation /> <strong>cloud</strong> - Shelly Cloud</label>
                  <span class="text-secondary">{cloudVisible ? '▾' : '▸'}</span>
                </div>
                {#if cloudVisible}
                  <div class="row g-2">
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={cloudEnableField} disabled={!cloudEnabled} />Enable Cloud</label><select class="form-select" bind:value={cloudEnable} disabled={!cloudEnabled || !cloudEnableField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  </div>
                {/if}
              </div>
            </div>

            <div class="card bg-black border-secondary">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (otaOpen = !otaOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (otaOpen = !otaOpen)} style="cursor: pointer">
                  <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer"><input type="checkbox" class="form-check-input" bind:checked={otaEnabled} on:click|stopPropagation /> <strong>ota</strong> - Firmware Update</label>
                  <span class="text-secondary">{otaVisible ? '▾' : '▸'}</span>
                </div>
                {#if otaVisible}
                  <div class="row g-2">
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={otaStageEnabled} disabled={!otaEnabled} />Stage</label><select class="form-select" bind:value={otaStage} disabled={!otaEnabled || !otaStageEnabled}><option value="stable">Stable</option><option value="beta">Beta</option></select></div>
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={otaAutoUpdateEnabled} disabled={!otaEnabled} />Update automatically</label><select class="form-select" bind:value={otaAutoUpdate} disabled={!otaEnabled || !otaAutoUpdateEnabled}><option value="off">Disable auto update</option><option value="stable">Enable update to stable version</option><option value="beta">Enable update to beta version</option></select><div class="text-secondary mt-2">BETA firmware may cause instability</div></div>
                  </div>
                {/if}
              </div>
            </div>

            <div class="card bg-black border-secondary">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (authOpen = !authOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (authOpen = !authOpen)} style="cursor: pointer">
                  <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer"><input type="checkbox" class="form-check-input" bind:checked={authEnabled} on:click|stopPropagation /> <strong>auth</strong> - Set Device Password (Gen 2+)</label>
                  <span class="text-secondary">{authVisible ? '▾' : '▸'}</span>
                </div>
                {#if authVisible}
                  <div class="row g-2">
                    <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={authPassEnabled} disabled={!authEnabled} />Password</label><input class="form-control" type="password" bind:value={authPass} disabled={!authEnabled || !authPassEnabled} /></div>
                  </div>
                {/if}
              </div>
            </div>

            <div class="card bg-black border-secondary">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (wifiOpen = !wifiOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (wifiOpen = !wifiOpen)} style="cursor: pointer">
                  <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer"><input type="checkbox" class="form-check-input" bind:checked={wifiEnabled} on:click|stopPropagation /> <strong>wifi</strong> - WiFi STA</label>
                  <span class="text-secondary">{wifiVisible ? '▾' : '▸'}</span>
                </div>
                {#if wifiVisible}
                  <div class="row g-2">
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wifiSTAEnabled} disabled={!wifiEnabled} />Enable STA</label><div class="text-secondary">On when section selected</div></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wifiSSIDEnabled} disabled={!wifiEnabled} />SSID</label><input class="form-control" bind:value={wifiSSID} disabled={!wifiEnabled || !wifiSSIDEnabled} /></div>
                    <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wifiPassEnabled} disabled={!wifiEnabled} />Password</label><input class="form-control" type="password" bind:value={wifiPass} disabled={!wifiEnabled || !wifiPassEnabled} /></div>
                  </div>
                {/if}
              </div>
            </div>
          </div>
        {/if}

        <div class="d-flex gap-2 mt-3 flex-wrap">
          <button class="btn btn-warning text-dark" on:click={runProvision} disabled={selected.size === 0 || running}>{running ? 'Provisioning...' : `Provision ${selected.size}`}</button>
          <button class="btn btn-outline-light" on:click={syncJSONFromForm} disabled={viewMode !== 'form'}>Sync JSON</button>
        </div>
      </div>
    </div>
  </div>
</div>

{#if results.length}
  <div class="card bg-dark border-secondary mt-3">
    <div class="card-body">
      <h2 class="h5">Results</h2>
      <pre class="mb-0">{JSON.stringify(results, null, 2)}</pre>
    </div>
  </div>
{/if}
