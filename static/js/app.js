const MAX_CONVERSATIONS = 3;
var converter = new showdown.Converter({
  tables: true,
  tablesHeaderId: true,
  strikethrough: true,
  tasklists: true
});
let currentChatId = null;
let chatsList = [];

// Utility function
function escapeHtml(text) {
  if (!text) return '';
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

// Typewriter animation for streaming effect
async function typewriterEffect(element, html, speed = 10) {
  // Create a temporary container to parse the HTML
  const temp = document.createElement('div');
  temp.innerHTML = html;

  // Clear the element
  element.innerHTML = '';

  // Process each child node
  for (const node of temp.childNodes) {
    await typewriteNode(element, node, speed);
  }
}

async function typewriteNode(parent, node, speed) {
  if (node.nodeType === Node.TEXT_NODE) {
    // Text node - animate word by word for speed
    const words = node.textContent.split(/(\s+)/);
    for (const word of words) {
      if (word) {
        parent.appendChild(document.createTextNode(word));
        scrollToBottom();
        await sleep(speed);
      }
    }
  } else if (node.nodeType === Node.ELEMENT_NODE) {
    // Element node - clone and append, then animate children
    const clone = node.cloneNode(false);
    parent.appendChild(clone);

    // For code blocks, insert all at once (no animation inside code)
    if (node.tagName === 'PRE' || node.tagName === 'CODE') {
      clone.innerHTML = node.innerHTML;
      // Highlight code blocks
      if (node.tagName === 'PRE') {
        clone.querySelectorAll('code').forEach(block => hljs.highlightElement(block));
      }
      scrollToBottom();
      await sleep(50);
    } else {
      // Animate children
      for (const child of node.childNodes) {
        await typewriteNode(clone, child, speed);
      }
    }
  }
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

// Theme management
function initTheme() {
  const savedTheme = localStorage.getItem('theme') || 'light';
  document.documentElement.setAttribute('data-theme', savedTheme);
  updateThemeIcon(savedTheme);
}

function toggleTheme() {
  const current = document.documentElement.getAttribute('data-theme') || 'light';
  const next = current === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  updateThemeIcon(next);
  fetch('/api/settings/theme', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ value: next })
  }).catch(err => console.log('Theme sync error:', err));
}

function updateThemeIcon(theme) {
  const btn = document.getElementById('theme-toggle');
  if (btn) btn.textContent = theme === 'dark' ? '☀️' : '🌙';
}

// Model management
let currentModel = '';
let availableModels = [];

async function loadModels() {
  const select = document.getElementById('modelSelect');
  const providerNameEl = document.getElementById('providerName');
  if (!select) return;

  try {
    const res = await fetch('/api/active-provider');
    if (!res.ok) {
      select.innerHTML = '<option>No provider</option>';
      if (providerNameEl) providerNameEl.textContent = '';
      return;
    }

    const data = await res.json();
    currentModel = data.model;
    availableModels = data.models || [];

    // Display provider name
    if (providerNameEl && data.name) {
      providerNameEl.textContent = data.name;
      providerNameEl.style.display = 'inline-block';
    } else if (providerNameEl) {
      providerNameEl.style.display = 'none';
    }

    if (availableModels.length === 0) {
      select.innerHTML = '<option>No models</option>';
      return;
    }

    select.innerHTML = availableModels.map(model =>
      `<option value="${escapeHtml(model)}" ${model === currentModel ? 'selected' : ''}>${escapeHtml(model)}</option>`
    ).join('');

    // Add change handler
    select.addEventListener('change', async function () {
      await switchModel(this.value);
    });
  } catch (err) {
    console.log('Error loading models:', err);
    select.innerHTML = '<option>Error</option>';
    if (providerNameEl) providerNameEl.style.display = 'none';
  }
}

async function switchModel(model) {
  if (model === currentModel) return;

  try {
    const res = await fetch('/api/switch-model', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model })
    });

    if (res.ok) {
      currentModel = model;
      console.log('Switched to model:', model);
    } else {
      // Revert selection
      document.getElementById('modelSelect').value = currentModel;
      alert('Failed to switch model');
    }
  } catch (err) {
    console.log('Error switching model:', err);
    document.getElementById('modelSelect').value = currentModel;
  }
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', async function () {
  initTheme();

  // Load models for dropdown
  await loadModels();

  // Load chats list and current chat
  await loadChatsList();
  await loadCurrentChat();

  // Set up form submission
  const form = document.getElementById('chatForm');
  if (form) {
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      sendMessage();
    });
  }

  // Set up textarea
  const textarea = document.getElementById('prompt');
  if (textarea) {
    textarea.addEventListener('input', function () {
      this.style.height = 'auto';
      this.style.height = Math.min(this.scrollHeight, 200) + 'px';
    });

    textarea.addEventListener('keydown', function (e) {
      if (e.ctrlKey && e.key === 'Enter') {
        e.preventDefault();
        sendMessage();
      }
    });
  }

  // Save button handler
  const saveBtn = document.getElementById('btnSave');
  if (saveBtn) {
    saveBtn.addEventListener('click', exportChat);
  }

  // New chat button handler
  const newChatBtn = document.getElementById('btnNewChat');
  if (newChatBtn) {
    newChatBtn.addEventListener('click', startNewChat);
  }

  // Sidebar toggle (mobile)
  const sidebarToggle = document.getElementById('sidebarToggle');
  if (sidebarToggle) {
    sidebarToggle.addEventListener('click', toggleSidebar);
  }

  // Close sidebar when clicking overlay
  document.addEventListener('click', function (e) {
    if (e.target.classList.contains('sidebar-overlay')) {
      closeSidebar();
    }
  });

  // Chat search functionality
  const chatSearch = document.getElementById('chatSearch');
  if (chatSearch) {
    chatSearch.addEventListener('input', function () {
      filterChats(this.value);
    });
  }
});

