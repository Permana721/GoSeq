// GoSeq Logseq-Style Outliner Editor
(function () {
  'use strict';

  const textarea = document.getElementById('editorTextarea');
  const slugInput = document.getElementById('slugInput');
  const sectionHidden = document.getElementById('sectionHidden');
  const slugHidden = document.getElementById('slugHidden');
  const statusIndicator = document.getElementById('statusIndicator');
  const saveBtn = document.getElementById('saveBtn');
  const copyBtn = document.getElementById('copyBtn');
  const draftAlert = document.getElementById('draftAlert');
  const restoreDraftBtn = document.getElementById('restoreDraftBtn');
  const discardDraftBtn = document.getElementById('discardDraftBtn');
  const previewContent = document.getElementById('previewContent');
  const workspace = document.getElementById('editorWorkspace');
  const modeButtons = document.querySelectorAll('.mode-toggle button');

  if (!textarea) return;

  let activeSection = (sectionHidden && sectionHidden.value) || '';
  let activeSlug = (slugHidden && slugHidden.value) || '';
  let serverContent = textarea.value;
  let isDirty = false;
  let draftDebounceTimer = null;

  // Format current time as HH:MM:SS
  function formatTime(date) {
    return date.toTimeString().split(' ')[0];
  }

  function setStatus(text, state) {
    if (!statusIndicator) return;
    statusIndicator.textContent = text;
    statusIndicator.className = 'status-indicator ' + (state || '');
  }

  // --- Draft Management (localStorage) ---
  function getDraftKey(section, slug) {
    return 'goseq_draft_' + section + '_' + slug;
  }

  function checkLocalDraft() {
    const draftKey = getDraftKey(activeSection, activeSlug);
    const draftRaw = localStorage.getItem(draftKey);
    if (!draftRaw) {
      if (draftAlert) draftAlert.style.display = 'none';
      return;
    }

    try {
      const draft = JSON.parse(draftRaw);
      if (draft && draft.content && draft.content !== serverContent) {
        if (draftAlert) {
          const draftTime = draft.updatedAt ? new Date(draft.updatedAt).toLocaleTimeString() : '';
          const infoSpan = document.getElementById('draftTimeInfo');
          if (infoSpan) infoSpan.textContent = draftTime ? ` (from ${draftTime})` : '';
          draftAlert.style.display = 'flex';
        }
      } else {
        localStorage.removeItem(draftKey);
        if (draftAlert) draftAlert.style.display = 'none';
      }
    } catch (e) {
      localStorage.removeItem(draftKey);
      if (draftAlert) draftAlert.style.display = 'none';
    }
  }

  function saveLocalDraft() {
    const draftKey = getDraftKey(activeSection, activeSlug);
    if (textarea.value !== serverContent) {
      localStorage.setItem(draftKey, JSON.stringify({
        content: textarea.value,
        updatedAt: Date.now()
      }));
    } else {
      localStorage.removeItem(draftKey);
    }
  }

  function discardLocalDraft() {
    const draftKey = getDraftKey(activeSection, activeSlug);
    localStorage.removeItem(draftKey);
    if (draftAlert) draftAlert.style.display = 'none';
    if (textarea.value !== serverContent) {
      isDirty = true;
      setStatus('Unsaved changes', 'unsaved');
    } else {
      isDirty = false;
      setStatus('Up to date', '');
    }
  }

  function restoreLocalDraft() {
    const draftKey = getDraftKey(activeSection, activeSlug);
    const draftRaw = localStorage.getItem(draftKey);
    if (draftRaw) {
      try {
        const draft = JSON.parse(draftRaw);
        if (draft && draft.content) {
          textarea.value = draft.content;
          updatePreview();
          isDirty = true;
          setStatus('Restored draft (unsaved)', 'unsaved');
          if (draftAlert) draftAlert.style.display = 'none';
        }
      } catch (e) {
        console.error('Failed to restore draft', e);
      }
    }
  }

  // --- Server Save (AJAX fetch) ---
  async function saveToServer() {
    // Sync slug from input if changed
    if (slugInput && slugInput.value.trim()) {
      activeSlug = slugInput.value.trim();
      if (slugHidden) slugHidden.value = activeSlug;
    }

    setStatus('Saving...', 'saving');
    const contentToSave = textarea.value;

    try {
      const res = await fetch('/write', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Accept': 'application/json'
        },
        body: JSON.stringify({
          section: activeSection,
          slug: activeSlug,
          content: contentToSave
        })
      });

      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText || 'Save failed');
      }

      const data = await res.json();
      serverContent = contentToSave;
      isDirty = false;

      // Clean local draft on success
      localStorage.removeItem(getDraftKey(activeSection, activeSlug));
      if (draftAlert) draftAlert.style.display = 'none';

      const savedTime = data.saved_at || formatTime(new Date());
      setStatus('Saved at ' + savedTime, '');
    } catch (err) {
      console.error('Save error:', err);
      saveLocalDraft();
      setStatus('Save failed — draft kept locally', 'failed');
    }
  }

  // --- Smart Outliner Keyboard Shortcuts ---
  textarea.addEventListener('keydown', function (e) {
    // 1. Instant Save: Ctrl+S / Cmd+S
    if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) {
      e.preventDefault();
      saveToServer();
      return;
    }

    // 2. Tab / Shift+Tab for Indentation
    if (e.key === 'Tab') {
      e.preventDefault();
      const start = this.selectionStart;
      const end = this.selectionEnd;
      const val = this.value;

      // Single line indentation
      if (start === end) {
        const lineStart = val.lastIndexOf('\n', start - 1) + 1;
        if (!e.shiftKey) {
          this.value = val.substring(0, lineStart) + '  ' + val.substring(lineStart);
          this.selectionStart = this.selectionEnd = start + 2;
        } else {
          if (val.startsWith('  ', lineStart)) {
            this.value = val.substring(0, lineStart) + val.substring(lineStart + 2);
            this.selectionStart = this.selectionEnd = Math.max(lineStart, start - 2);
          } else if (val.startsWith(' ', lineStart)) {
            this.value = val.substring(0, lineStart) + val.substring(lineStart + 1);
            this.selectionStart = this.selectionEnd = Math.max(lineStart, start - 1);
          }
        }
      } else {
        // Multi-line selection indentation
        const lineStart = val.lastIndexOf('\n', start - 1) + 1;
        let lineEnd = val.indexOf('\n', end);
        if (lineEnd === -1) lineEnd = val.length;

        const lines = val.substring(lineStart, lineEnd).split('\n');
        let deltaFirst = 0;
        let totalDelta = 0;

        const modified = lines.map((line, idx) => {
          if (!e.shiftKey) {
            totalDelta += 2;
            if (idx === 0) deltaFirst = 2;
            return '  ' + line;
          } else {
            if (line.startsWith('  ')) {
              totalDelta -= 2;
              if (idx === 0) deltaFirst = -2;
              return line.substring(2);
            } else if (line.startsWith(' ')) {
              totalDelta -= 1;
              if (idx === 0) deltaFirst = -1;
              return line.substring(1);
            }
            return line;
          }
        });

        this.value = val.substring(0, lineStart) + modified.join('\n') + val.substring(lineEnd);
        this.selectionStart = Math.max(lineStart, start + deltaFirst);
        this.selectionEnd = end + totalDelta;
      }
      onInputChanged();
      return;
    }

    // 3. Smart Enter for Outliner Bullets
    if (e.key === 'Enter') {
      const start = this.selectionStart;
      const end = this.selectionEnd;
      if (start !== end) return;

      const val = this.value;
      const lineStart = val.lastIndexOf('\n', start - 1) + 1;
      let lineEnd = val.indexOf('\n', start);
      if (lineEnd === -1) lineEnd = val.length;

      const currentLine = val.substring(lineStart, lineEnd);
      const match = currentLine.match(/^(\s*)(-\s*|\*\s*)?(.*)$/);

      if (match && match[2]) {
        const indent = match[1];
        const bullet = match[2];
        const text = match[3];

        if (text.trim() === '') {
          e.preventDefault();
          if (indent.length >= 2) {
            const newIndent = indent.substring(2);
            this.value = val.substring(0, lineStart) + newIndent + bullet + val.substring(lineEnd);
            this.selectionStart = this.selectionEnd = lineStart + newIndent.length + bullet.length;
          } else {
            this.value = val.substring(0, lineStart) + val.substring(lineEnd);
            this.selectionStart = this.selectionEnd = lineStart;
          }
          onInputChanged();
          return;
        }

        e.preventDefault();
        const insertion = '\n' + indent + bullet;
        this.value = val.substring(0, start) + insertion + val.substring(start);
        this.selectionStart = this.selectionEnd = start + insertion.length;
        onInputChanged();
        return;
      }
    }
  });

  function onInputChanged() {
    isDirty = textarea.value !== serverContent;
    if (isDirty) {
      setStatus('Unsaved changes', 'unsaved');
    }

    clearTimeout(draftDebounceTimer);
    draftDebounceTimer = setTimeout(saveLocalDraft, 400);

    updatePreview();
  }

  textarea.addEventListener('input', onInputChanged);

  // --- View Mode Toggling (Edit / Split / Preview) ---
  modeButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      modeButtons.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      const mode = btn.dataset.mode;

      workspace.className = 'editor-workspace mode-' + mode;
      const editorPane = document.getElementById('editorPane');
      const previewPane = document.getElementById('previewPane');

      if (mode === 'edit') {
        editorPane.style.display = 'flex';
        previewPane.style.display = 'none';
      } else if (mode === 'preview') {
        editorPane.style.display = 'none';
        previewPane.style.display = 'flex';
        updatePreview();
      } else {
        editorPane.style.display = 'flex';
        previewPane.style.display = 'flex';
        updatePreview();
      }
    });
  });

  // --- Copy Raw Markdown ---
  if (copyBtn) {
    copyBtn.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(textarea.value);
        const origText = copyBtn.innerHTML;
        copyBtn.innerHTML = 'Copied!';
        setTimeout(() => {
          copyBtn.innerHTML = origText;
        }, 1500);
      } catch (err) {
        alert('Failed to copy to clipboard');
      }
    });
  }

  if (saveBtn) {
    saveBtn.addEventListener('click', (e) => {
      e.preventDefault();
      saveToServer();
    });
  }

  if (restoreDraftBtn) restoreDraftBtn.addEventListener('click', restoreLocalDraft);
  if (discardDraftBtn) discardDraftBtn.addEventListener('click', discardLocalDraft);

  // Before unload warning if unsaved
  window.addEventListener('beforeunload', (e) => {
    if (isDirty) {
      e.preventDefault();
      e.returnValue = '';
    }
  });

  // --- Client-Side Live Outliner Renderer for Preview ---
  function escapeHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  function renderInline(text) {
    let t = escapeHTML(text);

    // Images
    t = t.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1" class="outliner-img" loading="lazy" />');
    // Markdown links
    t = t.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer" class="ext-link">$1</a>');
    // Multi-word tags
    t = t.replace(/(?:^|\s)#\[\[([^\]]+)\]\]/g, ' <a href="/?tag=$1" class="tag-badge">#$1</a>');
    // Tags
    t = t.replace(/(?:^|\s)#([a-zA-Z0-9_\-\/]+)/g, ' <a href="/?tag=$1" class="tag-badge">#$1</a>');
    // WikiLinks
    t = t.replace(/\[\[([^\]]+)\]\]/g, '<a href="/?link=$1" class="wikilink-badge"><span class="wl-brackets">[[</span>$1<span class="wl-brackets">]]</span></a>');
    // Bold, italic, strike, code
    t = t.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    t = t.replace(/\*([^*]+)\*/g, '<em>$1</em>');
    t = t.replace(/~~([^~]+)~~/g, '<del>$1</del>');
    t = t.replace(/`([^`]+)`/g, '<code>$1</code>');
    t = t.replace(/\n/g, '<br>');

    return t;
  }

  function renderClientOutliner(raw) {
    if (!raw.trim()) {
      return '<div class="empty-state"><p>Nothing written yet.</p></div>';
    }

    const lines = raw.replace(/\r\n/g, '\n').split('\n');
    const blocks = [];
    let currentBlock = null;
    let inCodeFence = false;
    let codeFenceLines = [];
    let codeFenceLang = '';

    for (const line of lines) {
      const trimmed = line.trim();

      if (inCodeFence) {
        if (trimmed.startsWith('```')) {
          inCodeFence = false;
          codeFenceLines.push('```');
          const codeStr = codeFenceLines.join('\n');
          if (currentBlock) {
            currentBlock.raw += '\n' + codeStr;
          } else {
            currentBlock = { level: 0, raw: codeStr };
            blocks.push(currentBlock);
          }
          codeFenceLines = [];
          continue;
        }
        codeFenceLines.push(line);
        continue;
      }

      if (trimmed.startsWith('```')) {
        inCodeFence = true;
        codeFenceLang = trimmed.substring(3).trim();
        codeFenceLines = ['```' + codeFenceLang];
        continue;
      }

      if (trimmed === '') continue;

      const leadingSpaces = line.length - line.trimStart().length;
      const lineAfterIndent = line.substring(leadingSpaces);

      if (lineAfterIndent.startsWith('- ') || lineAfterIndent.startsWith('* ')) {
        const text = lineAfterIndent.substring(2).trim();
        const level = Math.floor(leadingSpaces / 2);
        currentBlock = { level, raw: text };
        blocks.push(currentBlock);
      } else {
        if (currentBlock) {
          currentBlock.raw += '\n' + lineAfterIndent;
        } else {
          currentBlock = { level: Math.floor(leadingSpaces / 2), raw: lineAfterIndent };
          blocks.push(currentBlock);
        }
      }
    }

    if (inCodeFence && codeFenceLines.length > 0) {
      codeFenceLines.push('```');
      const codeStr = codeFenceLines.join('\n');
      if (currentBlock) currentBlock.raw += '\n' + codeStr;
      else blocks.push({ level: 0, raw: codeStr });
    }

    // Build tree
    const roots = [];
    const stack = [];

    for (const blk of blocks) {
      blk.children = [];

      const codeBlocks = [];
      let text = blk.raw.replace(/```([a-zA-Z0-9_-]*)\n([\s\S]*?)```/g, (match, lang, body) => {
        const idx = codeBlocks.length;
        const langClass = lang ? ` class="language-${escapeHTML(lang)}"` : '';
        codeBlocks.push(`<pre class="code-fence"><code${langClass}>${escapeHTML(body)}</code></pre>`);
        return `___CODE_BLOCK_${idx}___`;
      });

      let isTodo = false;
      let isDone = false;
      const trimmed = text.trim();
      if (trimmed.startsWith('TODO ') || trimmed === 'TODO') {
        isTodo = true;
        text = trimmed.substring(4).trim();
      } else if (trimmed.startsWith('DONE ') || trimmed === 'DONE') {
        isDone = true;
        text = trimmed.substring(4).trim();
      }

      let rendered = renderInline(text);

      codeBlocks.forEach((cb, idx) => {
        rendered = rendered.replace(`___CODE_BLOCK_${idx}___`, cb);
      });

      let badge = '';
      if (isTodo) {
        badge = '<span class="task-badge task-todo"><span class="task-checkbox"></span>TODO</span> ';
      } else if (isDone) {
        badge = '<span class="task-badge task-done"><span class="task-checkbox checked">✓</span>DONE</span> ';
      }
      blk.html = badge + rendered;

      while (stack.length > 0 && stack[stack.length - 1].level >= blk.level) {
        stack.pop();
      }
      if (stack.length === 0) {
        roots.push(blk);
      } else {
        stack[stack.length - 1].block.children.push(blk);
      }
      stack.push({ block: blk, level: blk.level });
    }

    function renderNode(node) {
      let html = '<li class="outliner-item">';
      html += '<div class="block-row">';
      html += '<span class="bullet" aria-hidden="true"></span>';
      html += '<div class="block-content">' + node.html + '</div>';
      html += '</div>';
      if (node.children && node.children.length > 0) {
        html += '<ul class="outliner-children">';
        for (const child of node.children) {
          html += renderNode(child);
        }
        html += '</ul>';
      }
      html += '</li>';
      return html;
    }

    let out = '<ul class="outliner-root">';
    for (const root of roots) {
      out += renderNode(root);
    }
    out += '</ul>';
    return out;
  }

  function updatePreview() {
    if (!previewContent) return;
    previewContent.innerHTML = renderClientOutliner(textarea.value);
  }

  // Initial check on load
  checkLocalDraft();
  updatePreview();
})();
