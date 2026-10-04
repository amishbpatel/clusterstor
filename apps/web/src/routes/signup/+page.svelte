<script lang="ts">
  import { goto } from '$app/navigation';
  import { signup } from '$lib/api';

  let displayName = '';
  let email = '';
  let password = '';
  let error = '';
  let busy = false;

  async function submit() {
    error = '';
    busy = true;
    try {
      await signup(email, password, displayName);
      goto('/dashboard');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unable to create account.';
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head><title>Create account · ClusterStor</title></svelte:head>
<main class="auth-wrap">
  <section class="card auth-card">
    <div class="brand"><span class="brand-mark">C</span><span>ClusterStor</span></div>
    <div class="eyebrow">Get started</div>
    <h1>Create account</h1>
    <p class="muted">Your password must be at least 12 characters.</p>
    {#if error}<div class="error">{error}</div>{/if}
    <form class="form" on:submit|preventDefault={submit}>
      <label>Name<input class="input" bind:value={displayName} autocomplete="name" /></label>
      <label>Email<input class="input" type="email" bind:value={email} autocomplete="email" required /></label>
      <label>Password<input class="input" type="password" bind:value={password} minlength="12" autocomplete="new-password" required /></label>
      <button class="btn primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create account'}</button>
    </form>
    <p class="muted">Already registered? <a href="/login">Sign in</a></p>
  </section>
</main>
