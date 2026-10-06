<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import AppShell from '$lib/AppShell.svelte';
  import LoadingState from '$lib/LoadingState.svelte';
  import { api, getToken, formatBytes, type ProviderAccount } from '$lib/api';

  type ProviderDefinition = {
    id: string;
    name: string;
    short: string;
    available: boolean;
    description: string;
  };

  const providerDefinitions: ProviderDefinition[] = [
    { id: 'google_drive', name: 'Google Drive', short: 'G', available: true, description: 'Use your dedicated ClusterStor folder in Google Drive.' },
    { id: 'onedrive', name: 'OneDrive', short: 'O', available: false, description: 'Microsoft cloud storage adapter.' },
    { id: 'dropbox', name: 'Dropbox', short: 'D', available: false, description: 'Dropbox cloud storage adapter.' },
    { id: 'box', name: 'Box', short: 'B', available: false, description: 'Box cloud storage adapter.' }
  ];

  let accounts: ProviderAccount[] = [];
  let loading = true;
  let working = '';
  let error = '';
  let success = '';

  $: connectedCount = accounts.filter((account) => account.status === 'connected').length;

  function accountFor(provider: string) {
    return accounts.find((account) => account.provider === provider);
  }

  function usedPercent(account?: ProviderAccount) {
    const total = account?.quota_total_bytes || 0;
    const used = account?.quota_used_bytes || 0;
    if (total <= 0) return 0;
    return Math.max(0, Math.min(100, (used / total) * 100));
  }

  function syncLabel(value?: string | null) {
    if (!value) return 'Not synced yet';
    return new Date(value).toLocaleString();
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const result = await api<{ providers: ProviderAccount[] }>('/api/v1/providers');
      accounts = result.providers;
    } catch (e) {
      if (!getToken()) { goto('/login'); return; }
      error = e instanceof Error ? e.message : 'Unable to load providers.';
    } finally {
      loading = false;
    }
  }

  async function connectGoogle() {
    working = 'google_drive';
    error = '';
    success = '';
    try {
      const result = await api<{ authorization_url: string }>('/api/v1/providers/google_drive/oauth/start', { method: 'POST' });
      window.location.href = result.authorization_url;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to connect Google Drive.';
      working = '';
    }
  }

  async function refreshGoogle() {
    working = 'google_drive';
    error = '';
    success = '';
    try {
      await api('/api/v1/providers/google_drive/root', { method: 'POST' });
      await api('/api/v1/uploads/preflight', {
        method: 'POST',
        body: JSON.stringify({ provider: 'google_drive', size_bytes: 0 })
      });
      try {
        await api('/api/v1/providers/google_drive/changes', { method: 'POST' });
      } catch {
        // Root and quota refresh are still useful if incremental sync is temporarily unavailable.
      }
      await load();
      success = 'Google Drive connection refreshed.';
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to refresh Google Drive.';
    } finally {
      working = '';
    }
  }

  async function disconnectGoogle() {
    const account = accountFor('google_drive');
    if (!account || account.status !== 'connected') return;
    if (!window.confirm('Disconnect Google Drive from ClusterStor? Your files will remain in Google Drive and will not be deleted.')) return;

    working = 'google_drive';
    error = '';
    success = '';
    try {
      await api('/api/v1/providers/google_drive', { method: 'DELETE' });
      await load();
      success = 'Google Drive disconnected. Files remain in Google Drive.';
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to disconnect Google Drive.';
    } finally {
      working = '';
    }
  }

  onMount(async () => {
    if (!getToken()) { goto('/login'); return; }
    await load();

    const connected = new URLSearchParams(window.location.search).get('connected');
    if (connected === 'google_drive') {
      success = 'Google Drive connected.';
      history.replaceState({}, '', '/providers');
      await refreshGoogle();
    }
  });
</script>

<svelte:head><title>Providers · ClusterStor</title></svelte:head>

