<script>
  import { afterUpdate, onMount } from 'svelte';
  import { logs, clearLogs, orderedLogs } from '../stores/logs.js';
  import { t } from '../i18n/index.js';
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';

  let filter = 'all';
  let openingFolder = false;
  let folderError = '';

  // Reveal the on-disk log directory (the same RPC the settings page's
  // "log folder" row uses — kept here so users deep in a log-reading
  // session can grab the files without detouring through settings).
  async function openFolder() {
    if (openingFolder) return;
    openingFolder = true;
    folderError = '';
    try {
      await TunnelService.OpenFolder('logs');
    } catch (e) {
      folderError = e?.message || String(e);
    }
    openingFolder = false;
  }
  // Category filter — mirrors logging.ValidCategories on the Go side.
  // Keep in sync when adding a category.
  const categories = ['app', 'update', 'settings', 'tunnel', 'network', 'policy', 'system'];
  let categoryFilter = 'all';
  let autoScroll = true;
  let logContainer;
  let prevVersion = -1;
  let shouldScroll = true;
  let copyFeedback = false;

  const levels = ['debug', 'info', 'warn', 'error'];

  // ---- Keyword search (highlights + filters the CURRENT view) ----
  // 关键词搜索（对当前视图做过滤并高亮命中片段）
  let search = '';

  // ---- On-disk daily log files ----
  // An empty selectedFile means the live session view; picking a file
  // switches the pane to that file's (tailed) raw content.
  // selectedFile 为空表示实时会话视图；选择文件后切到该文件的（尾部）原始内容。
  let logFiles = [];
  let selectedFile = '';
  let fileContent = null;
  let fileTruncated = false;
  let fileLoading = false;
  let fileError = '';

  async function refreshLogFiles() {
    try {
      logFiles = (await TunnelService.ListLogFiles()) || [];
    } catch (e) {
      logFiles = [];
    }
  }

  async function selectFile(name) {
    selectedFile = name;
    fileError = '';
    fileTruncated = false;
    if (!name) {
      fileContent = null;
      return;
    }
    fileLoading = true;
    try {
      fileContent = await TunnelService.ReadLogFile(name);
      fileTruncated = !!fileContent?.truncated;
    } catch (e) {
      fileError = e?.message || String(e);
      fileContent = null;
      selectedFile = '';
    }
    fileLoading = false;
  }

  // highlightParts splits text into alternating plain/matched fragments for
  // case-insensitive <mark> highlighting. Params only — safe from the Svelte
  // 5 untrack trap (no reads of component state inside).
  // highlightParts 把文本切成 普通/命中 交替的片段，用于大小写不敏感的
  // <mark> 高亮。仅依赖参数——不受 Svelte 5 untrack 陷阱影响（函数体内
  // 不读组件状态）。
  function highlightParts(text, term) {
    const raw = text ?? '';
    const t = (term || '').trim();
    if (!t) return [{ s: raw, hit: false }];
    const lower = raw.toLowerCase();
    const needle = t.toLowerCase();
    const parts = [];
    let i = 0;
    for (;;) {
      const idx = lower.indexOf(needle, i);
      if (idx < 0) {
        parts.push({ s: raw.slice(i), hit: false });
        break;
      }
      if (idx > i) parts.push({ s: raw.slice(i, idx), hit: false });
      parts.push({ s: raw.slice(idx, idx + needle.length), hit: true });
      i = idx + needle.length;
    }
    return parts;
  }

  // Materialize the ring-buffer into a chronological array — only when
  // LogViewer is mounted (here). Push path in logs.js is O(1) and
  // allocation-free; the slice/concat happens only on this read.
  $: ordered = orderedLogs($logs);
  $: filtered = ordered.filter((entry) => {
    // Exact-level filtering: clicking "INFO" shows only info records —
    // the count badges already show how many records each level has, so
    // a ">= level" filter (which silently kept showing everything when a
    // level had zero records) only confused users.
    if (filter !== 'all' && (entry.level || 'info') !== filter) {
      return false;
    }
    if (categoryFilter !== 'all' && (entry.category || 'app') !== categoryFilter) {
      return false;
    }
    return true;
  });

  // Search narrows the already level/category-filtered live entries by
  // message content (case-insensitive), and raw file lines in file mode.
  // 搜索在已按级别/类别过滤的实时记录上按消息内容（大小写不敏感）再收窄；
  // 文件模式下则作用于原始行。
  $: term = search.trim().toLowerCase();
  $: liveFiltered = term
    ? filtered.filter((entry) => (entry.message || '').toLowerCase().includes(term))
    : filtered;
  $: fileLines = fileContent?.lines ?? [];
  $: fileFiltered =
    selectedFile && term ? fileLines.filter((line) => line.toLowerCase().includes(term)) : fileLines;

  // Per-level / per-category counts so the filter buttons always show how
  // many records each level/category actually has.
  $: levelCounts = (() => {
    const c = { debug: 0, info: 0, warn: 0, error: 0 };
    for (const e of ordered) c[e.level] = (c[e.level] || 0) + 1;
    return c;
  })();
  $: categoryCounts = (() => {
    const c = {};
    for (const e of ordered) {
      const k = e.category || 'app';
      c[k] = (c[k] || 0) + 1;
    }
    return c;
  })();

  $: {
    const v = $logs?.version ?? 0;
    if (v !== prevVersion) {
      shouldScroll = true;
      prevVersion = v;
    }
  }

  onMount(() => {
    refreshLogFiles();
    if (logContainer) logContainer.scrollTop = logContainer.scrollHeight;
  });

  afterUpdate(() => {
    // Auto-scroll only applies to the live session view — a file view is a
    // static snapshot and scrolling it to the bottom on every update would
    // fight the user's own scrolling.
    // 自动滚动只作用于实时会话视图——文件视图是静态快照，每次更新都滚到底
    // 会干扰用户自己的滚动。
    if (!selectedFile && autoScroll && shouldScroll && logContainer) {
      logContainer.scrollTop = logContainer.scrollHeight;
      shouldScroll = false;
    }
  });

  function clear() {
    clearLogs();
  }

  function formatTime(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    const h = String(d.getHours()).padStart(2, '0');
    const m = String(d.getMinutes()).padStart(2, '0');
    const s = String(d.getSeconds()).padStart(2, '0');
    const ms = String(d.getMilliseconds()).padStart(3, '0');
    return `${h}:${m}:${s}.${ms}`;
  }

  function formatEntry(entry) {
    return `${formatTime(entry.time)}\t${entry.source}\t${entry.level.toUpperCase()}\t${entry.category || 'app'}\t${entry.message}`;
  }

  async function copyAll() {
    // In file mode copy the raw (filtered) lines verbatim — they are what
    // the user would grab from the file itself.
    // 文件模式下按原样复制（过滤后的）原始行——那正是用户从文件里取的内容。
    const text = selectedFile
      ? fileFiltered.join('\n')
      : liveFiltered.map(formatEntry).join('\n');
    try {
      await navigator.clipboard.writeText(text);
      copyFeedback = true;
      setTimeout(() => copyFeedback = false, 1500);
    } catch (e) {
      console.error('copy failed:', e);
    }
  }

  // Intercept native copy to format grid cells as tab-separated text
  // instead of the browser's default (which often collapses grid columns
  // into a single line or adds weird spacing).
  function handleCopy(e) {
    if (selectedFile) return; // raw file lines copy natively / 文件原始行走原生复制
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed) return;

    // Walk all .log-entry elements that intersect the selection range
    if (!logContainer) return;
    const entries = logContainer.querySelectorAll('.log-entry');
    const lines = [];
    for (const row of entries) {
      if (sel.containsNode(row, true)) {
        const time = row.querySelector('.log-time')?.textContent || '';
        const source = row.querySelector('.log-source')?.textContent || '';
        const level = row.querySelector('.log-level')?.textContent || '';
        const category = row.querySelector('.log-category')?.textContent || '';
        const msg = row.querySelector('.log-msg')?.textContent || '';
        lines.push(`${time}\t${source}\t${level}\t${category}\t${msg}`);
      }
    }
    if (lines.length > 0) {
      e.preventDefault();
      e.clipboardData.setData('text/plain', lines.join('\n'));
    }
  }
