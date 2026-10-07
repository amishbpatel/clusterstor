<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import AppShell from '$lib/AppShell.svelte';
  import { getToken } from '$lib/api';

  onMount(() => {
    if (!getToken()) goto('/login?next=/settings');
  });
</script>

<svelte:head><title>Settings · ClusterStor</title></svelte:head>

<AppShell>
  <div class="topbar">
    <div>
      <div class="eyebrow">Account & desktop</div>
      <h1>Settings</h1>
      <p class="muted" style="margin:.4rem 0 0">Manage ClusterStor behavior, storage connections, devices, Peer participation, and your plan.</p>
    </div>
  </div>

  <div class="settings-grid">
    <section class="card settings-card" id="general">
      <div class="eyebrow">General</div>
      <h2>ClusterStor experience</h2>
      <p>Desktop drive name, drive letter, startup behavior, and local desktop preferences will be managed here as the visual agent UI is completed.</p>
      <span class="settings-state">Desktop controls coming with the production tray UI</span>
    </section>

    <section class="card settings-card" id="sync">
      <div class="eyebrow">Sync</div>
      <h2>Synchronization & local storage</h2>
      <p>Local availability is device-specific. <strong>Automatic</strong> is the default; files can later be marked <strong>Always keep on this device</strong> or <strong>Online-only</strong>. Selective sync is separate and can exclude a subtree from a device entirely.</p>
      <p class="muted" style="margin:.6rem 0 0">ClusterStor will never reclaim local bytes that contain pending work, an unresolved conflict, or an unverified remote copy. Files On-Demand placeholder execution will be enabled after the provider sync worker is connected.</p>
      <span class="settings-state">Policy implemented · Pause / Resume is available from the Windows tray</span>
    </section>

    <section class="card settings-card" id="providers">
      <div class="eyebrow">Providers</div>
      <h2>Storage connections</h2>
      <p>Manage Google Drive now and OneDrive, Dropbox, and Box as those adapters launch.</p>
      <a class="btn ghost" href="/providers">Manage providers</a>
    </section>

    <section class="card settings-card" id="devices">
      <div class="eyebrow">Devices</div>
      <h2>Authorized computers</h2>
      <p>Review desktop agents, last-seen activity, Peer contribution, and revoke device credentials.</p>
      <a class="btn ghost" href="/devices">Manage devices</a>
    </section>

    <section class="card settings-card" id="peer">
      <div class="eyebrow">Peer Storage</div>
      <h2>Peer participation</h2>
      <p>Peer contribution is always opt-in. Desktop settings will allow contribution to be enabled, disabled, or resized explicitly.</p>
      <span class="settings-state">Never enabled automatically</span>
    </section>

    <section class="card settings-card" id="notifications">
      <div class="eyebrow">Notifications</div>
      <h2>Desktop alerts</h2>
      <p>Control sync-error, conflict, provider, storage-capacity, update, and account notifications.</p>
      <span class="settings-state">Planned</span>
    </section>

    <section class="card settings-card settings-billing" id="billing">
      <div>
        <div class="eyebrow">Billing</div>
        <h2>Plan & storage</h2>
        <p>Billing will cover the provider-adapter plan, ClusterStor Cloud capacity, Peer Storage capacity, renewal cycle, payment method, invoices, upgrades, downgrades, and cancellation.</p>
      </div>
      <div class="settings-plan">
        <strong>Free</strong>
        <span>Up to 2 connected providers</span>
        <small>Billing checkout and subscription enforcement will be connected during the billing phase.</small>
      </div>
    </section>

    <section class="card settings-card" id="about">
      <div class="eyebrow">About & updates</div>
      <h2>ClusterStor Desktop</h2>
      <p>Version information, update channel, automatic-update controls, release notes, and support diagnostics will live here.</p>
      <span class="settings-state">Windows agent developer preview</span>
    </section>
  </div>
</AppShell>
