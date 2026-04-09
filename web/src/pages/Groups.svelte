<script lang="ts">
  import { onMount } from 'svelte'
  import { APIError, api } from '../lib/api'
  import type { CredentialGroup, Device } from '../lib/types'
  import ErrorNotice from '../components/ErrorNotice.svelte'

  let groups: CredentialGroup[] = []
  let devices: Device[] = []
  let assignments: Record<string, string> = {}
  let selected = new Set<string>()

  let groupName = ''
  let groupUsername = 'admin'
  let groupPassword = ''
  let groupHA1 = ''
  let groupTags = ''
  let assignGroupName = ''

  let loading = false
  let saving = false
  let error = ''
  let errorDetails = ''
  let status = ''

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`
      return
    }
    error = (err as Error).message
    errorDetails = String(err)
  }

  function setStatus(message: string) {
    status = message
    setTimeout(() => {
      if (status === message) status = ''
    }, 2000)
  }

  async function load() {
    loading = true
    error = ''
    errorDetails = ''
    try {
      const [loadedGroups, loadedDevices, loadedAssignments] = await Promise.all([
        api.listCredentialGroups(),
        api.getDevices(),
        api.getCredentialGroupAssignments(),
      ])
      groups = loadedGroups
      devices = loadedDevices
      assignments = loadedAssignments.assignments
    } catch (err) {
      captureError(err)
    } finally {
      loading = false
    }
  }

  function toggle(mac: string, checked: boolean) {
    if (checked) selected.add(mac)
    else selected.delete(mac)
    selected = new Set(selected)
  }

  function selectAll() {
    selected = new Set(devices.map((device) => device.mac))
  }

  function clearSelection() {
    selected = new Set()
  }

  function editGroup(group: CredentialGroup) {
    groupName = group.name
    groupUsername = group.username
    groupPassword = group.password
    groupHA1 = group.ha1
    groupTags = (group.tags || []).join(', ')
  }

  async function saveGroup() {
    if (!groupName.trim() || !groupUsername.trim()) {
      error = 'Group name and username are required'
      errorDetails = ''
      return
    }
    if (!groupPassword.trim() && !groupHA1.trim()) {
      error = 'Group requires password or HA1'
      errorDetails = ''
      return
    }
    saving = true
    error = ''
    errorDetails = ''
    try {
      await api.saveCredentialGroup({
        name: groupName.trim(),
        username: groupUsername.trim(),
        password: groupPassword,
        ha1: groupHA1.trim(),
        tags: groupTags.split(',').map((item) => item.trim()).filter(Boolean),
      })
      await load()
      setStatus('Group saved')
    } catch (err) {
      captureError(err)
    } finally {
      saving = false
    }
  }

  async function removeGroup(name: string) {
    if (!confirm(`Delete group "${name}"?`)) return
    saving = true
    error = ''
    errorDetails = ''
    try {
      await api.deleteCredentialGroup(name)
      if (assignGroupName === name) assignGroupName = ''
      if (groupName === name) {
        groupName = ''
        groupUsername = 'admin'
        groupPassword = ''
        groupHA1 = ''
        groupTags = ''
      }
      await load()
      setStatus('Group deleted')
    } catch (err) {
      captureError(err)
    } finally {
      saving = false
    }
  }

  async function assignSelected() {
    if (selected.size === 0 || !assignGroupName.trim()) return
    saving = true
    error = ''
    errorDetails = ''
    try {
      await api.saveCredentialGroupAssignments([...selected], assignGroupName.trim())
      await load()
      setStatus(`Assigned ${selected.size} devices`)
    } catch (err) {
      captureError(err)
    } finally {
      saving = false
    }
  }

  async function unassignSelected() {
    if (selected.size === 0) return
    saving = true
    error = ''
    errorDetails = ''
    try {
      await api.saveCredentialGroupAssignments([...selected], '')
      await load()
      setStatus(`Unassigned ${selected.size} devices`)
    } catch (err) {
      captureError(err)
    } finally {
      saving = false
    }
  }

  onMount(() => void load())
</script>

<ErrorNotice summary={error} details={errorDetails} />
{#if status}
  <div class="alert alert-secondary">{status}</div>
{/if}

<div class="row g-3">
  <div class="col-lg-6">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Auth Groups</h2>
        <p class="text-secondary mb-3">Each group contains its own credentials. Group assignments are kept for auth-required device workflows.</p>
        <div class="d-flex flex-column gap-2 mb-3">
          {#if groups.length === 0}
            <div class="text-secondary">No groups created yet.</div>
          {:else}
            {#each groups as group}
              <div class="border rounded p-2">
                <div class="d-flex justify-content-between align-items-center gap-2">
                  <strong>{group.name}</strong>
                  <div class="d-flex gap-2">
                    <button class="btn btn-sm btn-outline-light" on:click={() => editGroup(group)}>Edit</button>
                    <button class="btn btn-sm btn-outline-danger" on:click={() => removeGroup(group.name)} disabled={saving}>Delete</button>
                  </div>
                </div>
                <div class="text-secondary">{group.username}</div>
              </div>
            {/each}
          {/if}
        </div>
        <label class="form-label" for="group-name">Group name</label>
        <input id="group-name" class="form-control mb-2" placeholder="Example: site-a" bind:value={groupName} />

        <label class="form-label" for="group-username">Group username</label>
        <input id="group-username" class="form-control mb-2" placeholder="Usually admin" bind:value={groupUsername} />

        <label class="form-label" for="group-password">Group password</label>
        <input id="group-password" class="form-control mb-2" type="password" placeholder="Leave empty if HA1 is set" bind:value={groupPassword} />

        <label class="form-label" for="group-ha1">Group HA1 (optional)</label>
        <input id="group-ha1" class="form-control mb-2" placeholder="Use when password is not available" bind:value={groupHA1} />

        <label class="form-label" for="group-tags">Group tags (optional)</label>
        <input id="group-tags" class="form-control mb-3" placeholder="Comma-separated labels" bind:value={groupTags} />

        <div class="d-flex gap-2 flex-wrap">
          <button class="btn btn-outline-light" on:click={saveGroup} disabled={saving}>Save Group</button>
          <button class="btn btn-outline-light" on:click={() => { groupName = ''; groupUsername = 'admin'; groupPassword = ''; groupHA1 = ''; groupTags = '' }} disabled={saving}>Clear</button>
        </div>
      </div>
    </div>
  </div>

  <div class="col-lg-6">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <div class="d-flex justify-content-between align-items-center gap-2 flex-wrap mb-3">
          <h2 class="h5 mb-0">Device Assignments</h2>
          <div class="d-flex gap-2 flex-wrap">
            <button class="btn btn-sm btn-outline-light" on:click={selectAll} disabled={loading}>All</button>
            <button class="btn btn-sm btn-outline-light" on:click={clearSelection} disabled={loading}>None</button>
          </div>
        </div>
        <div class="d-flex gap-2 align-items-center flex-wrap mb-3">
          <select class="form-select" bind:value={assignGroupName} style="width: 14rem">
            <option value="">select group</option>
            {#each groups as group}
              <option value={group.name}>{group.name}</option>
            {/each}
          </select>
          <button class="btn btn-outline-light" on:click={assignSelected} disabled={saving || selected.size === 0 || !assignGroupName}>Assign Selected</button>
          <button class="btn btn-outline-light" on:click={unassignSelected} disabled={saving || selected.size === 0}>Unassign Selected</button>
          <span class="text-secondary">{selected.size} selected</span>
        </div>
        {#if loading}
          <div class="text-secondary">Loading devices...</div>
        {:else if devices.length === 0}
          <div class="text-secondary">No devices available yet.</div>
        {:else}
          <div class="table-responsive device-list-scroll">
            <table class="table table-dark table-striped table-nowrap mb-0">
              <thead>
                <tr>
                  <th></th>
                  <th>Device</th>
                  <th>IP</th>
                  <th>MAC</th>
                  <th>Group</th>
                  <th>Credential</th>
                </tr>
              </thead>
              <tbody>
                {#each devices as device}
                  {@const groupName = assignments[device.mac] || ''}
                  {@const group = groups.find((item) => item.name === groupName)}
                  <tr>
                    <td><input type="checkbox" class="form-check-input" checked={selected.has(device.mac)} on:change={(e) => toggle(device.mac, (e.currentTarget as HTMLInputElement).checked)} /></td>
                    <td>{device.name || device.serial || device.mac}</td>
                    <td>{device.ip}</td>
                    <td class="font-monospace">{device.mac}</td>
                    <td>{groupName || 'none'}</td>
                    <td>{group ? `${group.name} (${group.username})` : 'none'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>
