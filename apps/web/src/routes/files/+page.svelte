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
  let trashItems: DriveItem[] = [];
  let trashMode = false;
  let uploadProgress = 0;
  let uploadLabel = '';
  let dragActive = false;

  $: currentProviderParent = currentFolder ? currentFolder.provider_item_id : rootProviderID;
  $: visibleItems = (trashMode ? trashItems : items.filter((item) => item.parent_item_id === currentProviderParent))
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

  function uploadToGoogle(uploadURL: string, file: File, onProgress: (fraction: number) => void) {
    return new Promise<{ id: string }>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('PUT', uploadURL);
      xhr.setRequestHeader('Content-Type', file.type || 'application/octet-stream');
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable && event.total > 0) onProgress(event.loaded / event.total);
      };
      xhr.onerror = () => reject(new Error(`Unable to upload ${file.name}.`));
      xhr.onload = () => {
        if (xhr.status < 200 || xhr.status >= 300) { reject(new Error(`Google Drive upload failed for ${file.name}.`)); return; }
        try {
          const body = JSON.parse(xhr.responseText || '{}');
          if (!body.id) throw new Error('Missing Google file ID.');
          resolve(body);
        } catch {
          reject(new Error(`Google Drive did not return a file ID for ${file.name}.`));
        }
      };
      xhr.send(file);
    });
  }

  async function uploadFiles(files: File[]) {
    if (files.length === 0) return;
    working = true;
    error = ''; success = '';
    uploadProgress = 0;
    try {
      for (let index = 0; index < files.length; index += 1) {
        const file = files[index];
        uploadLabel = files.length === 1 ? `Uploading ${file.name}` : `Uploading ${index + 1} of ${files.length}: ${file.name}`;
        const session = await api<{ upload_url: string }>('/api/v1/providers/google_drive/uploads', {
          method: 'POST',
          body: JSON.stringify({
            name: file.name,
            content_type: file.type || 'application/octet-stream',
            size_bytes: file.size,
            parent_node_id: currentFolder?.node_id || null
          })
        });
        const googleFile = await uploadToGoogle(session.upload_url, file, (fraction) => {
          uploadProgress = Math.round(((index + fraction) / files.length) * 100);
        });
        await api('/api/v1/providers/google_drive/uploads/complete', {
          method: 'POST',
          body: JSON.stringify({ provider_item_id: googleFile.id })
        });
        uploadProgress = Math.round(((index + 1) / files.length) * 100);
      }
      success = files.length === 1 ? 'Upload complete.' : `${files.length} files uploaded.`;
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to upload files.';
    } finally {
      working = false;
      uploadLabel = '';
      uploadProgress = 0;
    }
  }

  async function uploadSelected(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const files = Array.from(input.files || []);
    await uploadFiles(files);
    input.value = '';
  }

  function handleDragOver(event: DragEvent) {
    event.preventDefault();
    if (!trashMode) dragActive = true;
  }

  function handleDragLeave(event: DragEvent) {
    event.preventDefault();
    dragActive = false;
  }

  async function handleDrop(event: DragEvent) {
    event.preventDefault();
    dragActive = false;
    if (trashMode) return;
    await uploadFiles(Array.from(event.dataTransfer?.files || []));
  }

  function sourceLabel(provider: string) {
    if (provider === 'google_drive') return 'Google Drive';
    if (provider === 'onedrive') return 'OneDrive';
    if (provider === 'dropbox') return 'Dropbox';
    if (provider === 'box') return 'Box';
    if (provider === 'clusterstor') return 'ClusterStor';
    return provider || 'Unknown';
  }

  function collectDescendants(folder: DriveItem): DriveItem[] {
    const children = items.filter((item) => item.parent_item_id === folder.provider_item_id);
    return children.flatMap((child) => child.node_type === 'folder' ? [child, ...collectDescendants(child)] : [child]);
  }

  function folderPath(folder: DriveItem) {
    return ['ClusterStor', ...buildBreadcrumb(folder).map((item) => item.name)].join(' / ');
  }

  async function renameItem(item: DriveItem) {
    const name = window.prompt(`Rename ${item.node_type}`, item.name);
    if (!name?.trim() || name.trim() === item.name) return;
    working = true; error = ''; success = '';
    try {
      await api(`/api/v1/nodes/${item.node_id}/name`, { method: 'PATCH', body: JSON.stringify({ name: name.trim() }) });
      success = 'Item renamed.';
      await load();
    } catch (e) { error = e instanceof Error ? e.message : 'Unable to rename item.'; }
    finally { working = false; }
  }

  async function moveItem(item: DriveItem) {
    const descendantIDs = new Set(collectDescendants(item).map((entry) => entry.node_id));
    const folders = items.filter((candidate) => candidate.node_type === 'folder' && candidate.node_id !== item.node_id && !descendantIDs.has(candidate.node_id));
    const choices = ['0: ClusterStor', ...folders.map((folder, index) => `${index + 1}: ${folderPath(folder)}`)];
    const answer = window.prompt(`Move "${item.name}" to:\n\n${choices.join('\n')}\n\nEnter destination number:`);
    if (answer == null) return;
    const choice = Number.parseInt(answer, 10);
    if (!Number.isInteger(choice) || choice < 0 || choice > folders.length) { error = 'Invalid destination.'; return; }
    const parentNodeID = choice === 0 ? null : folders[choice - 1].node_id;
    working = true; error = ''; success = '';
    try {
      await api(`/api/v1/nodes/${item.node_id}/move`, { method: 'POST', body: JSON.stringify({ parent_node_id: parentNodeID }) });
      success = 'Item moved.';
      await load();
    } catch (e) { error = e instanceof Error ? e.message : 'Unable to move item.'; }
    finally { working = false; }
  }

  async function showTrash() {
    working = true; error = ''; success = '';
    try {
      const result = await api<{ items: DriveItem[] }>('/api/v1/trash');
      trashItems = result.items;
      trashMode = true;
      currentFolder = null;
      selected = new Set();
    } catch (e) { error = e instanceof Error ? e.message : 'Unable to load trash.'; }
    finally { working = false; }
  }

  function showFiles() {
    trashMode = false;
    selected = new Set();
  }

  async function restoreItem(item: DriveItem) {
    working = true; error = ''; success = '';
    try {
      await api(`/api/v1/nodes/${item.node_id}/restore`, { method: 'POST' });
      success = item.node_type === 'folder' ? 'Folder restored.' : 'File restored.';
      await showTrash();
    } catch (e) { error = e instanceof Error ? e.message : 'Unable to restore item.'; }
    finally { working = false; }
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

  async function deleteItem(item: DriveItem) {
    const label = item.node_type === 'folder'
      ? `Delete folder "${item.name}" and everything inside it? It will be moved to Google Drive trash.`
      : `Delete "${item.name}"? It will be moved to Google Drive trash.`;
    if (!window.confirm(label)) return;

    working = true;
    error = ''; success = '';
    try {
      await api<{ deleted: number }>(`/api/v1/nodes/${item.node_id}`, { method: 'DELETE' });
      selected.delete(item.node_id);
      selected = new Set(selected);
      success = item.node_type === 'folder' ? 'Folder deleted.' : 'File deleted.';
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to delete item.';
    } finally {
      working = false;
    }
  }

  function topLevelSelectedItems() {
    const chosen = items.filter((item) => selected.has(item.node_id));
    const chosenProviderIDs = new Set(chosen.map((item) => item.provider_item_id));
    return chosen.filter((item) => {
      let parent = item.parent_item_id;
      const seen = new Set<string>();
      while (parent && !seen.has(parent)) {
        if (chosenProviderIDs.has(parent)) return false;
        seen.add(parent);
        const parentItem = items.find((candidate) => candidate.provider_item_id === parent);
        parent = parentItem?.parent_item_id || null;
      }
      return true;
    });
  }

  async function deleteSelected() {
    const chosen = topLevelSelectedItems();
    if (chosen.length === 0) return;
    const folderCount = chosen.filter((item) => item.node_type === 'folder').length;
    const message = folderCount > 0
      ? `Delete ${chosen.length} selected item${chosen.length === 1 ? '' : 's'}? Selected folders and all contents will be moved to Google Drive trash.`
      : `Delete ${chosen.length} selected file${chosen.length === 1 ? '' : 's'}? They will be moved to Google Drive trash.`;
    if (!window.confirm(message)) return;

    working = true;
    error = ''; success = '';
    try {
      for (const item of chosen) {
        await api<{ deleted: number }>(`/api/v1/nodes/${item.node_id}`, { method: 'DELETE' });
      }
      selected = new Set();
      success = `Deleted ${chosen.length} selected item${chosen.length === 1 ? '' : 's'}.`;
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to delete selected items.';
    } finally {
      working = false;
    }
  }
  async function restoreSelected() {
    const chosen = trashItems.filter((item) => selected.has(item.node_id));
    if (chosen.length === 0) return;
    working = true; error = ''; success = '';
    try {
      for (const item of chosen) {
        await api(`/api/v1/nodes/${item.node_id}/restore`, { method: 'POST' });
      }
      selected = new Set();
      success = `Restored ${chosen.length} item${chosen.length === 1 ? '' : 's'}.`;
      await showTrash();
    } catch (e) { error = e instanceof Error ? e.message : 'Unable to restore selected items.'; }
    finally { working = false; }
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
      <button class="btn primary" on:click={chooseUpload} disabled={working || trashMode}>Upload files</button>
      <button class="btn ghost" on:click={trashMode ? showFiles : showTrash} disabled={working}>{trashMode ? 'My Files' : 'Trash'}</button>
      <input bind:this={fileInput} type="file" multiple style="display:none" on:change={uploadSelected} />
    </div>
  </div>

  {#if error}<div class="error" style="margin-bottom:16px">{error}</div>{/if}
  {#if success}<div class="success" style="margin-bottom:16px">{success}</div>{/if}

  {#if uploadLabel}
    <div class="upload-status">
      <div><span>{uploadLabel}</span><strong>{uploadProgress}%</strong></div>
      <progress max="100" value={uploadProgress}></progress>
    </div>
  {/if}

  <section class:drag-active={dragActive} class="card file-drop-zone" on:dragover={handleDragOver} on:dragleave={handleDragLeave} on:drop={handleDrop}>
    <div class="toolbar">
      <div class="file-breadcrumb" aria-label="Folder path">
        {#if trashMode}
          <strong>Trash</strong>
        {:else}
          <button class="crumb" on:click={goRoot}>ClusterStor</button>
          {#each breadcrumb as folder}
            <span class="crumb-separator">/</span>
            <button class="crumb" on:click={() => openFolder(folder)}>📁 {folder.name}</button>
          {/each}
        {/if}
      </div>
      <div class="actions">
        {#if !trashMode}
          <button class="btn" on:click={downloadSelected} disabled={working || selected.size === 0}>Download selected{selected.size ? ` (${selected.size})` : ``}</button>
          <button class="btn danger" on:click={deleteSelected} disabled={working || selected.size === 0}>Delete selected</button>
          <button class="btn ghost" on:click={load} disabled={loading || working}>Refresh</button>
        {:else}
          <button class="btn primary" on:click={restoreSelected} disabled={working || selected.size === 0}>Restore selected{selected.size ? ` (${selected.size})` : ``}</button>
          <button class="btn ghost" on:click={showTrash} disabled={working}>Refresh trash</button>
        {/if}
      </div>
    </div>

    {#if loading}
      <div class="empty">Loading files…</div>
    {:else if visibleItems.length === 0}
      <div class="empty">
        <strong>{trashMode ? 'Trash is empty.' : 'This folder is empty.'}</strong>
        {#if !trashMode}<p>Upload files, drop them here, or create a folder to get started.</p>{/if}
      </div>
    {:else}
      <div style="overflow:auto">
        <table class="table">
          <thead><tr><th class="check-col"><input type="checkbox" aria-label="Select all visible items" checked={allVisibleSelected} on:change={toggleAllVisible} /></th><th>Name</th><th>Source</th><th>Type</th><th>Size</th><th>Modified</th><th></th></tr></thead>
          <tbody>
            {#each visibleItems as item}
              <tr>
                <td class="check-col"><input type="checkbox" aria-label={`Select ${item.name}`} checked={selected.has(item.node_id)} on:change={() => toggleSelected(item)} /></td>
                <td>
                  {#if item.node_type === 'folder'}
                    {#if trashMode}
                      📁 {item.name}
                    {:else}
                      <button class="btn ghost" style="padding:4px 0" on:click={() => openFolder(item)}>📁 {item.name}</button>
                    {/if}
                  {:else}
                    📄 {item.name}
                  {/if}
                </td>
                <td><span class="source-pill">{sourceLabel(item.provider)}</span></td>
                <td>{item.node_type}</td>
                <td>{item.node_type === 'folder' ? '—' : formatBytes(item.size_bytes)}</td>
                <td>{item.modified_at ? new Date(item.modified_at).toLocaleString() : '—'}</td>
                <td>
                  <div class="actions">
                    {#if trashMode}
                      <button class="btn primary" on:click={() => restoreItem(item)} disabled={working}>Restore</button>
                    {:else}
                      {#if item.node_type === 'file'}
                        <button class="btn" on:click={() => download(item)}>Download</button>
                      {/if}
                      <button class="btn ghost" on:click={() => renameItem(item)} disabled={working}>Rename</button>
                      <button class="btn ghost" on:click={() => moveItem(item)} disabled={working}>Move</button>
                      <button class="btn danger" on:click={() => deleteItem(item)} disabled={working}>Delete</button>
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
</AppShell>
