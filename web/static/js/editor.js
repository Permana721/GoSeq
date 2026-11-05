(function () {
  'use strict';

  const textarea = document.getElementById('editor') || document.getElementById('editorTextarea') || document.querySelector('textarea');
  const slugInput = document.getElementById('slugInput');
  const sectionHidden = document.getElementById('sectionHidden');
  const slugHidden = document.getElementById('slugHidden');
  const langSelect = document.getElementById('langSelect');
  const langToggleBtn = document.getElementById('langToggleBtn');
  const categoryInput = document.getElementById('categoryInput');
  const newEntryBtn = document.getElementById('newEntryBtn');
  const deleteBtn = document.getElementById('deleteBtn');
  const statusIndicator = document.getElementById('statusIndicator');
  const saveBtn = document.getElementById('saveBtn');
  const copyBtn = document.getElementById('copyBtn');
  const draftAlert = document.getElementById('draftAlert');
  const restoreDraftBtn = document.getElementById('restoreDraftBtn');
  const discardDraftBtn = document.getElementById('discardDraftBtn');
  const previewContent = document.getElementById('previewContent');
  const workspace = document.getElementById('editorWorkspace');
  const modeButtons = document.querySelectorAll('.mode-toggle button');
  const formatToolbar = document.getElementById('formatToolbar');
  const imageFileInput = document.getElementById('imageFileInput');
  const editorBox = document.querySelector('.editor-box');
  const sectionSelect = document.getElementById('sectionSelect');
  const sectionInput = document.getElementById('sectionInput');

  if (!textarea) return;

  let activeSection = (sectionHidden && sectionHidden.value) || '';
  let activeSlug = (slugHidden && slugHidden.value) || '';
  let serverContent = textarea.value;
  let isDirty = false;
  let draftDebounceTimer = null;

  function formatTime(date) {
    return date.toTimeString().split(' ')[0];
  }

  function setStatus(text, state) {
    if (!statusIndicator) return;
    statusIndicator.textContent = text;
    statusIndicator.className = 'status-indicator ' + (state || '');
  }

  function parseLangFromContent(content) {
    if (!content) return null;
    const normalized = content.replace(/\r\n/g, '\n');
    const lines = normalized.split('\n');

    if (lines.length > 0 && lines[0].trim() === '---') {
      for (let i = 1; i < lines.length; i++) {
        if (lines[i].trim() === '---') break;
        const m = lines[i].match(/^[Ll]ang\s*:\s*(.+)$/i);
        if (m) {
          const val = m[1].trim().toUpperCase();
          if (val === 'ID' || val.startsWith('ID')) return 'ID';
          if (val === 'EN' || val.startsWith('EN')) return 'EN';
          return val;
        }
      }
    }

    const limit = Math.min(lines.length, 20);
    for (let i = 0; i < limit; i++) {
      const m = lines[i].match(/^\s*(?:[-*]\s*)?lang::\s*(.+)$/i);
      if (m) {
        const val = m[1].trim().toUpperCase();
        if (val === 'ID' || val.startsWith('ID')) return 'ID';
        if (val === 'EN' || val.startsWith('EN')) return 'EN';
        return val;
      }
    }

    return null;
  }

  function setLangInContent(content, lang) {
    const normalized = (content || '').replace(/\r\n/g, '\n');
    const lines = normalized.split('\n');

    if (lines.length > 0 && lines[0].trim() === '---') {
      let closeIdx = -1;
      for (let i = 1; i < lines.length; i++) {
        if (lines[i].trim() === '---') { closeIdx = i; break; }
      }
      if (closeIdx !== -1) {
        let langLineIdx = -1;
        for (let i = 1; i < closeIdx; i++) {
          if (/^[Ll]ang\s*:/i.test(lines[i])) { langLineIdx = i; break; }
        }
        if (langLineIdx !== -1) {
          lines[langLineIdx] = 'Lang: ' + lang;
        } else {
          lines.splice(closeIdx, 0, 'Lang: ' + lang);
        }
        return lines.join('\n');
      }
    }

    const limit = Math.min(lines.length, 20);
    let replaced = false;
    const filteredLines = [];
    for (let i = 0; i < lines.length; i++) {
      if (i < limit && /^\s*(?:[-*]\s*)?lang::/i.test(lines[i])) {
        if (!replaced) {
          filteredLines.push('- lang:: ' + lang);
          replaced = true;
        }
        continue;
      }
      filteredLines.push(lines[i]);
    }
    if (replaced) {
      return filteredLines.join('\n');
    }

    return '- lang:: ' + lang + '\n' + normalized;
  }

  function getCurrentLang() {
    if (langSelect && (langSelect.value === 'ID' || langSelect.value === 'EN')) {
      return langSelect.value;
    }
    if (langToggleBtn) {
      const text = langToggleBtn.textContent.trim().toUpperCase();
      if (text === 'ID' || text === 'EN') return text;
    }
    return 'EN';
  }

  function syncUIState(lang) {
    const normalized = (lang === 'ID') ? 'ID' : 'EN';
    if (langSelect && langSelect.value !== normalized) {
      langSelect.value = normalized;
    }
    if (langToggleBtn) {
      langToggleBtn.textContent = normalized;
      if (normalized === 'ID') {
        langToggleBtn.classList.add('lang-active-id');
      } else {
        langToggleBtn.classList.remove('lang-active-id');
      }
    }
  }

  function triggerButtonFlash() {
    if (!langToggleBtn) return;
    langToggleBtn.classList.remove('flashing');
    void langToggleBtn.offsetWidth;
    langToggleBtn.classList.add('flashing');
    langToggleBtn.addEventListener('animationend', () => {
      langToggleBtn.classList.remove('flashing');
    }, { once: true });
  }

  function updateLangInEditor(lang) {
    if (!textarea) return;
    const oldContent = textarea.value;
    const newContent = setLangInContent(oldContent, lang);
    if (newContent !== oldContent) {
      const selStart = textarea.selectionStart;
      const selEnd = textarea.selectionEnd;
      const delta = newContent.length - oldContent.length;

      textarea.value = newContent;

      const adjustedStart = Math.min(newContent.length, Math.max(0, selStart + (selStart > 0 ? delta : 0)));
      const adjustedEnd = Math.min(newContent.length, Math.max(0, selEnd + (selEnd > 0 ? delta : 0)));
      textarea.setSelectionRange(adjustedStart, adjustedEnd);

      textarea.dispatchEvent(new Event('input', { bubbles: true }));
    }
  }

  function setLanguage(lang, { flash = true, updateContent = true } = {}) {
    const normalized = (lang === 'ID') ? 'ID' : 'EN';
    syncUIState(normalized);
    if (updateContent) {
      updateLangInEditor(normalized);
    }
    if (flash) {
      triggerButtonFlash();
    }
  }

  function toggleLang() {
    const current = getCurrentLang();
    const next = (current === 'EN') ? 'ID' : 'EN';
    setLanguage(next, { flash: true, updateContent: true });
  }

  function syncLangFromContent() {
    const detected = parseLangFromContent(textarea.value);
    if (detected === 'ID' || detected === 'EN') {
      syncUIState(detected);
    } else {
      syncUIState(getCurrentLang());
    }
  }

  function syncLangSelectFromContent() { syncLangFromContent(); }
  function syncLangToggleBtn() { syncUIState(getCurrentLang()); }

  function parseCategoryFromContent(content) {
    if (!content) return '';
    const normalized = content.replace(/\r\n/g, '\n');
    const lines = normalized.split('\n');

    if (lines.length > 0 && lines[0].trim() === '---') {
      for (let i = 1; i < lines.length; i++) {
        if (lines[i].trim() === '---') break;
        const m = lines[i].match(/^(?:Category|Topic)\s*:\s*(.+)$/i);
        if (m) return m[1].trim();
      }
    }

    const limit = Math.min(lines.length, 20);
    for (let i = 0; i < limit; i++) {
      const m = lines[i].match(/^\s*(?:[-*]\s*)?(?:category|topic)::\s*(.+)$/i);
      if (m) return m[1].trim();
    }

    return '';
  }

  function setCategoryInContent(content, category) {
    const cat = (category || '').trim();
    const normalized = (content || '').replace(/\r\n/g, '\n');
    const lines = normalized.split('\n');

    if (lines.length > 0 && lines[0].trim() === '---') {
      let closeIdx = -1;
      for (let i = 1; i < lines.length; i++) {
        if (lines[i].trim() === '---') { closeIdx = i; break; }
      }
      if (closeIdx !== -1) {
        let catLineIdx = -1;
        for (let i = 1; i < closeIdx; i++) {
          if (/^(?:Category|Topic)\s*:/i.test(lines[i])) { catLineIdx = i; break; }
        }
        if (cat) {
          if (catLineIdx !== -1) {
            lines[catLineIdx] = 'Category: ' + cat;
          } else {
            lines.splice(closeIdx, 0, 'Category: ' + cat);
          }
        } else if (catLineIdx !== -1) {
          lines.splice(catLineIdx, 1);
        }
        return lines.join('\n');
      }
    }

    const limit = Math.min(lines.length, 20);
    let replaced = false;
    const filteredLines = [];
    for (let i = 0; i < lines.length; i++) {
      if (i < limit && /^\s*(?:[-*]\s*)?(?:category|topic)::/i.test(lines[i])) {
        if (cat && !replaced) {
          filteredLines.push('- category:: ' + cat);
          replaced = true;
        }
        continue;
      }
      filteredLines.push(lines[i]);
    }
    if (replaced || (!cat && filteredLines.length !== lines.length)) {
      return filteredLines.join('\n');
    }

    if (!cat) {
      return normalized;
    }

    let langIdx = -1;
    for (let i = 0; i < Math.min(filteredLines.length, 20); i++) {
      if (/^\s*(?:[-*]\s*)?lang::/i.test(filteredLines[i])) {
        langIdx = i;
        break;
      }
    }

    if (langIdx !== -1) {
      filteredLines.splice(langIdx + 1, 0, '- category:: ' + cat);
      return filteredLines.join('\n');
    }

    return '- category:: ' + cat + '\n' + normalized;
  }

  function syncCategoryFromContent() {
    if (!categoryInput) return;
    const detected = parseCategoryFromContent(textarea.value);
    if (detected) {
      categoryInput.value = detected;
    }
  }

  function updateArticleDropdown(articles, currentSlug) {
    let dropdown = document.getElementById('articleDropdown');
    if (!dropdown) {
      dropdown = document.createElement('select');
      dropdown.id = 'articleDropdown';
      dropdown.className = 'date-input';
      dropdown.style.maxWidth = '160px';
      dropdown.addEventListener('change', () => {
        if (dropdown.value) {
          window.location.href = `/write?section=${encodeURIComponent(activeSection)}&slug=${encodeURIComponent(dropdown.value)}`;
        }
      });
      if (slugInput && slugInput.parentNode) {
        slugInput.parentNode.insertBefore(dropdown, slugInput.nextSibling);
      }
    }
    if (dropdown) {
      dropdown.innerHTML = '<option value="">Jump to article...</option>' +
        (articles || []).map(s => `<option value="${escapeHTML(s)}"${s === currentSlug ? ' selected' : ''}>${escapeHTML(s)}</option>`).join('');
    }
  }

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
          syncLangFromContent();
          isDirty = true;
          setStatus('Restored draft (unsaved)', 'unsaved');
          if (draftAlert) draftAlert.style.display = 'none';
        }
      } catch (e) {
        console.error('Failed to restore draft', e);
      }
    }
  }

  async function saveToServer() {
    const selStart = textarea.selectionStart;
    const selEnd = textarea.selectionEnd;

    if (slugInput && slugInput.value.trim()) {
      activeSlug = slugInput.value.trim();
      if (slugHidden) slugHidden.value = activeSlug;
    }

    setStatus('Saving...', 'saving');

    const currentLang = getCurrentLang();
    let contentToSave = setLangInContent(textarea.value, currentLang);
    const currentCategory = categoryInput ? categoryInput.value.trim() : '';
    contentToSave = setCategoryInContent(contentToSave, currentCategory);

    if (contentToSave !== textarea.value) {
      textarea.value = contentToSave;
    }
    syncUIState(currentLang);

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
      
      if (data.content && data.content !== textarea.value) {
        textarea.value = data.content;
        textarea.setSelectionRange(selStart, selEnd);
      }
      serverContent = textarea.value;
      isDirty = false;

      localStorage.removeItem(getDraftKey(activeSection, activeSlug));
      if (draftAlert) draftAlert.style.display = 'none';

      if (data.articles) {
        updateArticleDropdown(data.articles, activeSlug);
      }

      const newURL = `/write?section=${encodeURIComponent(activeSection)}&slug=${encodeURIComponent(activeSlug)}`;
      window.history.replaceState(null, '', newURL);

      const viewLink = document.getElementById('viewPublicLink');
      if (viewLink) {
        viewLink.href = `/read/${encodeURIComponent(activeSection)}/${encodeURIComponent(activeSlug)}`;
        viewLink.style.display = '';
      }

      const savedTime = data.saved_at || formatTime(new Date());
      setStatus('Saved at ' + savedTime, '');
    } catch (err) {
      console.error('Save error:', err);
      saveLocalDraft();
      setStatus('Save failed — draft kept locally', 'failed');
    }
  }

  function replaceSelectionWithHistory(newText, cursorOffsetStart, cursorOffsetEnd) {
    textarea.focus();
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;

    let success = false;
    try {
      success = document.execCommand('insertText', false, newText);
    } catch (e) {
      success = false;
    }

    if (!success) {
      textarea.setRangeText(newText, start, end, 'end');
    }

    const finalSelStart = typeof cursorOffsetStart === 'number' ? start + cursorOffsetStart : start + newText.length;
    const finalSelEnd = typeof cursorOffsetEnd === 'number' ? start + cursorOffsetEnd : finalSelStart;

    textarea.setSelectionRange(finalSelStart, finalSelEnd);
    textarea.focus();
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
  }

  function wrapSelection(prefix, suffix = prefix, placeholder = "text") {
    if (!textarea) return;
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const selected = textarea.value.substring(start, end);
    const textToInsert = selected.length > 0 ? selected : placeholder;
    const replacement = prefix + textToInsert + suffix;

    textarea.setRangeText(replacement, start, end, 'select');
    textarea.focus();
    if (selected.length === 0) {
      textarea.setSelectionRange(start + prefix.length, start + prefix.length + placeholder.length);
    }
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
  }

  function toggleLineMarker(marker) {
    if (!textarea) return;
    const start = textarea.selectionStart;
    const val = textarea.value;
    const lineStart = val.lastIndexOf('\n', start - 1) + 1;
    let lineEnd = val.indexOf('\n', start);
    if (lineEnd === -1) lineEnd = val.length;
    
    let line = val.substring(lineStart, lineEnd);
    const bulletMatch = line.match(/^(\s*-\s*)(.*)$/);
    if (bulletMatch) {
      const prefix = bulletMatch[1];
      let rest = bulletMatch[2];
      if (rest.startsWith(marker + ' ')) {
        rest = rest.substring(marker.length + 1);
      } else {
        rest = rest.replace(/^(TODO|DONE)\s+/, '');
        rest = marker + ' ' + rest;
      }
      const newLine = prefix + rest;
      textarea.value = val.substring(0, lineStart) + newLine + val.substring(lineEnd);
      textarea.focus();
      textarea.setSelectionRange(lineStart + newLine.length, lineStart + newLine.length);
      textarea.dispatchEvent(new Event('input', { bubbles: true }));
    }
  }

  function formatBold() {
    wrapSelection('**', '**', 'bold text');
  }

  function formatItalic() {
    wrapSelection('*', '*', 'italic text');
  }

  function formatCode() {
    wrapSelection('`', '`', 'code');
  }

  function formatCodeBlock() {
    wrapSelection('```\n', '\n```', 'code block');
  }

  function formatWikiLink() {
    wrapSelection('[[', ']]', 'Page');
  }

  function formatTag() {
    wrapSelection('#', '', 'tag');
  }

  function toggleTodoDone(marker = 'TODO') {
    toggleLineMarker(marker);
  }

  function indentSelection(isOutdent) {
    textarea.focus();
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const val = textarea.value;

    const lineStart = val.lastIndexOf('\n', start - 1) + 1;
    let lineEnd = val.indexOf('\n', end);
    if (lineEnd === -1) lineEnd = val.length;

    if (start === end && !val.substring(lineStart, lineEnd).includes('\n')) {
      const currentLine = val.substring(lineStart, lineEnd);
      if (!isOutdent) {
        const newLine = '  ' + currentLine;
        textarea.setSelectionRange(lineStart, lineEnd);
        replaceSelectionWithHistory(newLine, start - lineStart + 2, start - lineStart + 2);
      } else {
        let removeCount = 0;
        if (currentLine.startsWith('  ')) removeCount = 2;
        else if (currentLine.startsWith(' ')) removeCount = 1;

        if (removeCount > 0) {
          const newLine = currentLine.substring(removeCount);
          textarea.setSelectionRange(lineStart, lineEnd);
          const newPos = Math.max(lineStart, start - removeCount);
          replaceSelectionWithHistory(newLine, newPos - lineStart, newPos - lineStart);
        }
      }
    } else {
      const lines = val.substring(lineStart, lineEnd).split('\n');
      let deltaFirst = 0;
      let totalDelta = 0;

      const modified = lines.map((line, idx) => {
        if (!isOutdent) {
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

      const newText = modified.join('\n');
      textarea.setSelectionRange(lineStart, lineEnd);
      const newStart = Math.max(lineStart, start + deltaFirst);
      const newEnd = end + totalDelta;
      replaceSelectionWithHistory(newText, newStart - lineStart, newEnd - lineStart);
    }
  }

  textarea.addEventListener('keydown', function (e) {
    if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) {
      e.preventDefault();
      saveToServer();
      return;
    }

    if ((e.ctrlKey || e.metaKey) && (e.key === 'b' || e.key === 'B')) {
      e.preventDefault();
      formatBold();
      return;
    }

    if ((e.ctrlKey || e.metaKey) && (e.key === 'i' || e.key === 'I')) {
      e.preventDefault();
      formatItalic();
      return;
    }

    if ((e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault();
      formatWikiLink();
      return;
    }

    if ((e.ctrlKey || e.metaKey) && (e.key === 'l' || e.key === 'L')) {
      e.preventDefault();
      toggleLang();
      return;
    }

    if (e.key === 'Tab') {
      e.preventDefault();
      indentSelection(e.shiftKey);
      return;
    }

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

    const detected = parseLangFromContent(textarea.value);
    if (detected === 'EN' || detected === 'ID') {
      syncUIState(detected);
    }

    clearTimeout(draftDebounceTimer);
    draftDebounceTimer = setTimeout(saveLocalDraft, 400);

    updatePreview();
  }

  textarea.addEventListener('input', onInputChanged);

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

  if (deleteBtn) {
    deleteBtn.addEventListener('click', async (e) => {
      e.preventDefault();
      if (!activeSection || !activeSlug) {
        alert('No active article to delete.');
        return;
      }

      const confirmed = window.confirm('Are you sure you want to delete this entry? This action cannot be undone.');
      if (!confirmed) return;

      setStatus('Deleting...', 'saving');

      try {
        const res = await fetch('/delete', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Accept': 'application/json'
          },
          body: JSON.stringify({
            section: activeSection,
            slug: activeSlug
          })
        });

        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || 'Failed to delete entry');
        }

        const data = await res.json();
        localStorage.removeItem(getDraftKey(activeSection, activeSlug));
        isDirty = false;

        if (data.redirect) {
          window.location.href = data.redirect;
        } else {
          window.location.href = `/write?section=${encodeURIComponent(activeSection)}&deleted=1`;
        }
      } catch (err) {
        console.error('Delete error:', err);
        setStatus('Delete failed', 'failed');
        alert('Failed to delete entry: ' + err.message);
      }
    });
  }

  if (newEntryBtn) {
    newEntryBtn.addEventListener('click', () => {
      if (isDirty) {
        if (!confirm('You have unsaved changes. Discard and create a new entry?')) {
          return;
        }
      }

      const now = new Date();
      const y = now.getFullYear();
      const m = String(now.getMonth() + 1).padStart(2, '0');
      const d = String(now.getDate()).padStart(2, '0');
      const todaySlug = `${y}_${m}_${d}`;

      let suggested = todaySlug;
      const dropdown = document.getElementById('articleDropdown');
      const existingSlugs = new Set();
      if (dropdown) {
        for (let i = 0; i < dropdown.options.length; i++) {
          if (dropdown.options[i].value) {
            existingSlugs.add(dropdown.options[i].value);
          }
        }
      }

      if (existingSlugs.has(suggested)) {
        let counter = 1;
        while (existingSlugs.has(`${todaySlug}_${counter}`)) {
          counter++;
        }
        suggested = `${todaySlug}_${counter}`;
      }

      if (slugInput) {
        slugInput.value = suggested;
        slugInput.focus();
        slugInput.select();
      }
      activeSlug = suggested;
      if (slugHidden) slugHidden.value = activeSlug;

      if (categoryInput) {
        categoryInput.value = '';
      }

      textarea.value = '- ';
      serverContent = '';
      isDirty = true;

      if (dropdown) {
        dropdown.value = '';
      }

      localStorage.removeItem(getDraftKey(activeSection, suggested));
      if (draftAlert) draftAlert.style.display = 'none';

      updatePreview();
      setStatus('New entry (unsaved)', 'unsaved');

      const viewLink = document.getElementById('viewPublicLink');
      if (viewLink) {
        viewLink.href = `/read/${encodeURIComponent(activeSection)}/${encodeURIComponent(suggested)}`;
        viewLink.style.display = '';
      }
    });
  }

  if (slugInput) {
    slugInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        saveToServer();
      }
    });
  }

  if (categoryInput) {
    categoryInput.addEventListener('input', () => {
      isDirty = true;
      setStatus('Unsaved changes', 'unsaved');
    });
  }

  if (restoreDraftBtn) restoreDraftBtn.addEventListener('click', restoreLocalDraft);
  if (discardDraftBtn) discardDraftBtn.addEventListener('click', discardLocalDraft);

  window.addEventListener('beforeunload', (e) => {
    if (isDirty) {
      e.preventDefault();
      e.returnValue = '';
    }
  });

  function escapeHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  function renderInline(text) {
    let t = escapeHTML(text);

    if (t.startsWith('# ')) {
      t = '<span class="outliner-h1">' + t.substring(2) + '</span>';
    } else if (t.startsWith('## ')) {
      t = '<span class="outliner-h2">' + t.substring(3) + '</span>';
    } else if (t.startsWith('### ')) {
      t = '<span class="outliner-h3">' + t.substring(4) + '</span>';
    }

    if (t.startsWith('Date:: ')) {
      t = '<span class="outliner-prop"><span class="prop-key">Date::</span> <span class="prop-val">' + t.substring(7) + '</span></span>';
    }

    t = t.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1" class="outliner-img" loading="lazy" />');
    t = t.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer" class="ext-link">$1</a>');
    t = t.replace(/(?:^|\s)#\[\[([^\]]+)\]\]/g, ' <a href="/?tag=$1" class="tag-badge">#$1</a>');
    t = t.replace(/(?:^|\s)#([a-zA-Z0-9_\-\/]+)/g, ' <a href="/?tag=$1" class="tag-badge">#$1</a>');
    t = t.replace(/\[\[([^\]]+)\]\]/g, '<a href="/?link=$1" class="wikilink-badge"><span class="wl-brackets">[[</span>$1<span class="wl-brackets">]]</span></a>');
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

      if (/^\s*(?:[-*]\s*)?(?:lang|category|topic)::/i.test(trimmed)) {
        currentBlock = null;
        continue;
      }

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

  function renderInlineProse(rawText) {
    let t = escapeHTML(rawText);
    t = t.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, function(match, alt, url) {
      const caption = alt.trim() ? '<figcaption class="prose-caption">' + alt + '</figcaption>' : '';
      return '<figure class="prose-figure"><img src="' + url + '" alt="' + alt + '" class="prose-img" loading="lazy" />' + caption + '</figure>';
    });
    t = t.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer" class="ext-link">$1</a>');
    t = t.replace(/(?:^|\s)#\[\[([^\]]+)\]\]/g, ' <a href="/?tag=$1" class="tag-badge">#$1</a>');
    t = t.replace(/(?:^|\s)#([a-zA-Z0-9_\-\/]+)/g, ' <a href="/?tag=$1" class="tag-badge">#$1</a>');
    t = t.replace(/\[\[([^\]]+)\]\]/g, '<a href="/?link=$1" class="wikilink-badge"><span class="wl-brackets">[[</span>$1<span class="wl-brackets">]]</span></a>');
    t = t.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    t = t.replace(/\*([^*]+)\*/g, '<em>$1</em>');
    t = t.replace(/~~([^~]+)~~/g, '<del>$1</del>');
    t = t.replace(/`([^`]+)`/g, '<code>$1</code>');
    return t;
  }

  function renderClientArticleProse(raw) {
    if (!raw.trim()) {
      return '<div class="empty-state"><p>Nothing written yet.</p></div>';
    }

    const lines = raw.replace(/\r\n/g, '\n').split('\n');
    let out = '';
    let inCodeFence = false;
    let codeFenceLines = [];
    let codeFenceLang = '';

    let paragraphLines = [];
    let quoteLines = [];
    let listItems = [];
    let inList = false;
    let isOrderedList = false;

    function flushParagraph() {
      if (paragraphLines.length === 0) return;
      const joined = paragraphLines.join(' ');
      const formatted = renderInlineProse(joined);
      const trimmed = formatted.trim();
      if (trimmed.startsWith('<figure') && trimmed.endsWith('</figure>')) {
        out += trimmed + '\n';
      } else if (trimmed !== '') {
        out += '<p class="prose-p">' + trimmed + '</p>\n';
      }
      paragraphLines = [];
    }

    function flushList() {
      if (listItems.length === 0) {
        inList = false;
        return;
      }
      out += isOrderedList ? '<ol class="prose-ol">\n' : '<ul class="prose-ul">\n';
      for (const item of listItems) {
        out += '  <li>' + renderInlineProse(item) + '</li>\n';
      }
      out += isOrderedList ? '</ol>\n' : '</ul>\n';
      listItems = [];
      inList = false;
      isOrderedList = false;
    }

    function flushQuote() {
      if (quoteLines.length === 0) return;
      out += '<blockquote class="prose-quote">\n';
      for (const ql of quoteLines) {
        out += '  <p>' + renderInlineProse(ql) + '</p>\n';
      }
      out += '</blockquote>\n';
      quoteLines = [];
    }

    function flushAll() {
      flushParagraph();
      flushList();
      flushQuote();
    }

    for (const line of lines) {
      const trimmed = line.trim();

      if (inCodeFence) {
        if (trimmed.startsWith('```')) {
          inCodeFence = false;
          const body = codeFenceLines.join('\n');
          const langClass = codeFenceLang ? ' class="language-' + escapeHTML(codeFenceLang) + '"' : '';
          out += '<div class="code-block-wrapper"><pre class="code-fence"><code' + langClass + '>' + escapeHTML(body) + '</code></pre><button type="button" class="copy-code-btn" aria-label="Copy code">Copy</button></div>\n';
          codeFenceLines = [];
          codeFenceLang = '';
          continue;
        }
        codeFenceLines.push(line);
        continue;
      }

      if (trimmed.startsWith('```')) {
        flushAll();
        inCodeFence = true;
        codeFenceLang = trimmed.substring(3).trim();
        codeFenceLines = [];
        continue;
      }

      if (/^\s*(?:[-*]\s*)?(?:lang|category|topic)::/i.test(trimmed)) {
        continue;
      }

      if (trimmed === '') {
        flushAll();
        continue;
      }

      if (/^(?:---|\*\*\*|___)\s*$/.test(trimmed)) {
        flushAll();
        out += '<hr class="prose-hr">\n';
        continue;
      }

      if (trimmed.startsWith('#### ')) {
        flushAll();
        out += '<h4 class="prose-h4">' + renderInlineProse(trimmed.substring(5)) + '</h4>\n';
        continue;
      } else if (trimmed.startsWith('### ')) {
        flushAll();
        out += '<h3 class="prose-h3">' + renderInlineProse(trimmed.substring(4)) + '</h3>\n';
        continue;
      } else if (trimmed.startsWith('## ')) {
        flushAll();
        out += '<h2 class="prose-h2">' + renderInlineProse(trimmed.substring(3)) + '</h2>\n';
        continue;
      } else if (trimmed.startsWith('# ')) {
        flushAll();
        out += '<h2 class="prose-h2">' + renderInlineProse(trimmed.substring(2)) + '</h2>\n';
        continue;
      }

      if (trimmed.startsWith('>')) {
        flushParagraph();
        flushList();
        let q = trimmed.substring(1);
        if (q.startsWith(' ')) q = q.substring(1);
        quoteLines.push(q);
        continue;
      }

      const orderedMatch = trimmed.match(/^(\d+)\.\s+(.*)$/);
      if (orderedMatch) {
        flushParagraph();
        flushQuote();
        if (inList && !isOrderedList) flushList();
        inList = true;
        isOrderedList = true;
        listItems.push(orderedMatch[2]);
        continue;
      }

      const leadingSpaces = line.length - line.trimStart().length;
      const lineAfterIndent = line.substring(leadingSpaces);
      if (lineAfterIndent.startsWith('- ') || lineAfterIndent.startsWith('* ')) {
        flushParagraph();
        flushQuote();
        if (inList && isOrderedList) flushList();
        inList = true;
        isOrderedList = false;
        listItems.push(lineAfterIndent.substring(2).trim());
        continue;
      }

      if (/^!\[([^\]]*)\]\(([^)]+)\)$/.test(trimmed)) {
        flushAll();
        out += renderInlineProse(trimmed) + '\n';
        continue;
      }

      flushList();
      flushQuote();
      paragraphLines.push(trimmed);
    }

    if (inCodeFence) {
      const body = codeFenceLines.join('\n');
      const langClass = codeFenceLang ? ' class="language-' + escapeHTML(codeFenceLang) + '"' : '';
      out += '<div class="code-block-wrapper"><pre class="code-fence"><code' + langClass + '>' + escapeHTML(body) + '</code></pre><button type="button" class="copy-code-btn" aria-label="Copy code">Copy</button></div>\n';
    }

    flushAll();
    return out;
  }

  function updatePreview() {
    if (!previewContent) return;
    const currentSection = (sectionSelect ? sectionSelect.value : (sectionInput ? sectionInput.value : activeSection)) || '';
    if (currentSection === 'articles' || currentSection === 'til') {
      previewContent.innerHTML = '<div class="article-prose">' + renderClientArticleProse(textarea.value) + '</div>';
    } else {
      previewContent.innerHTML = renderClientOutliner(textarea.value);
    }
  }

  function insertTextAtCursor(text) {
    if (!textarea) return;
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    textarea.setRangeText(text, start, end, 'end');
    textarea.focus();
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
    updatePreview();
  }

  function compressAndUploadImage(file) {
    if (!file || !file.type.startsWith('image/')) {
      alert('Please select a valid image file.');
      return;
    }

    setStatus('Compressing WebP...', 'saving');

    const reader = new FileReader();
    reader.onload = function(e) {
      const img = new Image();
      img.onload = function() {
        let width = img.naturalWidth || img.width;
        let height = img.naturalHeight || img.height;
        const MAX_SIZE = 1600;

        if (width > MAX_SIZE || height > MAX_SIZE) {
          if (width > height) {
            height = Math.round((height * MAX_SIZE) / width);
            width = MAX_SIZE;
          } else {
            width = Math.round((width * MAX_SIZE) / height);
            height = MAX_SIZE;
          }
        }

        const canvas = document.createElement('canvas');
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext('2d');
        ctx.drawImage(img, 0, 0, width, height);

        canvas.toBlob(function(blob) {
          if (!blob) {
            setStatus('Compression failed', 'error');
            return;
          }
          uploadWebPImageBlob(blob, file.name);
        }, 'image/webp', 0.80);
      };

      img.onerror = function() {
        setStatus('Image load error', 'error');
      };

      img.src = e.target.result;
    };

    reader.onerror = function() {
      setStatus('File read error', 'error');
    };

    reader.readAsDataURL(file);
  }

  function uploadWebPImageBlob(blob, origName) {
    setStatus('Uploading image...', 'saving');

    const formData = new FormData();
    const baseName = origName ? origName.replace(/\.[^/.]+$/, "") : "image";
    formData.append('image', blob, baseName + '.webp');

    fetch('/api/upload', {
      method: 'POST',
      body: formData
    })
    .then(function(res) {
      if (!res.ok) {
        return res.json().then(function(err) {
          throw new Error(err.error || ('Upload failed with status ' + res.status));
        });
      }
      return res.json();
    })
    .then(function(data) {
      if (data && data.url) {
        const mdImage = '![image](' + data.url + ')\n';
        insertTextAtCursor(mdImage);
        setStatus('Image uploaded', '');
        setTimeout(function() {
          if (statusIndicator && statusIndicator.textContent === 'Image uploaded') {
            setStatus('Up to date', '');
          }
        }, 3000);
      } else {
        throw new Error('No URL returned from server');
      }
    })
    .catch(function(err) {
      console.error('Upload error:', err);
      setStatus('Upload failed: ' + (err.message || 'error'), 'error');
      alert('Failed to upload image: ' + (err.message || 'unknown error'));
    });
  }

  const toolbarButtons = (typeof formatToolbar !== 'undefined' && formatToolbar)
    ? formatToolbar.querySelectorAll('.toolbar-btn')
    : document.querySelectorAll('.toolbar-btn');

  document.querySelectorAll('.toolbar-btn').forEach(btn => {
    btn.addEventListener('mousedown', (e) => e.preventDefault());
    btn.addEventListener('click', () => {
      const action = btn.dataset.action;
      switch(action) {
        case 'bold': wrapSelection('**', '**', 'bold text'); break;
        case 'italic': wrapSelection('*', '*', 'italic text'); break;
        case 'code': wrapSelection('`', '`', 'code'); break;
        case 'codeblock': wrapSelection('```\n', '\n```', 'code block'); break;
        case 'wikilink': wrapSelection('[[', ']]', 'Page'); break;
        case 'tag': wrapSelection('#', '', 'tag'); break;
        case 'image':
          if (imageFileInput) imageFileInput.click();
          break;
        case 'todo': toggleLineMarker('TODO'); break;
        case 'done': toggleLineMarker('DONE'); break;
        case 'indent': indentSelection(false); break;
        case 'outdent': indentSelection(true); break;
      }
    });
  });

  if (imageFileInput) {
    imageFileInput.addEventListener('change', () => {
      if (imageFileInput.files && imageFileInput.files.length > 0) {
        compressAndUploadImage(imageFileInput.files[0]);
        imageFileInput.value = '';
      }
    });
  }

  const dropTarget = editorBox || textarea;
  if (dropTarget) {
    ['dragenter', 'dragover'].forEach(eventName => {
      dropTarget.addEventListener(eventName, (e) => {
        e.preventDefault();
        e.stopPropagation();
        if (editorBox) editorBox.classList.add('drag-over');
      }, false);
    });

    ['dragleave', 'drop'].forEach(eventName => {
      dropTarget.addEventListener(eventName, (e) => {
        e.preventDefault();
        e.stopPropagation();
        if (editorBox) editorBox.classList.remove('drag-over');
      }, false);
    });

    dropTarget.addEventListener('drop', (e) => {
      const dt = e.dataTransfer;
      if (dt && dt.files && dt.files.length > 0) {
        for (let i = 0; i < dt.files.length; i++) {
          const file = dt.files[i];
          if (file.type && file.type.startsWith('image/')) {
            compressAndUploadImage(file);
            break;
          }
        }
      }
    });
  }

  if (textarea) {
    textarea.addEventListener('paste', (e) => {
      const clipboardData = e.clipboardData || window.clipboardData;
      if (clipboardData && clipboardData.items) {
        for (let i = 0; i < clipboardData.items.length; i++) {
          const item = clipboardData.items[i];
          if (item.type && item.type.indexOf('image') !== -1) {
            e.preventDefault();
            const file = item.getAsFile();
            if (file) {
              compressAndUploadImage(file);
              break;
            }
          }
        }
      }
    });
  }

  checkLocalDraft();

  if (!textarea.value || textarea.value.trim() === '') {
    textarea.value = '- ';
    serverContent = '- ';
  }

  syncLangFromContent();
  syncCategoryFromContent();
  updatePreview();

  const urlParams = new URLSearchParams(window.location.search);
  if (urlParams.get('deleted')) {
    setStatus('Entry deleted successfully', '');
    urlParams.delete('deleted');
    const newQuery = urlParams.toString();
    const newPath = window.location.pathname + (newQuery ? '?' + newQuery : '');
    window.history.replaceState(null, '', newPath);
  }

  if (langSelect) {
    langSelect.addEventListener('change', () => {
      setLanguage(langSelect.value, { flash: true, updateContent: true });
    });
  }

  if (langToggleBtn) {
    langToggleBtn.addEventListener('mousedown', (e) => e.preventDefault());
    langToggleBtn.addEventListener('click', () => {
      toggleLang();
    });
  }
  if (textarea.value === '- ') {
    textarea.focus();
    textarea.setSelectionRange(2, 2);
  } else {
    textarea.focus();
  }
})();
