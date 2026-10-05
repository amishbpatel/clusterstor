<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import AppShell from '$lib/AppShell.svelte';
  import { api, API_BASE, getToken, formatBytes, openEventSocket, type DriveItem } from '$lib/api';

  let items: DriveItem[] = [];
  let rootProviderID = '';
  let currentFolder: DriveItem | null = null;
  let error = '';
  let success = '';
  let loading = true;
  let working = false;
  let fileInput: HTMLInputElement;

  $: currentProviderParent = currentFolder ? currentFolder.provider_item_id : rootProviderID;
  $: visibleItems = items
    .filter((item) => item.parent_item_id === currentProviderParent)
    .sort((a, b) => {
      if (a.node_type !== b.node_type) return a.node_type === 'folder' ? -1 : 1;
      return a.name.localeCompare(b.name);
    });

  async function load() {
    error = '';
    try {
      const root = await api<{ root_provider_item_id: string }>('/api/v1/providers/google_drive/root', { method: 'POST' });
      rootProviderID = root.root_provider_item_id;
      const allItems: DriveItem[] = [];
      let pageToken = '';
      do {
        const params = new URLSearchParams({ page_size: '500' });
        if (pageToken) params.set('cursor', pageToken);
        const page = await api<{ items: DriveItem[]; next_page_token?: string }>(
          `/api/v1/providers/google_drive/files?${params.toString()}`
        );
        allItems.push(...page.items);
        pageToken = page.next_page_token || '';
      } while (pageToken);
      items = allItems;
    } catch (e) {
      if (!getToken()) { goto('/login'); return; }
      error = e instanceof Error ? e.message : 'Unable to load files.';
    } finally {
      loading = false;
    }
  }

  function openFolder(item: DriveItem) {
    currentFolder = item;
  }

  function goRoot() { currentFolder = null; }

  async function createFolder() {
    const name = window.prompt('Folder name');
    if (!name?.trim()) return;
    working = true;
    error = ''; success = '';
    try {
      const item = await api<DriveItem>('/api/v1/providers/google_drive/folders', {
        method: 'POST',
        body: JSON.stringify({ name: name.trim(), parent_node_id: currentFolder?.node_id || null })
      });
      items = [...items, item];
      success = 'Folder created.';
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to create folder.';
    } finally { working = false; }
  }

  function chooseUpload() { fileInput?.click(); }

  async function uploadSelected(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    working = true;
    error = ''; success = '';
    try {
      const session = await api<{ upload_url: string }>('/api/v1/providers/google_drive/uploads', {
        method: 'POST',
        body: JSON.stringify({
          name: file.name,
          content_type: file.type || 'application/octet-stream',
          size_bytes: file.size,
          parent_node_id: currentFolder?.node_id || null
        })
      });

      const uploaded = await fetch(session.upload_url, {
        method: 'PUT',
        headers: {
          'Content-Type': file.type || 'application/octet-stream'
        },
        body: file
      });
      if (!uploaded.ok) throw new Error('Google Drive upload failed.');
      const googleFile = await uploaded.json();
      if (!googleFile.id) throw new Error('Google Drive did not return a file ID.');

      await api('/api/v1/providers/google_drive/uploads/complete', {
        method: 'POST',
        body: JSON.stringify({ provider_item_id: googleFile.id })
      });
      success = 'Upload complete.';
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to upload file.';
    } finally {
      working = false;
      input.value = '';
    }
  }

  async function download(item: DriveItem) {
    const token = getToken();
    if (!token) { goto('/login'); return; }
    error = '';
    try {
      const response = await fetch(`${API_BASE}/api/v1/nodes/${item.node_id}/download`, {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (!response.ok) throw new Error('Unable to download file.');
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = item.name;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to download file.';
    }
  }

  onMount(() => {
    if (!getToken()) { goto('/login'); return; }
    load();

    let socket: WebSocket | null = null;
    let closed = false;
    openEventSocket(() => {
      if (!working) load();
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

<svelte:head><title>My Files · ClusterStor</title></svelte:head>
<AppShell>
  <div class="topbar">
    <div>
      <div class="eyebrow">Google Drive</div>
      <h1>My Files</h1>
      <p class="muted" style="margin:.4rem 0 0">ClusterStor-managed files live inside your dedicated Google Drive folder.</p>
    </div>
    <div class="actions">
      <button class="btn" on:click={createFolder} disabled={working}>New folder</button>
      <button class="btn primary" on:click={chooseUpload} disabled={working}>Upload file</button>
      <input bind:this={fileInput} type="file" style="display:none" on:change={uploadSelected} />
    </div>
  </div>

  {#if error}<div class="error" style="margin-bottom:16px">{error}</div>{/if}
  {#if success}<div class="success" style="margin-bottom:16px">{success}</div>{/if}

  <section class="card">
    <div class="toolbar">
      <div class="actions">
        <button class="btn ghost" on:click={goRoot} disabled={!currentFolder}>ClusterStor</button>
        {#if currentFolder}<span class="muted">/</span><span>{currentFolder.name}</span>{/if}
      </div>
      <button class="btn ghost" on:click={load} disabled={loading || working}>Refresh</button>
    </div>

    {#if loading}
      <div class="empty">Loading files…</div>
    {:else if visibleItems.length === 0}
      <div class="empty">
        <strong>This folder is empty.</strong>
        <p>Upload a file or create a folder to get started.</p>
      </div>
    {:else}
      <div style="overflow:auto">
        <table class="table">
          <thead><tr><th>Name</th><th>Type</th><th>Size</th><th>Modified</th><th></th></tr></thead>
          <tbody>
            {#each visibleItems as item}
              <tr>
                <td>
                  {#if item.node_type === 'folder'}
                    <button class="btn ghost" style="padding:4px 0" on:click={() => openFolder(item)}>📁 {item.name}</button>
                  {:else}
                    📄 {item.name}
                  {/if}
                </td>
                <td>{item.node_type}</td>
                <td>{item.node_type === 'folder' ? '—' : formatBytes(item.size_bytes)}</td>
                <td>{item.modified_at ? new Date(item.modified_at).toLocaleString() : '—'}</td>
                <td>
                  {#if item.node_type === 'file'}
                    <button class="btn" on:click={() => download(item)}>Download</button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
</AppShell>
