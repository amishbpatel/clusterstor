<script lang="ts">
  let password = '';
  let confirmPassword = '';
  let message = '';

  function submit() {
    message = password !== confirmPassword ? 'Passwords do not match.' : 'Reset-token processing is not enabled in this local build yet.';
  }
</script>

<svelte:head><title>Reset password · ClusterStor</title></svelte:head>
<main class="auth-wrap">
  <section class="card auth-card">
    <a class="brand" href="/"><span class="brand-mark">C</span><span>ClusterStor</span></a>
    <div class="eyebrow">Account recovery</div>
    <h1>Reset password</h1>
    <p class="muted">Choose a new password after opening a valid recovery link.</p>
    {#if message}<div class={password === confirmPassword ? 'success' : 'error'}>{message}</div>{/if}
    <form class="form" on:submit|preventDefault={submit}>
      <label>New password<input class="input" type="password" bind:value={password} minlength="12" autocomplete="new-password" required /></label>
      <label>Confirm password<input class="input" type="password" bind:value={confirmPassword} minlength="12" autocomplete="new-password" required /></label>
      <button class="btn primary" type="submit">Reset password</button>
    </form>
    <p class="muted"><a href="/">← Back to ClusterStor</a></p>
  </section>
</main>
