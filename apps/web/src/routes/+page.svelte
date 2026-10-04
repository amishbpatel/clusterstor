<script lang="ts">
  import { goto } from '$app/navigation';
  import { browser } from '$app/environment';
  import { getToken } from '$lib/api';
  import SignInModal from '$lib/SignInModal.svelte';

  let signInOpen = false;

  if (browser && getToken()) {
    goto('/dashboard');
  }
</script>

<svelte:head>
  <title>ClusterStor · One drive across your clouds</title>
  <meta name="description" content="Connect your cloud storage in one fast, unified ClusterStor workspace." />
</svelte:head>

<header class="marketing-header">
  <a class="brand marketing-brand" href="#top"><span class="brand-mark">C</span><span>ClusterStor</span></a>
  <nav class="marketing-nav" aria-label="Primary">
    <a href="#features">Features</a>
    <a href="#about">About</a>
    <a href="#pricing">Pricing</a>
    <a href="#whats-new">What’s New</a>
  </nav>
  <div class="marketing-actions">
    <button class="btn ghost" on:click={() => signInOpen = true}>Sign in</button>
    <a class="btn primary" href="/signup">Create account</a>
  </div>
</header>

<main id="top" class="marketing-page">
  <section class="marketing-hero parallax-panel">
    <div class="hero-orb orb-one"></div>
    <div class="hero-orb orb-two"></div>
    <div class="hero-copy">
      <div class="eyebrow">Unified cloud storage</div>
      <h1>One drive across <span>all your clouds.</span></h1>
      <p>Connect the storage you already use, keep ClusterStor-managed files organized in one place, and move through your data without bouncing between apps.</p>
      <div class="hero-actions">
        <a class="btn primary btn-large" href="/signup">Start free</a>
        <a class="btn btn-large" href="#features">See how it works</a>
      </div>
      <div class="hero-trust">
        <span>Google Drive first</span>
        <span>More providers next</span>
        <span>Files stay organized</span>
      </div>
    </div>
    <div class="hero-visual" aria-hidden="true">
      <div class="glass-window main-window">
        <div class="window-bar"><span></span><span></span><span></span></div>
        <div class="window-row"><strong>My Files</strong><span>ClusterStor</span></div>
        <div class="file-row"><span>📁</span><div><strong>Projects</strong><small>Folder</small></div></div>
        <div class="file-row"><span>📄</span><div><strong>Launch-plan.pdf</strong><small>2.4 MB</small></div></div>
        <div class="file-row"><span>📄</span><div><strong>Forecast.xlsx</strong><small>884 KB</small></div></div>
      </div>
      <div class="glass-window provider-card provider-card-one">Google Drive <span>Connected</span></div>
      <div class="glass-window provider-card provider-card-two">Live sync <span>On</span></div>
    </div>
  </section>

  <section id="features" class="marketing-section section-dark">
    <div class="section-heading">
      <div class="eyebrow">Features</div>
      <h2>Your storage, without the switching.</h2>
      <p>ClusterStor is designed around one simple idea: your files should feel like one system even when the storage underneath comes from different providers.</p>
    </div>
    <div class="feature-grid">
      <article class="feature-card"><span class="feature-icon">◎</span><h3>One file view</h3><p>Browse connected storage from a single ClusterStor workspace instead of jumping between provider tabs.</p></article>
      <article class="feature-card"><span class="feature-icon">↟</span><h3>Direct uploads</h3><p>Send files directly to the connected provider while ClusterStor keeps the logical file record in sync.</p></article>
      <article class="feature-card"><span class="feature-icon">↻</span><h3>Live changes</h3><p>Connected clients can catch up from a monotonic event sequence and react to changes in near real time.</p></article>
      <article class="feature-card"><span class="feature-icon">⌂</span><h3>Clean provider folders</h3><p>ClusterStor-created content stays under a dedicated top-level ClusterStor folder in each provider.</p></article>
      <article class="feature-card"><span class="feature-icon">◫</span><h3>Multi-device ready</h3><p>The architecture is built for desktop, web, and future mobile clients sharing the same account state.</p></article>
      <article class="feature-card"><span class="feature-icon">◇</span><h3>Cloud + Peer roadmap</h3><p>Provider storage comes first, with ClusterStor Cloud and opt-in Peer Storage planned behind the same experience.</p></article>
    </div>
  </section>

  <section id="about" class="marketing-section story-section parallax-story">
    <div class="story-copy">
      <div class="eyebrow">About ClusterStor</div>
      <h2>Built to make fragmented storage feel invisible.</h2>
      <p>Cloud storage became easy to buy and hard to manage. ClusterStor is being built to put the user back in control: one identity, one file view, one consistent experience across the providers they choose.</p>
      <p>The provider remains the storage destination. ClusterStor provides the organization, synchronization, account logic, and client experience around it.</p>
    </div>
    <div class="story-stack">
      <div class="story-number"><strong>1</strong><span>Unified workspace</span></div>
      <div class="story-number"><strong>∞</strong><span>Provider expansion path</span></div>
      <div class="story-number"><strong>0</strong><span>Peer participation by default</span></div>
    </div>
  </section>

  <section id="pricing" class="marketing-section section-dark">
    <div class="section-heading">
      <div class="eyebrow">Pricing</div>
      <h2>Simple storage tiers.</h2>
      <p>Connected provider storage remains separate. ClusterStor Cloud and Peer capacity are purchased independently when those products launch.</p>
    </div>
    <div class="pricing-groups">
      <div>
        <div class="pricing-label">Peer Storage</div>
        <div class="pricing-grid">
          <article class="price-card"><strong>100 GB</strong><span>$2.99/mo</span></article>
          <article class="price-card"><strong>500 GB</strong><span>$5.99/mo</span></article>
          <article class="price-card featured"><strong>1 TB</strong><span>$8.99/mo</span></article>
          <article class="price-card"><strong>2 TB</strong><span>$15.99/mo</span></article>
        </div>
      </div>
      <div>
        <div class="pricing-label">ClusterStor Cloud</div>
        <div class="pricing-grid three">
          <article class="price-card"><strong>500 GB</strong><span>$8.99/mo</span></article>
          <article class="price-card featured"><strong>1 TB</strong><span>$14.99/mo</span></article>
          <article class="price-card"><strong>2 TB</strong><span>$24.99/mo</span></article>
        </div>
      </div>
    </div>
    <div class="pricing-note">Connected Google Drive, OneDrive, Dropbox, and Box capacity is not counted as purchased ClusterStor Cloud or Peer capacity.</div>
  </section>

  <section id="whats-new" class="marketing-section news-section">
    <div class="section-heading left">
      <div class="eyebrow">What’s New</div>
      <h2>Follow the build.</h2>
      <p>This section can become the ClusterStor blog/changelog as development progresses.</p>
    </div>
    <div class="news-grid">
      <article class="news-card"><span>Latest build</span><h3>Google Drive vertical slice is connected end to end</h3><p>Connect, browse, create folders, upload, finalize, download, and receive live change signals.</p><a href="/signup">Try the local build →</a></article>
      <article class="news-card"><span>Architecture</span><h3>Provider-root policy keeps files organized</h3><p>Every ClusterStor-created item lives beneath a dedicated ClusterStor folder at the provider.</p><a href="#features">Explore features →</a></article>
      <article class="news-card"><span>Coming next</span><h3>OneDrive and stronger multi-device sync</h3><p>The provider abstraction is designed so the next integrations can reuse the same logical model.</p><a href="#about">Why we built it →</a></article>
    </div>
  </section>

  <section class="marketing-cta parallax-panel">
    <div>
      <div class="eyebrow">Ready when you are</div>
      <h2>Bring your clouds together.</h2>
      <p>Create your ClusterStor account and start with the storage you already have.</p>
      <div class="hero-actions"><a class="btn primary btn-large" href="/signup">Create account</a><button class="btn btn-large" on:click={() => signInOpen = true}>Sign in</button></div>
    </div>
  </section>
</main>

<footer class="marketing-footer">
  <a class="brand marketing-brand" href="#top"><span class="brand-mark">C</span><span>ClusterStor</span></a>
  <span>One drive across your clouds.</span>
  <div><a href="#features">Features</a><a href="#pricing">Pricing</a><a href="#whats-new">What’s New</a></div>
</footer>

<SignInModal bind:open={signInOpen} on:close={() => signInOpen = false} />
