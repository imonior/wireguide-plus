<script>
  import { t } from '../i18n/index.js';
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';
  import { onMount } from 'svelte';

  let routes = [];
  let loading = false;
  let error = '';

  async function loadRoutes() {
    loading = true;
    error = '';
    try {
      routes = (await TunnelService.GetRoutingTable()) || [];
    } catch (e) {
      error = e?.message || String(e);
    }
    loading = false;
  }

  // Prefer the backend's authoritative per-route flag (it matches the
  // route's interface against the actually-active tunnel interface names),
  // then fall back to the old name heuristic for platforms where the
  // backend can't tell (e.g. non-Windows builds).
  function isVPN(route) {
    if (route.is_vpn === true) return true;
    if (route.is_vpn === false) return false;
    const iface = route.interface || '';
    return iface.startsWith('utun') || iface.startsWith('wg') || iface.startsWith('tun');
  }

  // The backend only ever sends a real L3 next-hop IP in the Gateway field,
  // or an empty string for on-link routes (link#N / bare interface name /
  // MAC neighbor entries are all normalized away). Empty renders as "—".
  // IPv6 gateways arrive zone-stripped already; this frontend pass is
  // belt-and-suspenders for stale data.
  function gwLabel(gw) {
    if (!gw) return '-';
    return stripZone(gw);
  }

  // IPv6 addresses carry a zone id ("fe80::1%utun2"). The backend strips
  // it before sending, and this frontend-side pass is belt-and-suspenders
  // for stale data: only the zone token is removed, a trailing prefix
  // length ("fe80::%lo0/64" → "fe80::/64") survives.
  function stripZone(v) {
    if (typeof v !== 'string' || !v.includes('%')) return v;
    const slash = v.indexOf('/');
    const addr = slash >= 0 ? v.slice(0, slash) : v;
    const pct = addr.indexOf('%');
    const clean = pct > 0 ? addr.slice(0, pct) : addr;
    return slash >= 0 ? clean + v.slice(slash) : clean;
  }

  onMount(loadRoutes);

  // Interface kind → i18n key. The backend only ever sends the stable
  // kinds classifyIface produces; unknown kinds render as-is.
  const IFACE_TYPE_KEYS = new Set([
    'wifi', 'ethernet', 'vpn', 'loopback', 'bridge', 'virtual', 'cellular',
  ]);
  function ifaceTypeLabel(kind) {
    return IFACE_TYPE_KEYS.has(kind) ? $t(`tools.route_iface_type_${kind}`) : kind;
  }
</script>

