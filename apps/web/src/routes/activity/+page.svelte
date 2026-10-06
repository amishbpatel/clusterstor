<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import AppShell from '$lib/AppShell.svelte';
  import LoadingState from '$lib/LoadingState.svelte';
  import { api, getToken, formatBytes, openEventSocket } from '$lib/api';

  type ActivityItem = {
    id: string;
    sequence: number;
    event_type: string;
    resource_type?: string | null;
    resource_id?: string | null;
    name: string;
    node_type?: string;
    provider?: string;
    size_bytes?: number;
    created_at: string;
    payload?: Record<string, unknown>;
  };

  type ActivityPage = {
    items: ActivityItem[];
    next_cursor?: number;
    has_more: boolean;
  };

  let items: ActivityItem[] = [];
  let loading = true;
  let loadingMore = false;
  let error = '';
  let hasMore = false;
  let nextCursor = 0;

  let search = '';
  let eventType = '';
  let provider = '';
  let fromDate = '';
  let toDate = '';

  const eventOptions = [
    ['', 'All activity'],
    ['file.version.created', 'Uploads / new versions'],
    ['file.version.restored', 'Version restores'],
    ['file.downloaded', 'Downloads'],
    ['folder.created', 'Folder creation'],
    ['node.renamed', 'Renames'],
    ['node.moved', 'Moves'],
    ['node.deleted', 'Deletes'],
    ['node.restored', 'Trash restores']
  ];

  const providerOptions = [
    ['', 'All providers'],
    ['google_drive', 'Google Drive'],
    ['onedrive', 'OneDrive'],
    ['dropbox', 'Dropbox'],
    ['box', 'Box'],
    ['clusterstor', 'ClusterStor']
  ];

  function sourceLabel(value?: string) {
    if (value === 'google_drive') return 'Google Drive';
    if (value === 'onedrive') return 'OneDrive';
    if (value === 'dropbox') return 'Dropbox';
    if (value === 'box') return 'Box';
    if (value === 'clusterstor') return 'ClusterStor';
    return value || '—';
  }

  function activityLabel(type: string) {
    switch (type) {
      case 'file.version.created': return 'New version';
      case 'file.version.restored': return 'Version restored';
      case 'file.version.baseline': return 'Version history started';
      case 'file.downloaded': return 'Downloaded';
      case 'folder.created': return 'Folder created';
      case 'node.renamed': return 'Renamed';
      case 'node.moved': return 'Moved';
      case 'node.deleted': return 'Deleted';
      case 'node.restored': return 'Restored';
      default: return type.replaceAll('.', ' ');
    }
  }

  function activityDetail(item: ActivityItem) {
    const payload = item.payload || {};
    const count = Number(payload.deleted_count || payload.restored_count || 0);
    const destination = typeof payload.destination === 'string' ? payload.destination : '';

    switch (item.event_type) {
      case 'file.version.created':
        return 'A new file version became current.';
      case 'file.version.restored':
        return 'An older version was restored as a new current version.';
      case 'file.version.baseline':
        return 'The current provider revision was captured as the first tracked version.';
      case 'file.downloaded':
        return destination ? `Downloaded to ${destination}.` : 'File contents were downloaded.';
      case 'folder.created':
        return 'A new folder was created.';
      case 'node.renamed':
        return 'The item name was changed.';
      case 'node.moved':
        return 'The item was moved to another folder.';
      case 'node.deleted':
        return count > 1 ? `${count} items were moved to trash.` : 'The item was moved to trash.';
      case 'node.restored':
        return count > 1 ? `${count} items were restored from trash.` : 'The item was restored from trash.';
      default:
        return '';
    }
  }

  function localDateBoundary(date: string, end = false) {
    if (!date) return '';
    const [year, month, day] = date.split('-').map(Number);
    const value = new Date(year, month - 1, day + (end ? 1 : 0), 0, 0, 0, 0);
    return value.toISOString();
  }

  function buildParams(before = 0) {
    const params = new URLSearchParams({ limit: '50' });
    if (before > 0) params.set('before', String(before));
    if (search.trim()) params.set('q', search.trim());
    if (eventType) params.set('event_type', eventType);
    if (provider) params.set('provider', provider);
    const from = localDateBoundary(fromDate);
    const to = localDateBoundary(toDate, true);
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    return params;
  }

  async function load(reset = true) {
    if (reset) loading = true;
    else loadingMore = true;
    error = '';

    try {
      const result = await api<ActivityPage>(`/api/v1/activity?${buildParams(reset ? 0 : nextCursor).toString()}`);
      items = reset ? result.items : [...items, ...result.items];
      hasMore = result.has_more;
      nextCursor = result.next_cursor || 0;
    } catch (e) {
      if (!getToken()) { goto('/login'); return; }
      error = e instanceof Error ? e.message : 'Unable to load activity.';
    } finally {
      loading = false;
      loadingMore = false;
    }
  }

  function clearFilters() {
    search = '';
    eventType = '';
    provider = '';
    fromDate = '';
    toDate = '';
    load(true);
  }

  onMount(() => {
    if (!getToken()) { goto('/login'); return; }
    load(true);

    let socket: WebSocket | null = null;
    let closed = false;
    openEventSocket(() => {
      if (!loading && !loadingMore) load(true);
    }).then((value) => {
      if (closed) value.close();
      else socket = value;
    }).catch(() => {});

    return () => {
      closed = true;
      socket?.close();
    };
  });
