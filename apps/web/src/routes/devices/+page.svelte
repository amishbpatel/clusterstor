<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import AppShell from '$lib/AppShell.svelte';
  import LoadingState from '$lib/LoadingState.svelte';
  import { api, getToken, formatBytes, type Device } from '$lib/api';

  let devices: Device[] = [];
  let loading = true;
  let working = '';
  let error = '';
  let success = '';
  let pairCode = '';
  let pairingBusy = false;
  type PairingPreview = {
    user_code: string;
    name: string;
    platform: string;
    agent_version?: string | null;
    peer_contribution_enabled: boolean;
    peer_contribution_bytes: number;
    expires_at: string;
    approved: boolean;
  };
  let pairPreview: PairingPreview | null = null;

  $: activeDevices = devices.filter((device) => device.status !== 'revoked' && !device.revoked_at);
  $: revokedDevices = devices.filter((device) => device.status === 'revoked' || !!device.revoked_at);
  $: peerDevices = activeDevices.filter((device) => device.peer_contribution_enabled);

  function platformLabel(platform: string) {
    const value = platform.toLowerCase();
    if (value.includes('win')) return 'Windows';
    if (value.includes('mac') || value.includes('darwin')) return 'macOS';
    if (value.includes('linux')) return 'Linux';
    return platform || 'Unknown';
  }

  function platformMark(platform: string) {
    const label = platformLabel(platform);
    if (label === 'Windows') return 'W';
    if (label === 'macOS') return 'M';
    if (label === 'Linux') return 'L';
    return '?';
  }

  function seenLabel(value?: string | null) {
    if (!value) return 'Never';
    return new Date(value).toLocaleString();
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const result = await api<{ devices: Device[] }>('/api/v1/devices');
      devices = result.devices;
    } catch (e) {
      if (!getToken()) { goto('/login'); return; }
      error = e instanceof Error ? e.message : 'Unable to load devices.';
    } finally {
      loading = false;
    }
  }

  async function revokeDevice(device: Device) {
    if (device.status === 'revoked' || device.revoked_at) return;
    if (!window.confirm(`Revoke "${device.name}"? The device credential will stop working and the desktop agent will need to be registered again.`)) return;

    working = device.id;
    error = '';
    success = '';
    try {
      await api(`/api/v1/devices/${device.id}`, { method: 'DELETE' });
      await load();
      success = `${device.name} was revoked.`;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to revoke device.';
    } finally {
      working = '';
    }
  }

  async function loadPairingPreview() {
    if (!pairCode) return;
    pairingBusy = true;
    error = '';
    try {
      pairPreview = await api<PairingPreview>('/api/v1/device-pairings/preview?code=' + encodeURIComponent(pairCode));
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to load device pairing.';
      pairPreview = null;
    } finally {
      pairingBusy = false;
    }
  }

  async function approvePairing() {
    if (!pairCode || !pairPreview || pairPreview.approved) return;
    pairingBusy = true;
    error = '';
    success = '';
    try {
      pairPreview = await api<PairingPreview>('/api/v1/device-pairings/approve', {
        method: 'POST',
        body: JSON.stringify({ user_code: pairCode })
      });
      success = pairPreview.name + ' approved. The desktop agent is completing registration.';
      history.replaceState({}, '', '/devices');
      setTimeout(() => load(), 2500);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to approve device pairing.';
    } finally {
      pairingBusy = false;
    }
  }

  onMount(() => {
    pairCode = new URLSearchParams(window.location.search).get('pair') || '';
    if (!getToken()) {
      const next = window.location.pathname + window.location.search;
      goto('/login?next=' + encodeURIComponent(next));
      return;
    }
    load();
    if (pairCode) loadPairingPreview();
  });
</script>

<svelte:head><title>Devices · ClusterStor</title></svelte:head>