// Sidebar functions
function toggleSidebar() {
  const sidebar = document.getElementById('sidebar');
  sidebar.classList.toggle('open');

  // Add/remove overlay
  let overlay = document.querySelector('.sidebar-overlay');
  if (!overlay) {
    overlay = document.createElement('div');
    overlay.className = 'sidebar-overlay';
    document.body.appendChild(overlay);
  }
  overlay.classList.toggle('show', sidebar.classList.contains('open'));
}

function closeSidebar() {
  const sidebar = document.getElementById('sidebar');
  sidebar.classList.remove('open');
  const overlay = document.querySelector('.sidebar-overlay');
  if (overlay) overlay.classList.remove('show');
}

// Load chats list for sidebar
async function loadChatsList() {
  try {
    const res = await fetch('/api/chats');
    if (!res.ok) return;

    chatsList = await res.json();
    renderChatsList();
  } catch (err) {
    console.log('Error loading chats:', err);
    document.getElementById('chatList').innerHTML =
      '<div class="chat-list-loading">Error loading chats</div>';
  }
}

function renderChatsList(filter = '') {
  const container = document.getElementById('chatList');

  if (!chatsList || chatsList.length === 0) {
    container.innerHTML = '<div class="chat-list-loading">No chats yet</div>';
    return;
  }

  // Filter chats by title if search query provided
  const filteredChats = filter
    ? chatsList.filter(chat => chat.title.toLowerCase().includes(filter.toLowerCase()))
    : chatsList;

  if (filteredChats.length === 0) {
    container.innerHTML = '<div class="chat-list-loading">No matching chats</div>';
    return;
  }

  container.innerHTML = filteredChats.map(chat => `
    <div class="chat-item ${chat.id === currentChatId ? 'active' : ''}" 
         data-chat-id="${chat.id}">
      <div class="chat-item-content" onclick="selectChat(${chat.id})">
        <div class="chat-item-title">${escapeHtml(chat.title)}</div>
      </div>
      <div class="chat-item-actions">
        <button class="chat-item-rename" onclick="renameChat(${chat.id}, event)" title="Rename chat">✏️</button>
        <button class="chat-item-delete" onclick="deleteChat(${chat.id}, event)" title="Delete chat">×</button>
      </div>
    </div>
  `).join('');
}

// Filter chats by search query (searches titles and message content via API)
let searchTimeout = null;
function filterChats(query) {
  const q = query.trim();

  // Debounce the search
  if (searchTimeout) {
    clearTimeout(searchTimeout);
  }

  if (!q) {
    // No query, show all chats from cache
    renderChatsList('');
    return;
  }

  // Debounce API calls
  searchTimeout = setTimeout(async () => {
    try {
      const res = await fetch(`/api/chats/search?q=${encodeURIComponent(q)}`);
      if (res.ok) {
        const results = await res.json();
        renderSearchResults(results, q);
      }
    } catch (err) {
      console.log('Search error:', err);
      // Fallback to client-side title search
      renderChatsList(q);
    }
  }, 300);
}

function renderSearchResults(results, query) {
  const container = document.getElementById('chatList');

  if (results.length === 0) {
    container.innerHTML = '<div class="chat-list-loading">No matching chats</div>';
    return;
  }

  container.innerHTML = results.map(chat => `
    <div class="chat-item ${chat.id === currentChatId ? 'active' : ''}" 
         data-chat-id="${chat.id}">
      <div class="chat-item-content" onclick="selectChat(${chat.id})">
        <div class="chat-item-title">${escapeHtml(chat.title)}</div>
        <div class="chat-item-date">${formatDate(chat.updated_at)}</div>
      </div>
      <div class="chat-item-actions">
        <button class="chat-item-rename" onclick="renameChat(${chat.id}, event)" title="Rename chat">✏️</button>
        <button class="chat-item-delete" onclick="deleteChat(${chat.id}, event)" title="Delete chat">×</button>
      </div>
    </div>
  `).join('');
}

function formatDate(dateStr) {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now - date;
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 0) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  } else if (diffDays === 1) {
    return 'Yesterday';
  } else if (diffDays < 7) {
    return date.toLocaleDateString([], { weekday: 'short' });
  } else {
    return date.toLocaleDateString([], { month: 'short', day: 'numeric' });
  }
}

// Select a chat from sidebar
async function selectChat(chatId) {
  if (chatId === currentChatId) {
    closeSidebar();
    return;
  }

  try {
    const res = await fetch(`/api/chats/${chatId}`);
    if (!res.ok) return;

    const chat = await res.json();
    currentChatId = chat.id;

    // Update sidebar active state
    document.querySelectorAll('.chat-item').forEach(item => {
      item.classList.toggle('active', parseInt(item.dataset.chatId) === chatId);
    });

    // Clear and render messages
    const printout = document.getElementById('printout');
    printout.innerHTML = '';

    if (chat.messages && chat.messages.length > 0) {
      const welcome = document.getElementById('welcomeMessage');
      if (welcome) welcome.style.display = 'none';

      // Group messages by version_group
      const versionGroups = {};
      const regularMessages = [];

      for (const msg of chat.messages) {
        if (msg.version_group) {
          if (!versionGroups[msg.version_group]) {
            versionGroups[msg.version_group] = [];
          }
          versionGroups[msg.version_group].push(msg);
        } else {
          regularMessages.push(msg);
        }
      }

      // Render regular messages and version groups
      let i = 0;
      while (i < chat.messages.length) {
        const msg = chat.messages[i];

        if (msg.version_group && versionGroups[msg.version_group]) {
          // Render version group
          const groupMsgs = versionGroups[msg.version_group];
          delete versionGroups[msg.version_group]; // Mark as rendered

          // Pair messages: user+assistant pairs
          const pairs = [];
          for (let j = 0; j < groupMsgs.length; j++) {
            if (groupMsgs[j].role === 'user') {
              const pair = { user: groupMsgs[j], assistant: null };
              // Find the matching assistant message
              if (j + 1 < groupMsgs.length && groupMsgs[j + 1].role === 'assistant') {
                pair.assistant = groupMsgs[j + 1];
                j++;
              }
              pairs.push(pair);
            }
          }

          if (pairs.length > 0) {
            printout.insertAdjacentHTML('beforeend', createVersionGroupHtml(pairs, msg.version_group));
          }

          // Skip the messages we just processed
          while (i < chat.messages.length && chat.messages[i].version_group === msg.version_group) {
            i++;
          }
        } else {
          // Render regular message
          if (msg.role === 'user') {
            printout.insertAdjacentHTML('beforeend', createUserMessageHtml(msg.id, msg.content));
          } else {
            const meta = {};
            if (msg.model_name) meta.model = msg.model_name;
            if (msg.tokens_used) meta.tokens = msg.tokens_used;
            printout.insertAdjacentHTML('beforeend', createAssistantMessageHtml(msg.id, msg.content, true, meta));
          }
          i++;
        }
      }
    } else {
      printout.innerHTML = `
        <div class="welcome-message" id="welcomeMessage">
          <div class="welcome-icon">🦙</div>
          <h2>Welcome to OllamaGoWeb</h2>
          <p>Start a conversation by typing a message below.</p>
          <p class="welcome-hint">Press <kbd>Ctrl</kbd> + <kbd>Enter</kbd> to send</p>
        </div>
      `;
    }

    scrollToBottom();
    closeSidebar();
  } catch (err) {
    console.log('Error loading chat:', err);
  }
}