</script>

<svelte:head><title>Activity · ClusterStor</title></svelte:head>

<AppShell>
  <div class="topbar">
    <div>
      <div class="eyebrow">Account history</div>
      <h1>Activity</h1>
      <p class="muted" style="margin:.4rem 0 0">Review file operations and version activity across your connected storage.</p>
    </div>
    <button class="btn ghost" on:click={() => load(true)} disabled={loading || loadingMore}>Refresh</button>
  </div>

  {#if error}<div class="error" style="margin-bottom:16px">{error}</div>{/if}

  <section class="card activity-filter-card">
    <form class="activity-filters" on:submit|preventDefault={() => load(true)}>
      <label class="activity-search">
        <span>Search</span>
        <input bind:value={search} type="search" placeholder="File or folder name" />
      </label>
      <label>
        <span>Activity type</span>
        <select bind:value={eventType}>
          {#each eventOptions as option}
            <option value={option[0]}>{option[1]}</option>
          {/each}
        </select>
      </label>
      <label>
        <span>Provider</span>
        <select bind:value={provider}>
          {#each providerOptions as option}
            <option value={option[0]}>{option[1]}</option>
          {/each}
        </select>
      </label>
      <label>
        <span>From</span>
        <input bind:value={fromDate} type="date" />
      </label>
      <label>
        <span>To</span>
        <input bind:value={toDate} type="date" />
      </label>
      <div class="activity-filter-actions">
        <button class="btn primary" type="submit" disabled={loading || loadingMore}>Apply</button>
        <button class="btn ghost" type="button" on:click={clearFilters} disabled={loading || loadingMore}>Clear</button>
      </div>
    </form>
  </section>

  <section class="card activity-feed-card">
    <div class="activity-feed-head">
      <div>
        <div class="eyebrow">Newest first</div>
        <h2>Account activity</h2>
      </div>
      <span class="muted">{items.length} shown</span>
    </div>

    {#if loading}
      <LoadingState label="Loading activity…" />
    {:else if items.length === 0}
      <div class="activity-empty">No activity matches these filters.</div>
    {:else}
      <div class="activity-feed">
        {#each items as item}
          <article class="activity-feed-row">
            <div class="activity-when">
              <strong>{new Date(item.created_at).toLocaleDateString()}</strong>
              <span>{new Date(item.created_at).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}</span>
            </div>
            <div class="activity-main">
              <div class="activity-title-line">
                <span class="activity-type-pill">{activityLabel(item.event_type)}</span>
                <strong>{item.name}</strong>
              </div>
              <p>{activityDetail(item)}</p>
            </div>
            <div class="activity-source">
              <span class="source-pill">{sourceLabel(item.provider)}</span>
              {#if item.size_bytes && item.size_bytes > 0}<span>{formatBytes(item.size_bytes)}</span>{/if}
            </div>
          </article>
        {/each}
      </div>

      {#if hasMore}
        <div class="activity-load-more">
          <button class="btn" on:click={() => load(false)} disabled={loadingMore}>
            {loadingMore ? 'Loading…' : 'Load more'}
          </button>
        </div>
      {/if}
    {/if}
  </section>
</AppShell>