</script>

<div class="log-viewer">
  <div class="log-toolbar">
    <div class="log-filters">
      <button class:active={filter === 'all'} on:click={() => filter = 'all'}>
        {$t('log.filter_all')}
        <span class="filter-count">{ordered.length}</span>
      </button>
      {#each levels as lvl}
        <button class:active={filter === lvl} class="level-{lvl}" on:click={() => filter = lvl}>
          {lvl.toUpperCase()}
          <span class="filter-count">{levelCounts[lvl] || 0}</span>
        </button>
      {/each}
    </div>
    <div class="log-actions">
      <label>
        <input type="checkbox" bind:checked={autoScroll} /> {$t('log.auto_scroll')}
      </label>
      {#if folderError}
        <span class="folder-error">{$t('settings.open_folder_failed')}: {folderError}</span>
      {/if}
      <button class="btn-action" on:click={openFolder} disabled={openingFolder}>
        {$t('log.open_folder')}
      </button>
      <button class="btn-action" on:click={copyAll}>
        {copyFeedback ? '✓' : $t('log.copy')}
      </button>
      <button class="btn-action" on:click={clear}>{$t('log.clear')}</button>
    </div>
  </div>

  <div class="log-toolbar log-toolbar--categories">
    <div class="log-filters log-filters--categories">
      <button class:active={categoryFilter === 'all'} on:click={() => categoryFilter = 'all'}>
        {$t('log.filter_all')}
        <span class="filter-count">{ordered.length}</span>
      </button>
      {#each categories as cat}
        <button class:active={categoryFilter === cat} class="category-{cat}" on:click={() => categoryFilter = cat}>
          {$t(`log.category_${cat}`)}
          <span class="filter-count">{categoryCounts[cat] || 0}</span>
        </button>
      {/each}
    </div>
    <div class="log-actions">
      <select
        class="file-select"
        bind:value={selectedFile}
        on:change={(e) => selectFile(e.target.value)}
        on:focus={refreshLogFiles}
        disabled={fileLoading}
      >
        <option value="">{$t('log.file_live')}</option>
        {#each logFiles as f (f.name)}
          <option value={f.name}>{f.name}</option>
        {/each}
      </select>
      <input
        class="search-input"
        type="search"
        placeholder={$t('log.search_placeholder')}
        bind:value={search}
      />
    </div>
  </div>

  {#if fileError}
    <div class="file-error">{fileError}</div>
  {/if}

  {#if selectedFile}
    <div class="log-entries log-entries--raw" bind:this={logContainer} on:copy={handleCopy}>
      {#if fileTruncated}
        <div class="file-truncated">{$t('log.file_truncated')}</div>
      {/if}
      {#each fileFiltered as line}
        <div class="log-line">{#each highlightParts(line, search) as p}{#if p.hit}<mark>{p.s}</mark>{:else}{p.s}{/if}{/each}</div>
      {/each}
      {#if fileFiltered.length === 0}
        <div class="log-empty">{$t('log.no_entries')}</div>
      {/if}
    </div>
  {:else}
    <div class="log-entries" bind:this={logContainer} on:copy={handleCopy}>
      {#each liveFiltered as entry (entry.id)}
        <div class="log-entry level-{entry.level}">
          <span class="log-time">{formatTime(entry.time)}</span>
          <span class="log-source">{entry.source}</span>
          <span class="log-level">{entry.level.toUpperCase()}</span>
          <span class="log-category">{entry.category || 'app'}</span>
          <span class="log-msg">{#each highlightParts(entry.message, search) as p}{#if p.hit}<mark>{p.s}</mark>{:else}{p.s}{/if}{/each}</span>
        </div>
      {/each}
      {#if liveFiltered.length === 0}
        <div class="log-empty">{$t('log.no_entries')}</div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .log-viewer {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }
  .log-toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-2) var(--space-4);
    border-bottom: 0.5px solid var(--border);
    gap: var(--space-2);
  }
  .log-filters {
    display: flex;
    gap: var(--space-1);
  }
  .log-filters button {
    height: 22px;
    padding: 0 var(--space-2);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: var(--radius-xs);
    color: var(--text-secondary);
    font: 10px/1 var(--font-sans);
    font-weight: 600;
    letter-spacing: 0.04em;
    cursor: pointer;
  }
  .log-filters button:hover { background: var(--bg-hover); }
  .log-filters button.active {
    background: var(--accent);
    color: var(--text-inverse);
    border-color: transparent;
  }
  .filter-count {
    margin-left: 4px;
    font-size: 9px;
    font-weight: 500;
    opacity: 0.65;
  }
  .log-filters button.active .filter-count { opacity: 0.9; }
  .log-toolbar--categories {
    border-bottom: 0;
    padding-top: var(--space-1);
    padding-bottom: var(--space-1);
  }
  .log-filters--categories button {
    color: var(--text-tertiary);
    font-size: 9px;
    letter-spacing: 0.02em;
  }
  .log-actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    font: var(--text-footnote);
    color: var(--text-secondary);
  }
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
  .btn-action:hover { background: var(--bg-hover); }
  .btn-action:disabled { opacity: 0.6; cursor: default; }
  .folder-error {
    font: var(--text-footnote);
    color: var(--danger, #e5534b);
  }

  .search-input {
    height: 22px;
    width: 170px;
    padding: 0 var(--space-2);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: var(--radius-xs);
    color: var(--text-primary);
    font: var(--text-footnote);
    outline: none;
  }
  .search-input:focus { border-color: var(--accent); }
  .search-input::placeholder { color: var(--text-muted); }
  .file-select {
    height: 22px;
    max-width: 220px;
    padding: 0 var(--space-1);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: var(--radius-xs);
    color: var(--text-primary);
    font: var(--text-footnote);
    outline: none;
  }
  .file-select:focus { border-color: var(--accent); }

  .file-error {
    padding: var(--space-1) var(--space-4);
    font: var(--text-footnote);
    color: var(--danger, #e5534b);
  }
  .file-truncated {
    padding: var(--space-1) var(--space-2);
    font: var(--text-footnote);
    color: var(--text-muted);
    border-bottom: 0.5px solid var(--log-border);
  }

  .log-entries {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-2);
    font: 11px/1.5 var(--font-mono);
    background: var(--log-bg);
    contain: content;
    /* Ensure text is selectable — the global button reset or other rules
     * might set user-select: none. Log content MUST be selectable for
     * copy-paste into bug reports. */
    user-select: text;
    -webkit-user-select: text;
    cursor: text;
  }
  .log-entry {
    display: grid;
    grid-template-columns: 90px 55px 55px 62px minmax(0, 1fr);
    gap: var(--space-2);
    padding: 2px var(--space-2);
    border-bottom: 0.5px solid var(--log-border);
    align-items: baseline;
  }
  .log-time,
  .log-source,
  .log-level,
  .log-category,
  .log-msg { min-width: 0; }
  .log-time  { color: var(--text-muted); font-variant-numeric: tabular-nums; }
  .log-source{ color: var(--text-secondary); font-style: italic; }
  .log-level { font-weight: 700; }
  .log-category {
    color: var(--text-tertiary);
    font-size: 10px;
    text-transform: lowercase;
    letter-spacing: 0.02em;
  }
  .log-msg   {
    color: var(--text-primary);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
  /* Raw file lines: one full-width row per on-disk line, no grid columns —
   * both the GUI and the helper write here and their line shapes differ. */
  /* 原始文件行：每条磁盘行占满一行，不拆网格列——GUI 与 helper 都写这里，
   * 两者的行格式不同。 */
  .log-line {
    padding: 2px var(--space-2);
    border-bottom: 0.5px solid var(--log-border);
    color: var(--text-primary);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
  mark {
    background: var(--yellow, #d29922);
    color: #1f1f1f;
    border-radius: 2px;
    padding: 0 1px;
  }

  .level-debug .log-level { color: var(--text-muted); }
  .level-info  .log-level { color: var(--blue); }
  .level-warn  .log-level { color: var(--yellow); }
  .level-error .log-level { color: var(--red); }
  .level-error .log-msg   { color: var(--error-text); }

  .log-empty {
    padding: var(--space-8);
    text-align: center;
    color: var(--text-muted);
    font: var(--text-body);
  }
</style>