// Create HTML for a version group
function createVersionGroupHtml(pairs, versionGroupId) {
  const versionsHtml = pairs.map((pair, index) => {
    const userHtml = createUserMessageHtml(pair.user.id, pair.user.content);
    let assistantHtml = '';
    if (pair.assistant) {
      const meta = {};
      if (pair.assistant.model_name) meta.model = pair.assistant.model_name;
      if (pair.assistant.tokens_used) meta.tokens = pair.assistant.tokens_used;
      assistantHtml = createAssistantMessageHtml(pair.assistant.id, pair.assistant.content, true, meta);
    }
    return `<div class="version-item ${index === pairs.length - 1 ? 'active' : ''}" data-version-index="${index}">${userHtml}${assistantHtml}</div>`;
  }).join('');

  return `
    <div class="version-container" data-version-group="${versionGroupId}" data-current-version="${pairs.length - 1}">
      <div class="version-nav">
        <button class="version-nav-btn" onclick="navigateVersion(this, -1)" title="Previous version">◀</button>
        <span class="version-indicator">Version <span class="version-current">${pairs.length}</span> of <span class="version-total">${pairs.length}</span></span>
        <button class="version-nav-btn" onclick="navigateVersion(this, 1)" title="Next version" disabled>▶</button>
      </div>
      ${versionsHtml}
    </div>
  `;
}

// Delete a chat
async function deleteChat(chatId, event) {
  event.stopPropagation();

  if (!confirm('Delete this chat?')) return;

  try {
    const res = await fetch(`/api/chats/${chatId}`, { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete');

    // Remove from list
    chatsList = chatsList.filter(c => c.id !== chatId);
    renderChatsList();

    // If we deleted current chat, load another or create new
    if (chatId === currentChatId) {
      if (chatsList.length > 0) {
        await selectChat(chatsList[0].id);
      } else {
        await startNewChat();
      }
    }
  } catch (err) {
    console.log('Error deleting chat:', err);
  }
}

// Rename a chat (inline editing)
async function renameChat(chatId, event) {
  event.stopPropagation();

  const chatItem = document.querySelector(`.chat-item[data-chat-id="${chatId}"]`);
  if (!chatItem) return;

  const titleElement = chatItem.querySelector('.chat-item-title');
  if (!titleElement || titleElement.querySelector('.inline-title-input')) return;

  const chat = chatsList.find(c => c.id === chatId);
  const currentTitle = chat ? chat.title : 'Untitled';

  // Create inline edit input
  const originalHtml = titleElement.innerHTML;
  titleElement.innerHTML = `<input type="text" class="inline-title-input" value="${escapeHtml(currentTitle)}">`;

  const input = titleElement.querySelector('.inline-title-input');
  input.dataset.originalTitle = currentTitle;
  input.focus();
  input.select();

  // Handle save
  const saveTitle = async () => {
    const newTitle = input.value.trim();
    if (!newTitle || newTitle === currentTitle) {
      titleElement.innerHTML = originalHtml;
      return;
    }

    try {
      const res = await fetch(`/api/chats/${chatId}/rename`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: newTitle })
      });

      if (!res.ok) throw new Error('Failed to rename');

      // Update local list
      const listItem = chatsList.find(c => c.id === chatId);
      if (listItem) {
        listItem.title = newTitle;
        renderChatsList();
      }
    } catch (err) {
      console.log('Error renaming chat:', err);
      titleElement.innerHTML = originalHtml;
    }
  };

  input.addEventListener('blur', saveTitle);
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      input.blur();
    } else if (e.key === 'Escape') {
      input.value = currentTitle;
      input.blur();
    }
  });
}