<AppShell>
  <div class="topbar">
    <div>
      <div class="eyebrow">Desktop agents</div>
      <h1>Devices</h1>
      <p class="muted" style="margin:.4rem 0 0">Manage computers authorized to use the ClusterStor desktop agent.</p>
    </div>
    <div class="device-summary-counts">
      <div><strong>{activeDevices.length}</strong><span>active</span></div>
      <div><strong>{peerDevices.length}</strong><span>peer</span></div>
    </div>
  </div>

  {#if error}<div class="error" style="margin-bottom:16px">{error}</div>{/if}
  {#if success}<div class="success" style="margin-bottom:16px">{success}</div>{/if}

  {#if pairCode}
    <section class="device-pair-card">
      <div>
        <div class="eyebrow">Desktop pairing</div>
        {#if pairingBusy && !pairPreview}
          <strong>Checking pairing code {pairCode}…</strong>
        {:else if pairPreview}
          <strong>Approve {pairPreview.name}?</strong>
          <span>{platformLabel(pairPreview.platform)}{pairPreview.agent_version ? ' · Agent ' + pairPreview.agent_version : ''}</span>
          <span>Peer Storage: {pairPreview.peer_contribution_enabled ? 'ON · ' + formatBytes(pairPreview.peer_contribution_bytes) : 'OFF'}</span>
        {:else}
          <strong>Pairing code {pairCode}</strong>
          <span>Unable to load pairing details.</span>
        {/if}
      </div>
      {#if pairPreview && !pairPreview.approved}
        <button class="btn primary" on:click={approvePairing} disabled={pairingBusy}>{pairingBusy ? 'Approving…' : 'Approve device'}</button>
      {:else if pairPreview?.approved}
        <span class="device-status active">Approved</span>
      {/if}
    </section>
  {/if}

  <section class="device-agent-banner">
    <div>
      <div class="eyebrow">Desktop agent</div>
      <strong>Windows desktop agent foundation is active.</strong>
      <span>The agent pairs securely, mounts the ClusterStor drive, reports device presence, and keeps Peer Storage opt-in. File synchronization is the next desktop implementation phase.</span>
    </div>
    <span class="device-status active">Developer preview</span>
  </section>

  {#if loading}
    <section class="card"><LoadingState label="Loading devices…" /></section>
  {:else if devices.length === 0}
    <section class="card device-empty-state">
      <span class="device-empty-mark">PC</span>
      <div>
        <strong>No devices registered yet.</strong>
        <p>Your computers will appear here after the ClusterStor desktop agent is installed and registered.</p>
      </div>
    </section>
  {:else}
    <section class="card device-table-card">
      <div class="device-table-head">
        <div>
          <div class="eyebrow">Authorized computers</div>
          <h2>Registered devices</h2>
        </div>
        <button class="btn ghost" on:click={load} disabled={loading || !!working}>Refresh</button>
      </div>

      <div class="device-table-wrap">
        <table class="device-table">
          <thead>
            <tr>
              <th>Device</th>
              <th>Platform</th>
              <th>Agent</th>
              <th>Last seen</th>
              <th>Peer Storage</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each devices as device}
              <tr class:device-revoked={device.status === 'revoked' || !!device.revoked_at}>
                <td>
                  <div class="device-name-cell">
                    <span class="device-platform-mark">{platformMark(device.platform)}</span>
                    <div>
                      <strong>{device.name}</strong>
                      <span>Registered {new Date(device.created_at).toLocaleDateString()}</span>
                    </div>
                  </div>
                </td>
                <td>{platformLabel(device.platform)}</td>
                <td>{device.agent_version || '—'}</td>
                <td>{seenLabel(device.last_seen_at)}</td>
                <td>
                  {#if device.peer_contribution_enabled}
                    <span class="device-peer enabled">On · {formatBytes(device.peer_contribution_bytes)}</span>
                  {:else}
                    <span class="device-peer">Off</span>
                  {/if}
                </td>
                <td>
                  {#if device.status === 'revoked' || device.revoked_at}
                    <span class="device-status revoked">Revoked</span>
                  {:else}
                    <span class="device-status active">Active</span>
                  {/if}
                </td>
                <td class="device-actions">
                  {#if device.status !== 'revoked' && !device.revoked_at}
                    <button class="btn danger" on:click={() => revokeDevice(device)} disabled={working === device.id}>
                      {working === device.id ? 'Revoking…' : 'Revoke'}
                    </button>
                  {:else}
                    <span class="muted">—</span>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>
  {/if}

  {#if revokedDevices.length > 0}
    <section class="device-security-note">
      <div class="eyebrow">Credential security</div>
      <strong>Revoked devices cannot authenticate with their previous device secret.</strong>
      <span>Re-enabling a revoked computer will require a new registration from the desktop agent.</span>
    </section>
  {/if}
</AppShell>
