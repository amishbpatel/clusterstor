<script lang="ts">
  import { goto } from '$app/navigation';
  import { browser } from '$app/environment';
  import { getToken } from '$lib/api';

  if (browser && getToken()) {
    goto('/dashboard');
  }
</script>

<svelte:head>
  <title>ClusterStor</title>
  <meta name="description" content="One drive across your clouds." />
</svelte:head>

<main class="hero">
  <div class="brand" style="justify-content:center;">
    <span class="brand-mark">C</span>
    <span>ClusterStor</span>
  </div>
  <div class="eyebrow">Unified cloud storage</div>
  <h1>One drive across your clouds.</h1>
  <p>Connect your storage providers, keep ClusterStor-managed content organized in one place, and access it from any ClusterStor client.</p>
  <div class="hero-actions">
    <a class="btn primary" href="/login">Sign in</a>
    <a class="btn" href="/signup">Create account</a>
  </div>
</main>