// Load current/most recent chat
async function loadCurrentChat() {
  try {
    const res = await fetch('/api/chats/current');
    if (!res.ok) return;

    const chat = await res.json();
    currentChatId = chat.id;

    // Update sidebar active state
    renderChatsList();

    // Hide welcome message if there are messages
    if (chat.messages && chat.messages.length > 0) {
      const welcome = document.getElementById('welcomeMessage');
      if (welcome) welcome.style.display = 'none';

      // Render existing messages using the same logic as selectChat
      const printout = document.getElementById('printout');
      printout.innerHTML = '';

      // Group messages by version_group
      const versionGroups = {};
      for (const msg of chat.messages) {
        if (msg.version_group) {
          if (!versionGroups[msg.version_group]) {
            versionGroups[msg.version_group] = [];
          }
          versionGroups[msg.version_group].push(msg);
        }
      }

      // Render messages
      let i = 0;
      while (i < chat.messages.length) {
        const msg = chat.messages[i];

        if (msg.version_group && versionGroups[msg.version_group]) {
          const groupMsgs = versionGroups[msg.version_group];
          delete versionGroups[msg.version_group];

          const pairs = [];
          for (let j = 0; j < groupMsgs.length; j++) {
            if (groupMsgs[j].role === 'user') {
              const pair = { user: groupMsgs[j], assistant: null };
              if (j + 1 < groupMsgs.length && groupMsgs[j + 1].role === 'assistant') {
                pair.assistant = groupMsgs[j + 1];
                j++;
              }
              pairs.push(pair);
            }
          }

          if (pairs.length > 0) {
            printout.insertAdjacentHTML('beforeend', createVersionGroupHtml(pairs, msg.version_group));
          }

          while (i < chat.messages.length && chat.messages[i].version_group === msg.version_group) {
            i++;
          }
        } else {
          if (msg.role === 'user') {
            printout.insertAdjacentHTML('beforeend', createUserMessageHtml(msg.id, msg.content));
          } else {
            const meta = {};
            if (msg.model_name) meta.model = msg.model_name;
            if (msg.tokens_used) meta.tokens = msg.tokens_used;
            printout.insertAdjacentHTML('beforeend', createAssistantMessageHtml(msg.id, msg.content, true, meta));
          }
          i++;
        }
      }
      scrollToBottom();
    }

    updateProgressBar(chat.messages ? chat.messages.length : 0);
  } catch (err) {
    console.log('Error loading chat:', err);
  }
}

// Start a new chat
async function startNewChat() {
  try {
    const res = await fetch('/api/chats', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: 'New Chat' })
    });

    if (res.ok) {
      const data = await res.json();
      currentChatId = data.id;

      // Add to list and re-render
      chatsList.unshift({ id: data.id, title: data.title, updated_at: new Date().toISOString() });
      renderChatsList();

      // Clear UI
      const printout = document.getElementById('printout');
      printout.innerHTML = `
        <div class="welcome-message" id="welcomeMessage">
          <div class="welcome-icon">🦙</div>
          <h2>Welcome to OllamaGoWeb</h2>
          <p>Start a conversation by typing a message below.</p>
          <p class="welcome-hint">Press <kbd>Ctrl</kbd> + <kbd>Enter</kbd> to send</p>
        </div>
      `;
      updateProgressBar(0);
      closeSidebar();
    }
  } catch (err) {
    console.log('Error creating chat:', err);
  }
}

function createUserMessageHtml(id, content) {
  return `
    <div id="msg-${id}" class="message-group user-message-group">
      <div class="prompt-message">
        <span class="message-content">${escapeHtml(content)}</span>
        <button class="message-edit-btn" onclick="editMessage(${id}, 'user')" title="Edit message">✏️</button>
      </div>
    </div>
  `;
}

function createAssistantMessageHtml(id, content, isFormatted = false, meta = {}) {
  const formattedContent = isFormatted ? converter.makeHtml(content) : content;

  // Build metadata HTML
  let metaHtml = '';
  // Don't show regenerate button for pending messages
  const showRegenerate = id && !String(id).startsWith('pending');

  if (meta.model || meta.tokens || meta.speed || showRegenerate) {
    metaHtml = '<div class="message-meta">';
    if (meta.model) {
      metaHtml += `<span class="message-meta-item" title="Model"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>${escapeHtml(meta.model)}</span>`;
    }
    if (meta.tokens) {
      metaHtml += `<span class="message-meta-item" title="Tokens used"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>${meta.tokens} tokens</span>`;
    }
    if (meta.speed) {
      metaHtml += `<span class="message-meta-item" title="Speed">${escapeHtml(meta.speed)}</span>`;
    }

    if (showRegenerate) {
      metaHtml += `<button class="regenerate-btn" onclick="regenerateResponse(${id})" title="Regenerate response"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10"/><path d="M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></button>`;
    }

    metaHtml += '</div>';
  }

  return `
    <div class="message-group assistant-message-group" data-msg-id="${id}">
      <div class="response-wrapper">
        <div id="response-${id}" class="response-message">${formattedContent}</div>
        <div id="meta-${id}" class="message-meta-container">
          ${metaHtml}
        </div>
      </div>
    </div>
  `;
}