<div class="route-viz">
  <div class="page-toolbar">
    <h2 class="page-title">{$t('tools.route_title')}</h2>
    <div class="toolbar-actions">
      <span class="legend-inline">
        <span class="legend-item"><span class="dot vpn-dot"></span>{$t('tools.route_legend_vpn')}</span>
        <span class="legend-item"><span class="dot direct-dot"></span>{$t('tools.route_legend_direct')}</span>
      </span>
      <button class="btn-action" on:click={loadRoutes} disabled={loading}>
        {loading ? '…' : $t('tools.route_reload')}
      </button>
    </div>
  </div>

  <div class="page-body">
    <p class="page-description">{$t('tools.route_desc')}</p>

    {#if error}
      <div class="error-msg">{error}</div>
    {/if}

    {#if routes.length > 0}
      <div class="route-table">
        <div class="route-header">
          <span>{$t('tools.route_header_dest')}</span>
          <span>{$t('tools.route_header_gateway')}</span>
          <span>{$t('tools.route_header_iface')}</span>
        </div>
        {#each routes as route}
          <div class="route-row" class:vpn={isVPN(route)}>
            <span class="dest" title={route.destination}>{stripZone(route.destination)}</span>
            <span class="gw" title={route.gateway || ''}>{gwLabel(route.gateway)}</span>
            <span class="iface" class:vpn-iface={isVPN(route)}>
              {route.interface}
              {#if route.interface_detail || route.interface_type}
                <!-- Badge text: the specific label wins — the macOS
                     hardware port name ("Wi-Fi", identical to the tunnel
                     editor's egress dropdown) or the creator software
                     ("Tailscale"). The generic kind is the fallback and
                     stays in the tooltip. -->
                <span
                  class="iface-type"
                  title={[route.interface_type && ifaceTypeLabel(route.interface_type), route.interface_detail].filter(Boolean).join(' · ')}>
                  {route.interface_detail || ifaceTypeLabel(route.interface_type)}
                </span>
              {/if}
              {#if isVPN(route)}
                <span class="vpn-badge">{$t('tools.route_vpn_badge')}</span>
              {:else}
                <span class="direct-badge">{$t('tools.route_direct_badge')}</span>
              {/if}
            </span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .route-viz {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }

  /* Toolbar — matches History/LogViewer pattern. */
  .page-toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-2) var(--space-4);
    border-bottom: 0.5px solid var(--border);
    gap: var(--space-2);
    flex-shrink: 0;
  }
  .page-title {
    margin: 0;
    font: var(--text-headline);
    color: var(--text-primary);
  }
  .toolbar-actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .legend-inline {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    font: var(--text-footnote);
    color: var(--text-secondary);
  }
  .legend-item { display: inline-flex; align-items: center; gap: 5px; }
  .dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }
  .vpn-dot { background: var(--green); }
  .direct-dot { background: var(--text-muted); }

  .btn-action {
    height: 22px;
    padding: 0 var(--space-2);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: var(--radius-xs);
    color: var(--text-secondary);
    font: var(--text-footnote);
    cursor: pointer;
  }
  .btn-action:hover:not(:disabled) { background: var(--bg-hover); }
  .btn-action:disabled { opacity: 0.5; cursor: progress; }

  .page-body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-4) var(--space-4) var(--space-5);
    display: flex;
    flex-direction: column;
  }
  .page-description {
    margin: 0 0 var(--space-3);
    font: var(--text-body);
    color: var(--text-secondary);
    line-height: 1.5;
    max-width: 640px;
  }

  .route-table {
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: var(--radius-md);
    overflow-y: auto;
    flex: 1;
    min-height: 0;
  }
  .route-header {
    display: grid;
    /* minmax(0,…) is what makes the ellipsis work: a bare 1fr track is
       sized by its content, so a long IPv6 destination expanded the
       column and either pushed the row's other cells around or overflowed
       into them ("IP mixed with utun"). Destination gets the most room
       because it is the longest and most important value. */
    grid-template-columns: minmax(0, 1.6fr) minmax(0, 1.2fr) minmax(0, 1fr);
    padding: var(--space-2) var(--space-3);
    font: var(--text-footnote);
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    border-bottom: 0.5px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg-card);
    z-index: 1;
  }
  .route-row {
    display: grid;
    grid-template-columns: minmax(0, 1.6fr) minmax(0, 1.2fr) minmax(0, 1fr);
    padding: var(--space-1) var(--space-3);
    font: var(--text-body);
    font-family: var(--font-mono);
    border-bottom: 0.5px solid var(--border);
  }
  .route-row:last-child { border-bottom: 0; }
  .route-row.vpn { background: color-mix(in srgb, var(--green) 6%, transparent); }
  /* Never let a cell bleed into its neighbour — clip with an ellipsis and
     expose the full value through the title tooltip instead. */
  .dest,
  .gw {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dest { color: var(--text-primary); }
  .gw { color: var(--text-secondary); }
  .iface { color: var(--text-secondary); display: flex; align-items: center; gap: var(--space-1); }
  .iface-type {
    flex-shrink: 0;
    padding: 0 4px;
    border: 0.5px solid var(--border);
    border-radius: var(--radius-xs);
    background: var(--bg-card);
    color: var(--text-tertiary);
    font: var(--text-footnote);
  }
  .vpn-iface { color: var(--green); }
  .vpn-badge {
    padding: 1px var(--space-1);
    background: var(--green);
    color: var(--text-inverse);
    border-radius: var(--radius-xs);
    font: var(--text-footnote);
    font-weight: 600;
  }
  .direct-badge {
    padding: 1px var(--space-1);
    background: var(--bg-hover);
    color: var(--text-tertiary);
    border: 0.5px solid var(--border);
    border-radius: var(--radius-xs);
    font: var(--text-footnote);
    font-weight: 600;
  }
  .error-msg {
    margin-bottom: var(--space-3);
    padding: var(--space-2) var(--space-3);
    background: var(--error-bg);
    border: 0.5px solid color-mix(in srgb, var(--red) 35%, transparent);
    border-radius: var(--radius-sm);
    color: var(--error-text);
    font: var(--text-body);
  }
</style>
