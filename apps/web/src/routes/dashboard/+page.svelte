<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import AppShell from '$lib/AppShell.svelte';
  import LoadingState from '$lib/LoadingState.svelte';
  import { api, getToken, formatBytes, type User, type ProviderAccount } from '$lib/api';

  let user: User | null = null;
  let providers: ProviderAccount[] = [];
  let error = '';
  let loading = true;
  let connecting = false;
  type RecentActivity = {
    event_type: string;
    node_id: string;
    name: string;
    provider: string;
    size_bytes: number;
    destination?: string | null;
    created_at: string;
  };
  let recentUploads: RecentActivity[] = [];
  type FileTypeStat = {
    file_type: string;
    file_count: number;
    total_size_bytes: number;
  };
  type LargestFile = {
    node_id: string;
    name: string;
    provider: string;
    size_bytes: number;
  };
  let fileTypes: FileTypeStat[] = [];
  let largestFiles: LargestFile[] = [];

  $: google = providers.find((p) => p.provider === 'google_drive');
  $: storageProviders = providers.filter((p) => (p.quota_total_bytes || 0) > 0);
  $: aggregateTotal = storageProviders.reduce((sum, p) => sum + (p.quota_total_bytes || 0), 0);
  $: aggregateUsed = storageProviders.reduce((sum, p) => sum + (p.quota_used_bytes || 0), 0);
  $: aggregateFree = Math.max(aggregateTotal - aggregateUsed, 0);

  function providerLabel(provider: string) {
    if (provider === 'google_drive') return 'Google Drive';
    if (provider === 'onedrive') return 'OneDrive';
    if (provider === 'dropbox') return 'Dropbox';
    if (provider === 'box') return 'Box';
    if (provider === 'clusterstor') return 'ClusterStor';
    return provider;
  }

  function usedPercent(provider: ProviderAccount) {
    const total = provider.quota_total_bytes || 0;
    const used = provider.quota_used_bytes || 0;
    if (total <= 0) return 0;
    return Math.max(0, Math.min(100, Math.round((used / total) * 100)));
  }

  function totalUsedPercent() {
    if (aggregateTotal <= 0) return 0;
    return Math.max(0, Math.min(100, Math.round((aggregateUsed / aggregateTotal) * 100)));
  }

  function formatActivityTime(value: string) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString();
  }

  async function load() {
    error = '';
    try {
      const results = await Promise.all([
        api<User>('/api/v1/me'),
        api<{ providers: ProviderAccount[] }>('/api/v1/providers'),
        api<{ uploads: RecentActivity[] }>('/api/v1/dashboard/recent'),
        api<{ file_types: FileTypeStat[]; largest_files: LargestFile[] }>('/api/v1/dashboard/files')
      ]);
      user = results[0];
      providers = results[1].providers;
      recentUploads = results[2].uploads;
      fileTypes = results[3].file_types;
      largestFiles = results[3].largest_files;
    } catch (e) {
      if (!getToken()) {
        goto('/login');
        return;
      }
      error = e instanceof Error ? e.message : 'Unable to load dashboard.';
    } finally {
      loading = false;
    }
  }

  async function connectGoogle() {
    connecting = true;
    error = '';
    try {
      const result = await api<{ authorization_url: string }>('/api/v1/providers/google_drive/oauth/start', { method: 'POST' });
      window.location.href = result.authorization_url;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to connect Google Drive.';
      connecting = false;
    }
  }

  onMount(() => {
    if (!getToken()) {
      goto('/login');
      return;
    }
    load();
  });
</script>