async function sendMessage() {
  const textarea = document.getElementById('prompt');
  const prompt = textarea.value.trim();

  if (!prompt) return;

  // Ensure we have a chat
  if (!currentChatId) {
    const res = await fetch('/api/chats', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: 'New Chat' })
    });
    if (res.ok) {
      const data = await res.json();
      currentChatId = data.id;
      chatsList.unshift({ id: data.id, title: data.title, updated_at: new Date().toISOString() });
      renderChatsList();
    } else {
      console.error('Failed to create chat');
      return;
    }
  }

  // Hide welcome message
  const welcome = document.getElementById('welcomeMessage');
  if (welcome) welcome.style.display = 'none';

  textarea.value = '';
  textarea.style.height = 'auto';

  // Save user message to database
  let userMsgId;
  try {
    const res = await fetch(`/api/chats/${currentChatId}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ role: 'user', content: prompt })
    });
    if (res.ok) {
      const data = await res.json();
      userMsgId = data.id;

      // Update chat title in sidebar if this is first message
      const titlePreview = prompt.length > 30 ? prompt.substring(0, 30) + '...' : prompt;
      const chatItem = chatsList.find(c => c.id === currentChatId);
      if (chatItem && chatItem.title === 'New Chat') {
        chatItem.title = titlePreview;
        renderChatsList();
      }
    }
  } catch (err) {
    console.log('Error saving user message:', err);
  }

  // Display user message
  const printout = document.getElementById('printout');
  printout.insertAdjacentHTML('beforeend', createUserMessageHtml(userMsgId || Date.now(), prompt));

  // Create placeholder for assistant response
  const assistantMsgId = 'pending-' + Date.now();
  printout.insertAdjacentHTML('beforeend', `
    <div class="message-group assistant-message-group" data-msg-id="${assistantMsgId}">
      <div class="response-wrapper">
        <div id="response-${assistantMsgId}" class="response-message">
          <span class="message-loader spinner-border spinner-border-sm"></span>
        </div>
        <div id="meta-${assistantMsgId}" class="message-meta-container"></div>
      </div>
    </div>
  `);
  scrollToBottom();

  // Get response
  try {
    const response = await fetch('/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ input: prompt, chat_id: currentChatId })
    });

    if (!response.ok) throw new Error(`Server error: ${response.status}`);

    const outputEl = document.getElementById(`response-${assistantMsgId}`);
    outputEl.innerHTML = '';

    const decoder = new TextDecoder();
    const reader = response.body.getReader();
    let fullResponse = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      const text = decoder.decode(value);
      fullResponse += text;
      outputEl.insertAdjacentText('beforeend', text);
      scrollToBottom();
    }

    // Parse analytics from the end of the response
    let analytics = null;
    let responseContent = fullResponse;
    const analyticsMarker = '\n\n__ANALYTICS__';
    const analyticsIndex = fullResponse.indexOf(analyticsMarker);

    if (analyticsIndex !== -1) {
      responseContent = fullResponse.substring(0, analyticsIndex);
      try {
        const analyticsJson = fullResponse.substring(analyticsIndex + analyticsMarker.length);
        analytics = JSON.parse(analyticsJson);
        console.log('Response analytics:', analytics);
      } catch (e) {
        console.log('Failed to parse analytics:', e);
      }
    }

    // Format the response with typewriter effect (without analytics)
    const formattedHtml = converter.makeHtml(responseContent);
    await typewriterEffect(outputEl, formattedHtml, 8);
    // Final highlight pass for any code blocks
    outputEl.querySelectorAll('pre code').forEach(block => hljs.highlightElement(block));

    // Display analytics metadata
    const metaEl = document.getElementById(`meta-${assistantMsgId}`);
    if (metaEl && analytics) {
      let metaHtml = '<div class="message-meta">';
      if (analytics.model) {
        metaHtml += `<span class="message-meta-item" title="Model"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>${escapeHtml(analytics.model)}</span>`;
      }
      if (analytics.usage && analytics.usage.total_tokens) {
        metaHtml += `<span class="message-meta-item" title="Tokens used"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>${analytics.usage.total_tokens} tokens</span>`;
      }
      if (analytics.speed) {
        metaHtml += `<span class="message-meta-item" title="Speed">⚡${escapeHtml(analytics.speed)}</span>`;
      }
      metaHtml += '</div>';
      metaEl.innerHTML = metaHtml;
    }

    // Save assistant message to database with model name and tokens
    try {
      const msgPayload = {
        role: 'assistant',
        content: responseContent
      };
      if (analytics) {
        if (analytics.model) msgPayload.model_name = analytics.model;
        if (analytics.usage && analytics.usage.total_tokens) {
          msgPayload.tokens_used = analytics.usage.total_tokens;
        }
      }
      await fetch(`/api/chats/${currentChatId}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(msgPayload)
      });
    } catch (err) {
      console.log('Error saving assistant message:', err);
    }

    updateProgressBar();

  } catch (error) {
    console.error('Error:', error);
    const outputEl = document.getElementById(`response-${assistantMsgId}`);
    if (outputEl) {
      outputEl.innerHTML = `<span style="color: #ef4444;">Error: ${escapeHtml(error.message)}</span>`;
    }
  }
}

function updateProgressBar(count) {
  const container = document.getElementById('progress-container');
  if (!container) return;

  if (count === undefined) {
    fetch(`/api/chats/${currentChatId}`)
      .then(res => res.json())
      .then(chat => {
        const msgCount = chat.messages ? Math.ceil(chat.messages.length / 2) : 0;
        renderProgressBar(container, msgCount);
      })
      .catch(() => renderProgressBar(container, 0));
    return;
  }

  renderProgressBar(container, Math.ceil(count / 2));
}

function renderProgressBar(container, count) {
  container.innerHTML = '';
  for (let i = 0; i < MAX_CONVERSATIONS; i++) {
    const slot = document.createElement('div');
    slot.className = 'progress-slot' + (i < count ? ' filled' : '');
    container.appendChild(slot);
  }
}

