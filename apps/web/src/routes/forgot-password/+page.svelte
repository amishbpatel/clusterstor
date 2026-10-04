<script lang="ts">
  let email = '';
  let submitted = false;
</script>

<svelte:head><title>Forgot password · ClusterStor</title></svelte:head>
<main class="auth-wrap">
  <section class="card auth-card">
    <a class="brand" href="/"><span class="brand-mark">C</span><span>ClusterStor</span></a>
    <div class="eyebrow">Account recovery</div>
    <h1>Forgot password?</h1>
    <p class="muted">Enter your email address. Password-reset email delivery will be enabled with the recovery backend before public launch.</p>
    {#if submitted}
      <div class="success">Recovery screen is ready. Email delivery is not enabled in this local build yet.</div>
    {/if}
    <form class="form" on:submit|preventDefault={() => submitted = true}>
      <label>Email<input class="input" type="email" bind:value={email} autocomplete="email" required /></label>
      <button class="btn primary" type="submit">Request reset link</button>
    </form>
    <p class="muted"><a href="/">← Back to ClusterStor</a></p>
  </section>
</main>
