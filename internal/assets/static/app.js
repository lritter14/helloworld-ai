(() => {
  const API_URL = window.location.origin;

  const askForm = document.getElementById('ask-form');
  const questionInput = document.getElementById('question');
  const statusBanner = document.getElementById('status');
  const output = document.getElementById('output');
  const outputContainer = document.querySelector('.output-container');
  const submitBtn = document.getElementById('submit-btn');
  const folderInput = document.getElementById('folder-filter');
  const vaultInputs = Array.from(document.querySelectorAll('input[name="vaults"]'));
  const detailRadios = Array.from(document.querySelectorAll('input[name="detail-level"]'));
  const reindexBtn = document.getElementById('reindex-btn');
  const forceReindexBtn = document.getElementById('force-reindex-btn');
  const indexStatus = document.getElementById('index-status');
  let statusPollInterval = null;

  init();

  function init() {
    askForm.addEventListener('submit', handleSubmit);
    questionInput.addEventListener('keydown', handleKeydown);
    reindexBtn.addEventListener('click', () => handleReindex(false));
    forceReindexBtn.addEventListener('click', () => handleReindex(true));
  }

  function handleKeydown(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      askForm.requestSubmit();
    }
  }

  async function handleSubmit(event) {
    event.preventDefault();

    const question = questionInput.value.trim();
    if (!question) {
      showError('Please enter a question.');
      return;
    }

    const requestPayload = buildRequestPayload(question);
    clearQuestionInput();
    showOutput();
    renderPendingExchange(question);
    setLoadingState(true);

    try {
      const response = await fetch(`${API_URL}/api/v1/ask`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(requestPayload),
      });

      if (!response.ok) {
        throw await buildError(response);
      }

      const data = await response.json();
      renderAnswer(question, data.answer, data.references || []);
      clearStatus();
      output.scrollTop = output.scrollHeight;
    } catch (err) {
      const message = err?.message || 'Something went wrong.';
      showError(message);
      renderFailure(message);
    } finally {
      setLoadingState(false);
    }
  }

  function buildRequestPayload(question) {
    const payload = { question };

    const folders = parseFolders(folderInput.value);
    if (folders.length > 0) {
      payload.folders = folders;
    }

    const vaults = vaultInputs
      .filter((input) => input.checked)
      .map((input) => input.value);
    if (vaults.length > 0) {
      payload.vaults = vaults;
    }

    const detail = getSelectedDetail();
    if (detail) {
      payload.detail = detail;
    }

    return payload;
  }

  function renderPendingExchange(question) {
    output.innerHTML = `
      <span class="message-label">You:</span>
      <div>${escapeHtml(question)}</div>
      <br>
      <span class="message-label">AI:</span>
      <div class="ai-response"><span class="loading">Thinking...</span></div>
    `;
  }

  function renderAnswer(question, answer, references) {
    const answerHtml = renderMarkdown(answer || 'No answer returned.');
    const referencesHtml = renderReferences(references);

    output.innerHTML = `
      <span class="message-label">You:</span>
      <div>${escapeHtml(question)}</div>
      <br>
      <span class="message-label">AI:</span>
      <div class="ai-response">${answerHtml}</div>
      ${referencesHtml}
    `;
  }

  function renderFailure(message) {
    const aiDiv = output.querySelector('.ai-response');
    if (aiDiv) {
      aiDiv.innerHTML = `<span class="error">${escapeHtml(message)}</span>`;
    }
  }

  async function buildError(response) {
    const text = await response.text();
    try {
      const payload = JSON.parse(text);
      if (payload?.error) {
        return new Error(payload.error);
      }
    } catch (err) {
      // ignore JSON parse issues
    }
    return new Error(text || `HTTP ${response.status}`);
  }

  function parseFolders(raw) {
    return raw
      .split(',')
      .map((segment) => segment.trim())
      .filter(Boolean);
  }

  function getSelectedDetail() {
    const selected = detailRadios.find((radio) => radio.checked);
    return selected ? selected.value : '';
  }

  function renderReferences(references) {
    if (!Array.isArray(references) || references.length === 0) {
      return '';
    }

    const items = references
      .map((ref) => {
        const vault = escapeHtml(ref.vault || 'unknown');
        const relPath = escapeHtml(ref.rel_path || ref.relPath || '');
        const heading = escapeHtml(ref.heading_path || ref.headingPath || '');
        const href = buildReferenceURL(ref.vault, ref.rel_path || ref.relPath || '');
        return `
          <a class="reference-item" href="${href}" target="_blank" rel="noopener noreferrer">
            <div><span class="reference-vault">${vault}</span> / <span class="reference-path">${relPath}</span></div>
            <div class="reference-section">${heading}</div>
          </a>
        `;
      })
      .join('');

    return `
      <div class="references">
        <div class="references-title">References</div>
        ${items}
      </div>
    `;
  }

  function buildReferenceURL(vault, relPath) {
    if (!vault || !relPath) {
      return '#';
    }

    const safeVault = encodeURIComponent(vault);
    const normalizedPath = (relPath || '').replace(/\\/g, '/');
    const safePath = normalizedPath
      .split('/')
      .filter(Boolean)
      .map((segment) => encodeURIComponent(segment))
      .join('/');

    return `/notes/${safeVault}/${safePath}`;
  }

  function renderMarkdown(text) {
    if (window.marked && typeof window.marked.parse === 'function') {
      return window.marked.parse(text ?? '');
    }
    return simpleMarkdown(text);
  }

  function simpleMarkdown(text = '') {
    let safeText = escapeHtml(text);
    safeText = safeText.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    safeText = safeText.replace(/\*(.+?)\*/g, '<em>$1</em>');
    safeText = safeText.replace(/`(.+?)`/g, '<code>$1</code>');
    safeText = safeText.replace(/\n\n/g, '</p><p>');
    safeText = safeText.replace(/\n/g, '<br>');
    return `<p>${safeText}</p>`;
  }

  function escapeHtml(text = '') {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  function showError(message) {
    statusBanner.textContent = message;
    statusBanner.classList.remove('loading');
    statusBanner.classList.add('error');
  }

  function clearStatus() {
    statusBanner.textContent = '';
    statusBanner.classList.remove('loading', 'error');
  }

  function setLoadingState(isLoading) {
    submitBtn.disabled = isLoading;
    if (isLoading) {
      statusBanner.textContent = '';
      statusBanner.classList.remove('error');
      statusBanner.classList.remove('loading');
    } else if (!statusBanner.classList.contains('error')) {
      clearStatus();
    }
  }

  function clearQuestionInput() {
    questionInput.value = '';
  }

  function showOutput() {
    outputContainer?.classList.remove('hidden');
  }

  async function handleReindex(force) {
    // Disable buttons
    reindexBtn.disabled = true;
    forceReindexBtn.disabled = true;

    // Clear any existing status polling
    if (statusPollInterval) {
      clearInterval(statusPollInterval);
      statusPollInterval = null;
    }

    // Show initial status
    updateIndexStatus('Starting indexing...', 'loading');

    try {
      // Trigger indexing
      const url = force ? `${API_URL}/api/index?force=true` : `${API_URL}/api/index`;
      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });

      if (!response.ok) {
        const error = await buildError(response);
        throw error;
      }

      const data = await response.json();
      updateIndexStatus(force ? 'Force reindexing started...' : 'Reindexing started...', 'loading');

      // Start polling for status
      startStatusPolling();
    } catch (err) {
      const message = err?.message || 'Failed to start indexing.';
      updateIndexStatus(message, 'error');
      reindexBtn.disabled = false;
      forceReindexBtn.disabled = false;
    }
  }

  function startStatusPolling() {
    // Poll every 1 second
    statusPollInterval = setInterval(async () => {
      try {
        const response = await fetch(`${API_URL}/api/index/status`);
        if (!response.ok) {
          throw new Error('Failed to check status');
        }

        const data = await response.json();
        if (!data.is_indexing) {
          // Indexing is complete
          clearInterval(statusPollInterval);
          statusPollInterval = null;
          updateIndexStatus('Indexing completed successfully!', 'success');
          reindexBtn.disabled = false;
          forceReindexBtn.disabled = false;
        } else {
          // Still indexing
          updateIndexStatus('Indexing in progress...', 'loading');
        }
      } catch (err) {
        // On error, stop polling but keep status visible
        clearInterval(statusPollInterval);
        statusPollInterval = null;
        updateIndexStatus('Error checking status: ' + (err?.message || 'Unknown error'), 'error');
        reindexBtn.disabled = false;
        forceReindexBtn.disabled = false;
      }
    }, 1000);
  }

  function updateIndexStatus(message, type) {
    indexStatus.textContent = message;
    indexStatus.className = 'index-status';
    if (type === 'loading') {
      indexStatus.classList.add('loading');
    } else if (type === 'error') {
      indexStatus.classList.add('error');
    } else if (type === 'success') {
      indexStatus.classList.add('success');
    }
  }

})();