<svelte:head><title>Dashboard · ClusterStor</title></svelte:head>
<AppShell>
  <div class="topbar">
    <div>
      <div class="eyebrow">Overview</div>
      <h1>{user?.display_name ? 'Welcome, ' + user.display_name : 'Dashboard'}</h1>
    </div>
    <a class="btn primary" href="/files">Open My Files</a>
  </div>

  {#if error}<div class="error" style="margin-bottom:16px">{error}</div>{/if}

  {#if loading}
    <section class="card"><LoadingState label="Loading your ClusterStor dashboard…" /></section>
  {:else}
    <section class="dashboard-summary">
      <div class="card stat compact-stat">
        <span class="muted">Connected providers</span>
        <strong>{providers.length}</strong>
      </div>

      <section class="card storage-overview">
        <div class="storage-overview-head">
          <div>
            <div class="eyebrow">Storage overview</div>
            <h2>Capacity across providers</h2>
          </div>
          {#if aggregateTotal > 0}
            <div class="storage-total-copy">
              <strong>{formatBytes(aggregateUsed)}</strong>
              <span>of {formatBytes(aggregateTotal)} used</span>
            </div>
          {/if}
        </div>

        {#if aggregateTotal > 0}
          <div class="capacity-block aggregate-capacity">
            <div class="capacity-meta">
              <span>All connected storage</span>
              <strong>{totalUsedPercent()}%</strong>
            </div>
            <div class="capacity-track" aria-label={totalUsedPercent() + "% of connected storage used"}>
              <span class="capacity-fill" style={"width:" + totalUsedPercent() + "%"}></span>
            </div>
            <div class="capacity-foot">
              <span>{formatBytes(aggregateUsed)} used</span>
              <span>{formatBytes(aggregateFree)} free</span>
            </div>
          </div>
        {:else}
          <p class="muted">Connect a storage provider to see capacity and usage here.</p>
        {/if}

        <div class="provider-capacity-list">
          {#each storageProviders as provider}
            <article class="provider-capacity-row">
              <div class="provider-capacity-head">
                <div>
                  <strong>{providerLabel(provider.provider)}</strong>
                  {#if provider.display_name}<span>{provider.display_name}</span>{/if}
                </div>
                <span class="pill">● Connected</span>
              </div>
              <div class="capacity-block">
                <div class="capacity-meta">
                  <span>{formatBytes(provider.quota_used_bytes)} of {formatBytes(provider.quota_total_bytes)}</span>
                  <strong>{usedPercent(provider)}%</strong>
                </div>
                <div class="capacity-track" aria-label={usedPercent(provider) + "% of " + providerLabel(provider.provider) + " storage used"}>
                  <span class="capacity-fill" style={"width:" + usedPercent(provider) + "%"}></span>
                </div>
                <div class="capacity-foot">
                  <span>{formatBytes(provider.quota_used_bytes)} used</span>
                  <span>{formatBytes(provider.quota_free_bytes)} free</span>
                </div>
              </div>
            </article>
          {/each}
        </div>
      </section>
    </section>

    <section class="dashboard-widget-grid">
      <section class="card activity-card">
        <div class="activity-card-head">
          <div>
            <div class="eyebrow">File mix</div>
            <h2>Files by type</h2>
          </div>
        </div>
        {#if fileTypes.length === 0}
          <div class="activity-empty">No file statistics available yet.</div>
        {:else}
          <div class="activity-table-wrap">
            <table class="activity-table">
              <thead><tr><th>Type</th><th>Files</th><th>Total size</th></tr></thead>
              <tbody>
                {#each fileTypes as item}
                  <tr>
                    <td><span class="file-type-badge">{item.file_type}</span></td>
                    <td>{item.file_count.toLocaleString()}</td>
                    <td>{formatBytes(item.total_size_bytes)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </section>

      <section class="card activity-card">
        <div class="activity-card-head">
          <div>
            <div class="eyebrow">Storage detail</div>
            <h2>Largest files</h2>
          </div>
          <a class="btn ghost" href="/files">View files</a>
        </div>
        {#if largestFiles.length === 0}
          <div class="activity-empty">No files available yet.</div>
        {:else}
          <div class="activity-table-wrap">
            <table class="activity-table">
              <thead><tr><th>File</th><th>Size</th><th>Source</th></tr></thead>
              <tbody>
                {#each largestFiles as item}
                  <tr>
                    <td><strong>{item.name}</strong></td>
                    <td>{formatBytes(item.size_bytes)}</td>
                    <td>{providerLabel(item.provider)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </section>

      <section class="card activity-card recent-upload-card">
        <div class="activity-card-head">
          <div>
            <div class="eyebrow">Recent activity</div>
            <h2>Recently uploaded</h2>
          </div>
          <a class="btn ghost" href="/files">View files</a>
        </div>
        {#if recentUploads.length === 0}
          <div class="activity-empty">No recent uploads yet.</div>
        {:else}
          <div class="activity-table-wrap">
            <table class="activity-table">
              <thead><tr><th>File</th><th>Source</th><th>Size</th><th>Uploaded</th></tr></thead>
              <tbody>
                {#each recentUploads as item}
                  <tr>
                    <td><strong>{item.name}</strong></td>
                    <td>{providerLabel(item.provider)}</td>
                    <td>{formatBytes(item.size_bytes)}</td>
                    <td>{formatActivityTime(item.created_at)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </section>
    </section>
  {/if}
</AppShell>