<AppShell>
  <div class="topbar">
    <div>
      <div class="eyebrow">Storage connections</div>
      <h1>Providers</h1>
      <p class="muted" style="margin:.4rem 0 0">Connect and manage the cloud storage services behind your ClusterStor files.</p>
    </div>
    <div class="provider-count">
      <strong>{connectedCount}</strong>
      <span>connected</span>
    </div>
  </div>

  {#if error}<div class="error" style="margin-bottom:16px">{error}</div>{/if}
  {#if success}<div class="success" style="margin-bottom:16px">{success}</div>{/if}

  <section class="provider-plan-strip">
    <div>
      <span class="eyebrow">Provider plans</span>
      <strong>Free</strong>
      <span>Up to 2 connected cloud providers</span>
    </div>
    <div>
      <span class="eyebrow">Unified</span>
      <strong>$2/mo <small>or $15/yr</small></strong>
      <span>All 4 supported cloud providers</span>
    </div>
  </section>

  {#if loading}
    <section class="card"><LoadingState label="Loading provider connections…" /></section>
  {:else}
    <section class="provider-management-grid">
      {#each providerDefinitions as provider}
        {@const account = accountFor(provider.id)}
        {@const connected = account?.status === 'connected'}
        <article class:provider-connected={connected} class="card provider-management-card">
          <div class="provider-management-head">
            <div class="provider-identity">
              <span class="provider-mark">{provider.short}</span>
              <div>
                <h2>{provider.name}</h2>
                <p>{provider.description}</p>
              </div>
            </div>
            {#if connected}
              <span class="provider-status connected">Connected</span>
            {:else if provider.available}
              <span class="provider-status">Not connected</span>
            {:else}
              <span class="provider-status coming">Coming soon</span>
            {/if}
          </div>

          {#if provider.id === 'google_drive' && connected && account}
            <div class="provider-account-grid">
              <div><span>Account</span><strong>{account.display_name || 'Google Drive account'}</strong></div>
              <div><span>Last sync</span><strong>{syncLabel(account.last_synced_at)}</strong></div>
              <div><span>ClusterStor folder</span><strong>{account.managed_root_ready ? 'Ready' : 'Not initialized'}</strong></div>
              <div><span>Status</span><strong>{account.status}</strong></div>
            </div>

            {#if account.quota_total_bytes}
              <div class="provider-quota">
                <div class="provider-quota-head">
                  <span>Storage</span>
                  <strong>{formatBytes(account.quota_used_bytes)} of {formatBytes(account.quota_total_bytes)}</strong>
                </div>
                <div class="capacity-track">
                  <span class="capacity-fill" style={'width:' + usedPercent(account) + '%'}></span>
                </div>
                <div class="provider-quota-foot">
                  <span>{usedPercent(account).toFixed(1)}% used</span>
                  <span>{formatBytes(account.quota_free_bytes)} free</span>
                </div>
              </div>
            {/if}

            <div class="provider-management-actions">
              <button class="btn" on:click={refreshGoogle} disabled={working === provider.id}>{working === provider.id ? 'Working…' : 'Refresh'}</button>
              <button class="btn ghost" on:click={connectGoogle} disabled={working === provider.id}>Reconnect</button>
              <button class="btn danger" on:click={disconnectGoogle} disabled={working === provider.id}>Disconnect</button>
            </div>
          {:else if provider.id === 'google_drive'}
            {#if account?.status === 'disconnected'}
              <div class="provider-disconnected-note">
                <strong>Previously connected</strong>
                <span>Your Google Drive files were left untouched. Reconnect to resume ClusterStor access.</span>
              </div>
            {/if}
            <div class="provider-management-actions">
              <button class="btn primary" on:click={connectGoogle} disabled={working === provider.id}>{working === provider.id ? 'Connecting…' : 'Connect Google Drive'}</button>
            </div>
          {:else}
            <div class="provider-coming-copy">
              <strong>Adapter planned</strong>
              <span>This provider will become connectable when its ClusterStor adapter is implemented.</span>
            </div>
            <div class="provider-management-actions">
              <button class="btn ghost" disabled>Connect</button>
            </div>
          {/if}
        </article>
      {/each}
    </section>
  {/if}

  <section class="card provider-safety-note">
    <div class="eyebrow">Connection safety</div>
    <strong>Disconnecting a provider does not delete provider files.</strong>
    <p>ClusterStor removes its stored connection credential and stops provider access. Your files remain in the provider account unless you delete them separately.</p>
  </section>
</AppShell>
