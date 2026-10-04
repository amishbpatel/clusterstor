<script lang="ts">
  import { goto } from '$app/navigation';
  import { login } from '$lib/api';

  let email = '';
  let password = '';
  let error = '';
  let busy = false;

  async function submit() {
    error = '';
    busy = true;
    try {
      await login(email, password);
      goto('/dashboard');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to sign in.';
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head><title>Sign in · ClusterStor</title></svelte:head>
<main class="auth-wrap">
  <section class="card auth-card">
    <div class="brand"><span class="brand-mark">C</span><span>ClusterStor</span></div>
    <div class="eyebrow">Welcome back</div>
    <h1>Sign in</h1>
    <p class="muted">Use your ClusterStor account to access your connected storage.</p>
    {#if error}<div class="error">{error}</div>{/if}
    <form class="form" on:submit|preventDefault={submit}>
      <label>Email<input class="input" type="email" bind:value={email} autocomplete="email" required /></label>
      <label>Password<input class="input" type="password" bind:value={password} autocomplete="current-password" required /></label>
      <button class="btn primary" type="submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
    </form>
    <p class="muted">No account? <a href="/signup">Create one</a></p>
  </section>
</main>
