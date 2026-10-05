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
      <h2>Your clouds become one workspace.</h2>
      <p>ClusterStor sits above the storage providers you already use. It gives you one account, one file experience, and one synchronization layer while leaving the underlying storage where you chose to keep it.</p>
    </div>
    <div class="feature-grid detailed">
      <article class="feature-card"><span class="feature-icon">◎</span><h3>Unified file browser</h3><p>Open one workspace and browse provider-backed files without repeatedly switching between Google Drive, OneDrive, Dropbox, or Box interfaces.</p><small>Designed around provider-neutral logical file IDs so the app experience stays consistent even when the storage backend changes.</small></article>
      <article class="feature-card"><span class="feature-icon">↟</span><h3>Direct provider uploads</h3><p>Large file bodies move directly to the connected provider whenever practical instead of being unnecessarily relayed through ClusterStor.</p><small>ClusterStor tracks the logical file, provider object, version, and sync state around the transfer.</small></article>
      <article class="feature-card"><span class="feature-icon">↻</span><h3>Live multi-client updates</h3><p>Web and desktop clients can react quickly when another client creates a folder, uploads a file, or changes account state.</p><small>Clients resume from a monotonic event sequence so reconnecting does not require blindly rescanning everything.</small></article>
      <article class="feature-card"><span class="feature-icon">⌂</span><h3>Dedicated provider folders</h3><p>ClusterStor-created content is kept under a dedicated top-level <strong>ClusterStor</strong> folder at each supported provider.</p><small>The provider folder ID is tracked so normal renaming does not break ClusterStor's identity mapping.</small></article>
      <article class="feature-card"><span class="feature-icon">◫</span><h3>Built for multiple devices</h3><p>Your account is designed to span browser sessions and registered desktop devices rather than being tied to a single computer.</p><small>Device registration, revocation, live events, and account catch-up are already part of the backend foundation.</small></article>
      <article class="feature-card"><span class="feature-icon">◇</span><h3>Provider, Cloud, and Peer paths</h3><p>Use connected-provider storage first, then add ClusterStor Cloud or opt-in Peer capacity when those storage products launch.</p><small>Peer contribution is explicitly opt-in and remains off unless the user chooses to participate.</small></article>
      <article class="feature-card"><span class="feature-icon">▣</span><h3>Version-aware file model</h3><p>ClusterStor tracks logical files separately from their storage objects so versions and provider locations can evolve without changing the file's identity.</p><small>This creates a cleaner base for future version history, conflict handling, and provider migration.</small></article>
      <article class="feature-card"><span class="feature-icon">⌁</span><h3>Provider abstraction</h3><p>Google Drive is the first complete integration, but the backend is being structured so OneDrive, Dropbox, and Box follow the same model.</p><small>The goal is to add providers without rebuilding the product around each vendor's API.</small></article>
      <article class="feature-card"><span class="feature-icon">◈</span><h3>Security-conscious credentials</h3><p>Provider OAuth credentials are encrypted at rest and sensitive bearer credentials are kept out of public URLs and normal logs.</p><small>One-time socket tickets are used for browser real-time connections rather than exposing account session tokens in WebSocket URLs.</small></article>
    </div>

    <div class="how-it-works">
      <div class="section-heading left compact">
        <div class="eyebrow">How it works</div>
        <h2>Connect once. Work from ClusterStor.</h2>
      </div>
      <div class="steps-grid">
        <article><strong>01</strong><h3>Connect providers</h3><p>Start free with up to two supported cloud-provider adapters. ClusterStor obtains provider authorization without taking ownership of the underlying account.</p></article>
        <article><strong>02</strong><h3>Browse one workspace</h3><p>ClusterStor maps provider items into a consistent file model and gives you one place to browse and work with the content.</p></article>
        <article><strong>03</strong><h3>Create and upload</h3><p>New ClusterStor content is placed beneath the dedicated ClusterStor folder at the destination provider and recorded in the account event stream.</p></article>
        <article><strong>04</strong><h3>Stay synchronized</h3><p>Other clients learn that account changes are available, catch up from their last sequence, and refresh only what they need.</p></article>
      </div>
    </div>
  </section>

  <section id="about" class="marketing-section story-section parallax-story">
    <div class="story-copy">
      <div class="eyebrow">About ClusterStor</div>
      <h2>Storage should be infrastructure, not a collection of silos.</h2>
      <p>Most people accumulate cloud storage one service at a time. A Google account here, Microsoft storage there, another provider for work, another for archives. Each service may work well on its own, but the user ends up managing separate interfaces, folder systems, logins, and sync behaviors.</p>
      <p>ClusterStor is being built as the layer above those silos. The connected provider still stores the provider-backed bytes; ClusterStor supplies the unified account, logical file model, consistent client experience, synchronization events, device awareness, and future storage choices around them.</p>
      <p>That distinction matters. ClusterStor is not trying to hide where your data is stored. It is trying to make the storage location a choice instead of a daily usability problem.</p>
      <div class="about-principles">
        <span><strong>Provider choice</strong> Keep using the storage accounts you already have.</span>
        <span><strong>User-controlled Peer</strong> Peer participation is never silently enabled.</span>
        <span><strong>Portable architecture</strong> Files have ClusterStor identities independent of mutable provider paths.</span>
      </div>
    </div>
    <div class="story-stack">
      <div class="story-number"><strong>1</strong><span>Unified workspace across connected providers</span></div>
      <div class="story-number"><strong>4</strong><span>Target provider adapters: Google Drive, OneDrive, Dropbox, Box</span></div>
      <div class="story-number"><strong>2</strong><span>Provider adapters included on the free tier</span></div>
      <div class="story-number"><strong>0</strong><span>Peer contribution enabled by default</span></div>
    </div>
  </section>

  <section id="pricing" class="marketing-section section-dark">
    <div class="section-heading">
      <div class="eyebrow">Pricing</div>
      <h2>Start free. Add only what you need.</h2>
      <p>Provider adapters and storage capacity are separate. Connecting an existing cloud provider does not consume purchased ClusterStor Cloud or Peer capacity.</p>
    </div>

    <div class="pricing-matrix-wrap">
      <div class="pricing-label">Provider adapters</div>
      <div class="pricing-matrix adapter-matrix">
        <div class="pricing-row pricing-head"><div>Plan</div><div>Adapters</div><div>Supported providers</div><div>Price</div></div>
        <div class="pricing-row"><div><strong>Free</strong><small>Best for getting started</small></div><div><strong>2</strong> connected providers</div><div>Choose any 2 supported adapters</div><div><strong>$0</strong></div></div>
        <div class="pricing-row featured-row"><div><strong>Unified</strong><small>All supported provider adapters</small></div><div><strong>4</strong> connected providers</div><div>Google Drive, OneDrive, Dropbox, Box</div><div><strong>$2/mo</strong><small>or $15/year</small></div></div>
      </div>
    </div>

    <div class="pricing-matrix-wrap">
      <div class="pricing-label">ClusterStor Cloud <span>planned managed storage</span></div>
      <div class="pricing-matrix">
        <div class="pricing-row pricing-head"><div>Plan</div><div>Capacity</div><div>Storage type</div><div>Monthly</div></div>
        <div class="pricing-row"><div>Cloud 500</div><div>500 GB</div><div>Managed Cloud</div><div><strong>$8.99</strong></div></div>
        <div class="pricing-row featured-row"><div>Cloud 1TB</div><div>1 TB</div><div>Managed Cloud</div><div><strong>$14.99</strong></div></div>
        <div class="pricing-row"><div>Cloud 2TB</div><div>2 TB</div><div>Managed Cloud</div><div><strong>$24.99</strong></div></div>
      </div>
    </div>


    <div class="pricing-matrix-wrap peer-coming-soon">
      <div class="pricing-label">
        Peer Storage
        <span class="coming-soon-badge">COMING SOON</span>
      </div>
      <p class="coming-soon-copy">Peer Storage is planned for a future ClusterStor release. Participation will be completely opt-in and turned off by default.</p>
      <div class="pricing-matrix">
        <div class="pricing-row pricing-head"><div>Plan</div><div>Capacity</div><div>Participation</div><div>Monthly</div></div>
        <div class="pricing-row"><div>Peer 100</div><div>100 GB</div><div>Opt-in</div><div><strong>$2.99</strong></div></div>
        <div class="pricing-row"><div>Peer 500</div><div>500 GB</div><div>Opt-in</div><div><strong>$5.99</strong></div></div>
        <div class="pricing-row featured-row"><div>Peer 1TB</div><div>1 TB</div><div>Opt-in</div><div><strong>$8.99</strong></div></div>
        <div class="pricing-row"><div>Peer 2TB</div><div>2 TB</div><div>Opt-in</div><div><strong>$15.99</strong></div></div>
      </div>
    </div>

    <div class="pricing-explainer">
      <article><strong>Provider adapters</strong><p>These connect ClusterStor to storage you already pay for or receive from Google, Microsoft, Dropbox, or Box.</p></article>
      <article><strong>Peer Storage</strong><p>Future ClusterStor capacity using the opt-in Peer network. Participating as a storage contributor is always a separate choice.</p></article>
      <article><strong>ClusterStor Cloud</strong><p>Future managed ClusterStor storage for users who want capacity directly from ClusterStor instead of relying only on connected providers.</p></article>
    </div>
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