function scrollToBottom() {
  const messages = document.getElementById('chatMessages');
  if (messages) {
    messages.scrollTop = messages.scrollHeight;
  }
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

function exportChat() {
  const printout = document.getElementById('printout');
  if (!printout) return;

  const date = new Date();
  const fileName = `chat_${date.getFullYear()}${String(date.getMonth() + 1).padStart(2, '0')}${String(date.getDate()).padStart(2, '0')}_${String(date.getHours()).padStart(2, '0')}${String(date.getMinutes()).padStart(2, '0')}`;

  const llmTag = document.getElementById('llmtag');
  const providerInfo = llmTag ? llmTag.textContent : '';

  const html = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Chat Export - ${fileName}</title>
  <style>
    body { font-family: -apple-system, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; background: #f5f5f5; }
    .message-group { margin: 16px 0; }
    .prompt-message { background: #2563eb; color: white; padding: 12px 16px; border-radius: 16px; display: inline-block; max-width: 80%; }
    .response-message { background: white; padding: 16px; border-radius: 16px; border: 1px solid #e5e5e5; }
    pre { background: #1a1a1a; color: #e5e5e5; padding: 16px; border-radius: 8px; overflow-x: auto; }
    .user-message-group { text-align: right; }
  </style>
</head>
<body>
  <h1>💬 Chat Export</h1>
  <p><small>Provider: ${providerInfo} | Exported: ${date.toLocaleString()}</small></p>
  <hr>
  ${printout.innerHTML}
</body>
</html>`;

  const blob = new Blob([html], { type: 'text/html' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${fileName}.html`;
  a.click();
  URL.revokeObjectURL(url);
}

// Edit message functionality with version history
async function editMessage(msgId, role) {
  const msgElement = document.getElementById(`msg-${msgId}`);
  if (!msgElement) return;

  const contentElement = msgElement.querySelector('.message-content');
  if (!contentElement) return;

  // Check if already in edit mode
  if (msgElement.querySelector('.inline-edit-textarea')) return;

  const currentContent = contentElement.textContent;

  // Create inline edit UI
  const originalHtml = contentElement.innerHTML;
  contentElement.innerHTML = `
    <textarea class="inline-edit-textarea" rows="3">${escapeHtml(currentContent)}</textarea>
    <div class="inline-edit-actions">
      <button class="inline-edit-btn save" onclick="saveInlineEdit(${msgId}, this)" title="Save changes">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      </button>
      <button class="inline-edit-btn cancel" onclick="cancelInlineEdit(${msgId}, this)" title="Cancel">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </div>
  `;

  const textarea = contentElement.querySelector('.inline-edit-textarea');
  textarea.dataset.originalContent = currentContent;
  textarea.dataset.originalHtml = originalHtml;
  textarea.focus();
  textarea.setSelectionRange(textarea.value.length, textarea.value.length);

  // Handle keyboard shortcuts
  textarea.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      cancelInlineEdit(msgId, textarea);
    } else if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      saveInlineEdit(msgId, textarea);
    }
  });
}

async function saveInlineEdit(msgId, element) {
  const msgElement = document.getElementById(`msg-${msgId}`);
  const contentElement = msgElement.querySelector('.message-content');
  const textarea = contentElement.querySelector('.inline-edit-textarea');

  const newContent = textarea.value.trim();
  const originalContent = textarea.dataset.originalContent;

  if (newContent === '' || newContent === originalContent) {
    cancelInlineEdit(msgId, element);
    return;
  }

  const trimmedContent = newContent;

  try {
    // Find the assistant message that follows this user message
    const allMsgGroups = Array.from(document.querySelectorAll('.message-group'));
    const userMsgIndex = allMsgGroups.indexOf(msgElement);
    let assistantMsgGroup = null;

    if (userMsgIndex >= 0 && userMsgIndex < allMsgGroups.length - 1) {
      const nextMsgGroup = allMsgGroups[userMsgIndex + 1];
      if (nextMsgGroup && nextMsgGroup.classList.contains('assistant-message-group')) {
        assistantMsgGroup = nextMsgGroup;
      }
    }

    // Check if this is already part of a version group
    let versionContainer = msgElement.closest('.version-container');
    let versionGroupId = versionContainer?.dataset.versionGroup;

    if (!versionContainer) {
      // Create a version group ID based on the original message ID
      versionGroupId = `vg-${msgId}`;

      // Update the original messages with version_group in DB
      await fetch(`/api/messages/${msgId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version_group: versionGroupId })
      });

      if (assistantMsgGroup) {
        const assistantId = assistantMsgGroup.dataset.msgId;
        if (assistantId && !String(assistantId).startsWith('pending')) {
          await fetch(`/api/messages/${assistantId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ version_group: versionGroupId })
          });
        }
      }

      // Create a version container and wrap the current user + assistant pair
      versionContainer = document.createElement('div');
      versionContainer.className = 'version-container';
      versionContainer.dataset.currentVersion = '0';
      versionContainer.dataset.versionGroup = versionGroupId;

      // Create version wrapper for the original pair
      const version0 = document.createElement('div');
      version0.className = 'version-item active';
      version0.dataset.versionIndex = '0';

      // Move the current user message into the version
      msgElement.parentNode.insertBefore(versionContainer, msgElement);
      version0.appendChild(msgElement);

      // Move the assistant message if exists
      if (assistantMsgGroup) {
        version0.appendChild(assistantMsgGroup);
      }

      versionContainer.appendChild(version0);

      // Add navigation controls
      const navHtml = `
        <div class="version-nav">
          <button class="version-nav-btn" onclick="navigateVersion(this, -1)" title="Previous version">◀</button>
          <span class="version-indicator">Version <span class="version-current">1</span> of <span class="version-total">1</span></span>
          <button class="version-nav-btn" onclick="navigateVersion(this, 1)" title="Next version">▶</button>
        </div>
      `;
      versionContainer.insertAdjacentHTML('afterbegin', navHtml);
    }

    // Save the new user message to DB with version_group
    const userRes = await fetch(`/api/chats/${currentChatId}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ role: 'user', content: trimmedContent, version_group: versionGroupId })
    });

    if (!userRes.ok) throw new Error('Failed to save new message version');
    const newUserMsg = await userRes.json();

    // Create new version with the edited user message
    const versions = versionContainer.querySelectorAll('.version-item');
    const newVersionIndex = versions.length;

    const newVersion = document.createElement('div');
    newVersion.className = 'version-item';
    newVersion.dataset.versionIndex = String(newVersionIndex);
    newVersion.innerHTML = createUserMessageHtml(newUserMsg.id, trimmedContent);

    // Add placeholder for assistant response
    const pendingAssistantId = 'pending-' + Date.now();
    newVersion.insertAdjacentHTML('beforeend', `
      <div class="message-group assistant-message-group" data-msg-id="${pendingAssistantId}">
        <div class="response-wrapper">
          <div id="response-${pendingAssistantId}" class="response-message">
            <span class="message-loader spinner-border spinner-border-sm"></span>
          </div>
          <div id="meta-${pendingAssistantId}" class="message-meta-container"></div>
        </div>
      </div>
    `);

    versionContainer.appendChild(newVersion);

    // Update navigation to show new version
    versionContainer.dataset.currentVersion = String(newVersionIndex);
    updateVersionNav(versionContainer);

    // Hide old versions, show new one
    versions.forEach(v => v.classList.remove('active'));
    newVersion.classList.add('active');

    scrollToBottom();

    // Generate response for the new version
    await generateResponseForVersion(trimmedContent, pendingAssistantId, newVersion, versionGroupId);

  } catch (err) {
    console.error('Error creating message version:', err);
    alert('Failed to create new version');
  }
}

function cancelInlineEdit(msgId, element) {
  const msgElement = document.getElementById(`msg-${msgId}`);
  if (!msgElement) return;

  const contentElement = msgElement.querySelector('.message-content');
  const textarea = contentElement.querySelector('.inline-edit-textarea');

  if (textarea && textarea.dataset.originalContent) {
    contentElement.textContent = textarea.dataset.originalContent;
  }
}

// Generate response within a specific version
async function generateResponseForVersion(prompt, assistantMsgId, versionElement, versionGroupId) {
  try {
    const response = await fetch('/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ input: prompt, chat_id: currentChatId })
    });

    if (!response.ok) throw new Error(`Server error: ${response.status}`);

    const outputEl = document.getElementById(`response-${assistantMsgId}`);
    if (!outputEl) return;
    outputEl.innerHTML = '';

    const decoder = new TextDecoder();
    const reader = response.body.getReader();
    let fullResponse = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      const text = decoder.decode(value);
      fullResponse += text;
      outputEl.insertAdjacentText('beforeend', text);
      scrollToBottom();
    }

    // Parse analytics
    let analytics = null;
    let responseContent = fullResponse;
    const analyticsMarker = '\n\n__ANALYTICS__';
    const analyticsIndex = fullResponse.indexOf(analyticsMarker);

    if (analyticsIndex !== -1) {
      responseContent = fullResponse.substring(0, analyticsIndex);
      try {
        analytics = JSON.parse(fullResponse.substring(analyticsIndex + analyticsMarker.length));
      } catch (e) { }
    }

    // Format with typewriter
    const formattedHtml = converter.makeHtml(responseContent);
    await typewriterEffect(outputEl, formattedHtml, 8);
    outputEl.querySelectorAll('pre code').forEach(block => hljs.highlightElement(block));

    // Save assistant message with version_group
    try {
      const msgPayload = { role: 'assistant', content: responseContent, version_group: versionGroupId };
      if (analytics) {
        if (analytics.model) msgPayload.model_name = analytics.model;
        if (analytics.usage?.total_tokens) msgPayload.tokens_used = analytics.usage.total_tokens;
      }
      const saveRes = await fetch(`/api/chats/${currentChatId}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(msgPayload)
      });
      if (saveRes.ok) {
        const savedMsg = await saveRes.json();
        const msgGroup = versionElement.querySelector(`[data-msg-id="${assistantMsgId}"]`);
        if (msgGroup && savedMsg.id) {
          msgGroup.dataset.msgId = savedMsg.id;
          const metaEl = document.getElementById(`meta-${assistantMsgId}`);
          if (metaEl) {
            let metaHtml = '<div class="message-meta">';
            if (analytics?.model) metaHtml += `<span class="message-meta-item" title="Model">🤖 ${escapeHtml(analytics.model)}</span>`;
            if (analytics?.usage?.total_tokens) metaHtml += `<span class="message-meta-item" title="Tokens">📊 ${analytics.usage.total_tokens} tokens</span>`;
            metaHtml += '</div>';
            metaEl.innerHTML = metaHtml;
          }
        }
      }
    } catch (err) {
      console.log('Error saving assistant message:', err);
    }

  } catch (error) {
    console.error('Error generating response:', error);
    const outputEl = document.getElementById(`response-${assistantMsgId}`);
    if (outputEl) {
      outputEl.innerHTML = `<span style="color: #ef4444;">Error: ${escapeHtml(error.message)}</span>`;
    }
  }
}

