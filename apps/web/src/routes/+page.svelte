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
  <title>ClusterStor · One file system for all your storage</title>
  <meta name="description" content="One file system across Google Drive, OneDrive, Dropbox, Box, ClusterStor Cloud, and opt-in Peer Storage." />
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
      <div class="eyebrow">One file system. Your choice of storage.</div>
      <h1>All your files. <span>One ClusterStor.</span></h1>
      <p>Use the cloud accounts you already have, add managed ClusterStor Cloud, or choose opt-in Peer Storage. ClusterStor gives you one consistent way to browse, sync, version, and manage files no matter where the bytes live.</p>
      <div class="hero-actions">
        <a class="btn primary btn-large" href="/signup">Start free</a>
        <a class="btn btn-large" href="#features">See how it works</a>
      </div>
      <div class="hero-trust">
        <span>Connect existing clouds</span>
        <span>ClusterStor Cloud planned</span>
        <span>Peer Storage opt-in</span>
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
      <div class="glass-window provider-card provider-card-two">Storage choice <span>Unified</span></div>
    </div>
  </section>

  <section id="features" class="marketing-section section-dark">
    <div class="section-heading">
      <div class="eyebrow">Features</div>
      <h2>One way to work with every place you store files.</h2>
      <p>ClusterStor brings provider storage, managed ClusterStor Cloud, and opt-in Peer Storage into one file experience. You choose where files live; ClusterStor keeps the way you work with them consistent.</p>
    </div>
    <div class="feature-grid detailed">
      <article class="feature-card"><span class="feature-icon">◎</span><h3>One My Files experience</h3><p>Browse and manage files through one interface instead of learning a different workflow for every storage company.</p><small>Google Drive is integrated first, with OneDrive, Dropbox, and Box following the same ClusterStor experience.</small></article>
      <article class="feature-card"><span class="feature-icon">▣</span><h3>One version history</h3><p>Upload new versions, download older versions, and restore an earlier copy as a new current version through one consistent ClusterStor workflow.</p><small>ClusterStor maps that experience onto each provider's native revision capabilities where available.</small></article>
      <article class="feature-card"><span class="feature-icon">↻</span><h3>Activity across your storage</h3><p>See uploads, downloads, renames, moves, deletes, restores, and version activity in one chronological account history.</p><small>The goal is one audit trail even when files are spread across different storage backends.</small></article>
      <article class="feature-card"><span class="feature-icon">◫</span><h3>Web, desktop, and mobile</h3><p>Use the web app today, with a desktop agent planned for native file synchronization and mobile clients planned for access on the go.</p><small>Desktop devices will register to your account and can be revoked centrally from ClusterStor.</small></article>
      <article class="feature-card"><span class="feature-icon">⌂</span><h3>Your existing cloud accounts</h3><p>Keep using Google Drive, OneDrive, Dropbox, and Box rather than moving everything just to use ClusterStor.</p><small>Connected-provider files remain stored with the provider you chose.</small></article>
      <article class="feature-card"><span class="feature-icon">☁</span><h3>ClusterStor Cloud</h3><p>Add managed storage directly from ClusterStor when you want capacity that is not tied to an outside cloud account.</p><small>Planned Cloud tiers range from 500 GB to 2 TB and will sit beside provider-backed files in the same experience.</small></article>
      <article class="feature-card"><span class="feature-icon">◇</span><h3>Peer Storage</h3><p>Choose lower-cost ClusterStor capacity backed by the future Peer network while keeping participation completely optional.</p><small>Peer contribution is off by default. Users who opt in will choose exactly how much local disk space to contribute.</small></article>
      <article class="feature-card"><span class="feature-icon">↔</span><h3>Storage without lock-in</h3><p>ClusterStor is designed so your daily file experience does not depend on one storage vendor winning forever.</p><small>Your storage location remains a choice, while ClusterStor provides the consistent interface around it.</small></article>
      <article class="feature-card"><span class="feature-icon">◈</span><h3>Security-conscious connections</h3><p>Provider credentials are encrypted at rest, devices can be individually revoked, and sensitive credentials stay out of normal URLs and logs.</p><small>ClusterStor is being built so convenience does not require giving up control over where files are stored.</small></article>
    </div>

    <div class="how-it-works">
      <div class="section-heading left compact">
        <div class="eyebrow">How it works</div>
        <h2>Choose storage. Work the same way.</h2>
      </div>
      <div class="steps-grid">
        <article><strong>01</strong><h3>Choose where you store</h3><p>Connect the cloud accounts you already use. As ClusterStor Cloud and Peer Storage launch, those become additional storage choices inside the same account.</p></article>
        <article><strong>02</strong><h3>See one file system</h3><p>ClusterStor gives you one My Files experience across those storage locations, with a consistent way to browse, organize, download, and manage files.</p></article>
        <article><strong>03</strong><h3>Use the same controls</h3><p>Version history, activity, provider management, and device controls stay in ClusterStor instead of changing every time the storage backend changes.</p></article>
        <article><strong>04</strong><h3>Take it to every device</h3><p>The desktop agent will bring ClusterStor into the native filesystem, while mobile clients will provide access, uploads, versions, and management on the go.</p></article>
      </div>
    </div>
  </section>

  <section id="about" class="marketing-section story-section parallax-story">
    <div class="story-copy">
      <div class="eyebrow">About ClusterStor</div>
      <h2>Your storage should feel like one system—even when it is not all stored in one place.</h2>
      <p>People naturally end up with files spread across Google Drive, Microsoft, Dropbox, Box, local computers, and whatever storage they add next. The problem is not only finding the files. It is having to switch between different interfaces, sync tools, version systems, activity histories, and storage rules.</p>
      <p>ClusterStor is being built to make those differences fade into the background. Your existing providers remain useful, ClusterStor Cloud adds managed capacity when you want it, and Peer Storage adds an opt-in alternative. All three storage paths are meant to appear through the same ClusterStor file experience.</p>
      <p>You should still know where a file is stored and be able to choose it. The goal is not to hide the storage provider. The goal is to stop the provider from dictating how you have to work with the file.</p>
      <div class="about-principles">
        <span><strong>Use what you already have</strong> Connect existing cloud accounts instead of migrating everything first.</span>
        <span><strong>Add ClusterStor capacity</strong> Cloud and Peer become first-class storage choices, not hidden add-ons.</span>
        <span><strong>Keep control</strong> Peer participation is opt-in, provider identity stays visible, and devices can be managed centrally.</span>
      </div>
    </div>
    <div class="story-stack">
      <div class="story-number"><strong>1</strong><span>File experience across every storage path</span></div>
      <div class="story-number"><strong>4</strong><span>Target provider adapters: Google Drive, OneDrive, Dropbox, Box</span></div>
      <div class="story-number"><strong>3</strong><span>Storage paths: providers, ClusterStor Cloud, Peer Storage</span></div>
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
      <article class="news-card"><span>Latest build</span><h3>Google Drive now runs through the full ClusterStor experience</h3><p>My Files, version history and restore, Activity, provider management, and device management are now working through the same account experience.</p><a href="/signup">Explore ClusterStor →</a></article>
      <article class="news-card"><span>Product direction</span><h3>One file system, multiple storage choices</h3><p>Existing cloud providers, ClusterStor Cloud, and future opt-in Peer Storage are being designed as equal storage paths behind one consistent interface.</p><a href="#features">Explore features →</a></article>
      <article class="news-card"><span>Coming next</span><h3>The desktop agent brings ClusterStor into the native filesystem</h3><p>Windows is planned first, followed by multi-provider adapters and the broader desktop synchronization engine.</p><a href="#about">Why we built it →</a></article>
    </div>
  </section>

  <section class="marketing-cta parallax-panel">
    <div>
      <div class="eyebrow">Ready when you are</div>
      <h2>Make all your storage feel like one system.</h2>
      <p>Start with the cloud accounts you already have. Add ClusterStor Cloud or opt-in Peer Storage as your needs grow.</p>
      <div class="hero-actions"><a class="btn primary btn-large" href="/signup">Create account</a><button class="btn btn-large" on:click={() => signInOpen = true}>Sign in</button></div>
    </div>
  </section>
</main>

<footer class="marketing-footer">
  <a class="brand marketing-brand" href="#top"><span class="brand-mark">C</span><span>ClusterStor</span></a>
  <span>One file system for all your storage.</span>
  <div><a href="#features">Features</a><a href="#pricing">Pricing</a><a href="#whats-new">What’s New</a></div>
</footer>

<SignInModal bind:open={signInOpen} on:close={() => signInOpen = false} />
