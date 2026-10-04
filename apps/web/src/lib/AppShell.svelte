<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { logout } from '$lib/api';

  async function signOut() {
    await logout();
    goto('/login');
  }
</script>

<div class="shell">
  <aside class="sidebar">
    <a class="brand" href="/dashboard"><span class="brand-mark">C</span><span>ClusterStor</span></a>
    <nav class="nav">
      <a class:active={$page.url.pathname.startsWith('/dashboard')} href="/dashboard">Dashboard</a>
      <a class:active={$page.url.pathname.startsWith('/files')} href="/files">My Files</a>
      <button class="btn ghost" on:click={signOut}>Sign out</button>
    </nav>
  </aside>
  <main class="main">
    <slot />
  </main>
</div>