// Navigate between versions
function navigateVersion(btn, direction) {
  const container = btn.closest('.version-container');
  if (!container) return;

  const versions = container.querySelectorAll('.version-item');
  let currentIndex = parseInt(container.dataset.currentVersion) || 0;

  currentIndex += direction;
  if (currentIndex < 0) currentIndex = 0;
  if (currentIndex >= versions.length) currentIndex = versions.length - 1;

  container.dataset.currentVersion = String(currentIndex);

  versions.forEach((v, i) => {
    v.classList.toggle('active', i === currentIndex);
  });

  updateVersionNav(container);
}

function updateVersionNav(container) {
  const versions = container.querySelectorAll('.version-item');
  const currentIndex = parseInt(container.dataset.currentVersion) || 0;

  const currentSpan = container.querySelector('.version-current');
  const totalSpan = container.querySelector('.version-total');
  const prevBtn = container.querySelector('.version-nav-btn:first-of-type');
  const nextBtn = container.querySelector('.version-nav-btn:last-of-type');

  if (currentSpan) currentSpan.textContent = currentIndex + 1;
  if (totalSpan) totalSpan.textContent = versions.length;
  if (prevBtn) prevBtn.disabled = currentIndex === 0;
  if (nextBtn) nextBtn.disabled = currentIndex === versions.length - 1;
}

// Regenerate response - delete current response and generate a new one
async function regenerateResponse(assistantMsgId) {
  const msgGroup = document.querySelector(`[data-msg-id="${assistantMsgId}"]`);
  if (!msgGroup) return;

  // Find the user message before this assistant message
  const allMsgGroups = Array.from(document.querySelectorAll('.message-group'));
  const msgIndex = allMsgGroups.indexOf(msgGroup);
  if (msgIndex < 1) return;

  const userMsgGroup = allMsgGroups[msgIndex - 1];
  if (!userMsgGroup.classList.contains('user-message-group')) return;

  const userContent = userMsgGroup.querySelector('.message-content')?.textContent;
  if (!userContent) return;

  // Delete the assistant message from DB
  try {
    await fetch(`/api/messages/${assistantMsgId}`, { method: 'DELETE' });
  } catch (err) {
    console.log('Error deleting message:', err);
  }

  // Remove the message from UI
  msgGroup.remove();

  // Regenerate
  await generateResponse(userContent);
}

