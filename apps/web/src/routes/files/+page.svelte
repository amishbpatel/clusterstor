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
  let selected = new Set<string>();

  $: currentProviderParent = currentFolder ? currentFolder.provider_item_id : rootProviderID;
  $: visibleItems = items
    .filter((item) => item.parent_item_id === currentProviderParent)
    .sort((a, b) => {
      if (a.node_type !== b.node_type) return a.node_type === 'folder' ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
  $: breadcrumb = buildBreadcrumb(currentFolder);
  $: allVisibleSelected = visibleItems.length > 0 && visibleItems.every((item) => selected.has(item.node_id));

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
    selected = new Set();
  }

  function goRoot() { currentFolder = null; selected = new Set(); }

  function buildBreadcrumb(folder: DriveItem | null) {
    if (!folder) return [] as DriveItem[];
    const path: DriveItem[] = [];
    let cursor: DriveItem | undefined = folder;
    const seen = new Set<string>();
    while (cursor && !seen.has(cursor.node_id)) {
      path.unshift(cursor);
      seen.add(cursor.node_id);
      if (!cursor.parent_item_id || cursor.parent_item_id === rootProviderID) break;
      cursor = items.find((item) => item.provider_item_id === cursor?.parent_item_id);
    }
    return path;
  }

  function toggleSelected(item: DriveItem) {
    const next = new Set(selected);
    if (next.has(item.node_id)) next.delete(item.node_id); else next.add(item.node_id);
    selected = next;
  }

  function toggleAllVisible() {
    const next = new Set(selected);
    if (allVisibleSelected) visibleItems.forEach((item) => next.delete(item.node_id));
    else visibleItems.forEach((item) => next.add(item.node_id));
    selected = next;
  }

  function collectFiles(item: DriveItem, prefix = ''): Array<{ item: DriveItem; path: string }> {
    if (item.node_type === 'file') return [{ item, path: prefix + item.name }];
    const folderPrefix = prefix + item.name + '/';
    return items
      .filter((child) => child.parent_item_id === item.provider_item_id)
      .flatMap((child) => collectFiles(child, folderPrefix));
  }

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

  async function fetchDownload(item: DriveItem) {
    const token = getToken();
    if (!token) { goto('/login'); throw new Error('You are not signed in.'); }
    const response = await fetch(`${API_BASE}/api/v1/nodes/${item.node_id}/download`, {
      headers: { Authorization: `Bearer ${token}` }
    });
    if (!response.ok) throw new Error(`Unable to download ${item.name}.`);
    return response.blob();
  }

  async function saveBlob(blob: Blob, name: string) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }

  async function download(item: DriveItem) {
    error = '';
    try {
      await saveBlob(await fetchDownload(item), item.name);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to download file.';
    }
  }

  async function downloadSelected() {
    const chosen = items.filter((item) => selected.has(item.node_id));
    if (chosen.length === 0) return;
    working = true;
    error = ''; success = '';
    try {
      const files = chosen.flatMap((item) => collectFiles(item));
      if (files.length === 0) throw new Error('The selected folder does not contain downloadable files.');
      if (chosen.length === 1 && chosen[0].node_type === 'file') {
        await saveBlob(await fetchDownload(chosen[0]), chosen[0].name);
        return;
      }
      const JSZip = (await import('jszip')).default;
      const zip = new JSZip();
      for (const entry of files) {
        zip.file(entry.path, await fetchDownload(entry.item));
      }
      const blob = await zip.generateAsync({ type: 'blob' });
      await saveBlob(blob, currentFolder ? `${currentFolder.name}-download.zip` : 'ClusterStor-download.zip');
      success = `Downloaded ${files.length} file${files.length === 1 ? '' : 's'}.`;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to download selected items.';
    } finally {
      working = false;
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
      <div class="file-breadcrumb" aria-label="Folder path">
        <button class="crumb" on:click={goRoot}>ClusterStor</button>
        {#each breadcrumb as folder}
          <span class="crumb-separator">/</span>
          <button class="crumb" on:click={() => openFolder(folder)}>📁 {folder.name}</button>
        {/each}
      </div>
      <div class="actions">
        <button class="btn" on:click={downloadSelected} disabled={working || selected.size === 0}>Download selected{selected.size ? ` (${selected.size})` : ``}</button>
        <button class="btn ghost" on:click={load} disabled={loading || working}>Refresh</button>
      </div>
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
          <thead><tr><th class="check-col"><input type="checkbox" aria-label="Select all visible items" checked={allVisibleSelected} on:change={toggleAllVisible} /></th><th>Name</th><th>Type</th><th>Size</th><th>Modified</th><th></th></tr></thead>
          <tbody>
            {#each visibleItems as item}
              <tr>
                <td class="check-col"><input type="checkbox" aria-label={`Select ${item.name}`} checked={selected.has(item.node_id)} on:change={() => toggleSelected(item)} /></td>
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
