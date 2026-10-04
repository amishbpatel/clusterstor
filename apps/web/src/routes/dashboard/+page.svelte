<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import AppShell from '$lib/AppShell.svelte';
  import { api, getToken, formatBytes, type User, type ProviderAccount } from '$lib/api';

  let user: User | null = null;
  let providers: ProviderAccount[] = [];
  let error = '';
  let loading = true;
  let connecting = false;

  $: google = providers.find((p) => p.provider === 'google_drive');

  async function load() {
    error = '';
    try {
      const results = await Promise.all([
        api<User>('/api/v1/me'),
        api<{ providers: ProviderAccount[] }>('/api/v1/providers')
      ]);
      user = results[0];
      providers = results[1].providers;
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
    <section class="card">Loading your ClusterStor account…</section>
  {:else}
    <section class="grid">
      <div class="card stat">
        <span class="muted">Connected providers</span>
        <strong>{providers.length}</strong>
      </div>
      <div class="card stat">
        <span class="muted">Google Drive used</span>
        <strong>{formatBytes(google?.quota_used_bytes)}</strong>
      </div>
      <div class="card stat">
        <span class="muted">Google Drive free</span>
        <strong>{formatBytes(google?.quota_free_bytes)}</strong>
      </div>
    </section>

    <section class="card" style="margin-top:16px">
      <div class="toolbar">
        <div>
          <div class="eyebrow">Storage providers</div>
          <h2 style="margin:.35rem 0 0">Google Drive</h2>
        </div>
        {#if google}
          <span class="pill">● Connected</span>
        {:else}
          <button class="btn primary" on:click={connectGoogle} disabled={connecting}>
            {connecting ? 'Connecting…' : 'Connect Google Drive'}
          </button>
        {/if}
      </div>
      {#if google}
        <p class="muted">{google.display_name || 'Google Drive'} · {formatBytes(google.quota_used_bytes)} used of {formatBytes(google.quota_total_bytes)}</p>
        <p class="muted">ClusterStor-created files and folders are kept inside your dedicated <strong>ClusterStor</strong> folder in Google Drive.</p>
      {:else}
        <p class="muted">Connect Google Drive to browse files and begin using ClusterStor.</p>
      {/if}
    </section>
  {/if}
</AppShell>