// Generate response (reusable for regeneration)
async function generateResponse(prompt) {
  const printout = document.getElementById('printout');

  // Create placeholder for assistant response
  const assistantMsgId = 'pending-' + Date.now();
  printout.insertAdjacentHTML('beforeend', `
    <div class="message-group assistant-message-group" data-msg-id="${assistantMsgId}">
      <div class="response-wrapper">
        <div id="response-${assistantMsgId}" class="response-message">
          <span class="message-loader spinner-border spinner-border-sm"></span>
        </div>
        <div id="meta-${assistantMsgId}" class="message-meta-container"></div>
      </div>
    </div>
  `);
  scrollToBottom();

  try {
    const response = await fetch('/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ input: prompt, chat_id: currentChatId })
    });

    if (!response.ok) throw new Error(`Server error: ${response.status}`);

    const outputEl = document.getElementById(`response-${assistantMsgId}`);
    outputEl.innerHTML = '';

    const decoder = new TextDecoder();
    const reader = response.body.getReader();
    let fullResponse = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      const text = decoder.decode(value);
      fullResponse += text;
      outputEl.insertAdjacentText('beforeend', text);
      scrollToBottom();
    }

    // Parse analytics from the end of the response
    let analytics = null;
    let responseContent = fullResponse;
    const analyticsMarker = '\n\n__ANALYTICS__';
    const analyticsIndex = fullResponse.indexOf(analyticsMarker);

    if (analyticsIndex !== -1) {
      responseContent = fullResponse.substring(0, analyticsIndex);
      try {
        const analyticsJson = fullResponse.substring(analyticsIndex + analyticsMarker.length);
        analytics = JSON.parse(analyticsJson);
      } catch (e) {
        console.log('Failed to parse analytics:', e);
      }
    }

    // Format the response with typewriter effect
    const formattedHtml = converter.makeHtml(responseContent);
    await typewriterEffect(outputEl, formattedHtml, 8);
    outputEl.querySelectorAll('pre code').forEach(block => hljs.highlightElement(block));

    // Display analytics metadata
    const metaEl = document.getElementById(`meta-${assistantMsgId}`);
    if (metaEl && analytics) {
      let metaHtml = '<div class="message-meta">';
      if (analytics.model) {
        metaHtml += `<span class="message-meta-item" title="Model"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>${escapeHtml(analytics.model)}</span>`;
      }
      if (analytics.usage && analytics.usage.total_tokens) {
        metaHtml += `<span class="message-meta-item" title="Tokens used"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>${analytics.usage.total_tokens} tokens</span>`;
      }
      if (analytics.speed) {
        metaHtml += `<span class="message-meta-item" title="Speed">⚡${escapeHtml(analytics.speed)}</span>`;
      }
      metaHtml += '</div>';
      metaEl.innerHTML = metaHtml;
    }

    // Save assistant message to database
    try {
      const msgPayload = { role: 'assistant', content: responseContent };
      if (analytics) {
        if (analytics.model) msgPayload.model_name = analytics.model;
        if (analytics.usage && analytics.usage.total_tokens) {
          msgPayload.tokens_used = analytics.usage.total_tokens;
        }
      }
      const saveRes = await fetch(`/api/chats/${currentChatId}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(msgPayload)
      });
      if (saveRes.ok) {
        const savedMsg = await saveRes.json();
        // Update the element with the real ID and add regenerate button
        const msgGroup = document.querySelector(`[data-msg-id="${assistantMsgId}"]`);
        if (msgGroup && savedMsg.id) {
          msgGroup.dataset.msgId = savedMsg.id;
          const metaDiv = metaEl.querySelector('.message-meta');
          const btnHtml = `<button class="regenerate-btn" onclick="regenerateResponse(${savedMsg.id})" title="Regenerate response"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10"/><path d="M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></button>`;

          if (metaDiv) {
            metaDiv.insertAdjacentHTML('beforeend', btnHtml);
          } else {
            metaEl.innerHTML = `<div class="message-meta">${btnHtml}</div>`;
          }
        }
      }
    } catch (err) {
      console.log('Error saving assistant message:', err);
    }

  } catch (error) {
    console.error('Error:', error);
    const outputEl = document.getElementById(`response-${assistantMsgId}`);
    if (outputEl) {
      outputEl.innerHTML = `<span style="color: #ef4444;">Error: ${escapeHtml(error.message)}</span>`;
    }
  }
}

// System prompt functions
let currentSystemPrompt = '';

async function loadSystemPrompt() {
  if (!currentChatId) return;
  try {
    const res = await fetch(`/api/chats/${currentChatId}/system-prompt`);
    if (res.ok) {
      const data = await res.json();
      currentSystemPrompt = data.system_prompt || '';
      updateSystemPromptUI();
    }
  } catch (err) {
    console.log('Error loading system prompt:', err);
  }
}

function updateSystemPromptUI() {
  const textarea = document.getElementById('systemPromptInput');
  if (textarea) {
    textarea.value = currentSystemPrompt;
  }
  const indicator = document.getElementById('systemPromptIndicator');
  if (indicator) {
    indicator.style.display = currentSystemPrompt ? 'inline' : 'none';
  }
}

async function saveSystemPrompt() {
  const textarea = document.getElementById('systemPromptInput');
  if (!textarea || !currentChatId) return;

  const newPrompt = textarea.value.trim();

  try {
    const res = await fetch(`/api/chats/${currentChatId}/system-prompt`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ system_prompt: newPrompt })
    });

    if (res.ok) {
      currentSystemPrompt = newPrompt;
      updateSystemPromptUI();
      closeSystemPromptModal();
    }
  } catch (err) {
    console.error('Error saving system prompt:', err);
    alert('Failed to save system prompt');
  }
}

function openSystemPromptModal() {
  const modal = document.getElementById('systemPromptModal');
  if (modal) {
    modal.style.display = 'flex';
    loadSystemPrompt();
  }
}

function closeSystemPromptModal() {
  const modal = document.getElementById('systemPromptModal');
  if (modal) {
    modal.style.display = 'none';
  }
}
