<script lang="ts">
  export let label = 'Loading…';
</script>

<div class="loading-state" role="status" aria-live="polite">
  <span class="loading-spinner" aria-hidden="true"></span>
  <span>{label}</span>
</div>
