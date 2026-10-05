<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { goto } from '$app/navigation';
  import { login } from '$lib/api';

  export let open = false;
  const dispatch = createEventDispatcher();

  let email = '';
  let password = '';
  let error = '';
  let busy = false;

  function close() {
    open = false;
    error = '';
    dispatch('close');
  }

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

{#if open}
  <div class="modal-backdrop" role="presentation">
    <div class="modal-card" role="dialog" aria-modal="true" aria-labelledby="signin-title">
      <button class="modal-close" aria-label="Close sign in" on:click={close}>×</button>
      <div class="brand modal-brand"><span class="brand-mark">C</span><span>ClusterStor</span></div>
      <div class="eyebrow">Welcome back</div>
      <h2 id="signin-title">Sign in</h2>
      <p class="muted">Access your connected storage and ClusterStor workspace.</p>
      {#if error}<div class="error">{error}</div>{/if}
      <form class="form" on:submit|preventDefault={submit}>
        <label>Email<input class="input" type="email" bind:value={email} autocomplete="email" required /></label>
        <label>Password<input class="input" type="password" bind:value={password} autocomplete="current-password" required /></label>
        <div class="form-links">
          <a href="/forgot-password">Forgot password?</a>
          <a href="/reset-password">Reset password</a>
        </div>
        <button class="btn primary" type="submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
      </form>
      <p class="muted modal-footer">New to ClusterStor? <a href="/signup">Create an account</a></p>
    </div>
  </div>
{/if}
