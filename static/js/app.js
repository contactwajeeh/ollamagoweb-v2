const MAX_CONVERSATIONS = 3;
const MESSAGE_PAGE_SIZE = 50;
const UNDO_TIMEOUT = 5000;

var converter = new showdown.Converter({
  tables: true,
  tablesHeaderId: true,
  strikethrough: true,
  tasklists: true
});

// Note: ChatState is now defined in state.js as a class
// We need to initialize it with our app-specific defaults
if (typeof ChatState !== 'undefined') {
  // ChatState already exists from state.js
} else {
  // Fallback for when state.js is not loaded
  const ChatState = {
    currentChatId: null,
    chatsList: [],
    csrfToken: null,
    messageOffset: 0,
    hasMoreMessages: true,
    isLoadingMessages: false,
    deletedChatBuffer: null,
    deletedChatTimer: null
  };
  window.ChatState = ChatState;
}

// ============================================
// Error Boundary & Global Error Handling
// ============================================

class ErrorBoundary {
  constructor(onError) {
    this.onError = onError;
  }

  wrap(fn, context = '') {
    return (...args) => {
      try {
        return fn(...args);
      } catch (error) {
        this.handleError(error, context);
        return null;
      }
    };
  }

  handleError(error, context = '') {
    const errorInfo = {
      message: error.message,
      stack: error.stack,
      context,
      timestamp: new Date().toISOString()
    };
    console.error('[ErrorBoundary]', errorInfo);
    notify.error(`An error occurred${context ? ' in ' + context : ''}`);
    if (this.onError) this.onError(errorInfo);
  }
}

// Global error boundary instance
const errorBoundary = new ErrorBoundary((errorInfo) => {
  // Could send to error tracking service here
  console.error('Global error handler:', errorInfo);
});

// Wrap commonly used functions
const safeRender = errorBoundary.wrap((fn, ...args) => fn(...args), 'render');

// Global error handlers
window.addEventListener('error', (event) => {
  errorBoundary.handleError(event.error, 'window.onerror');
  event.preventDefault();
});

window.addEventListener('unhandledrejection', (event) => {
  errorBoundary.handleError(new Error(event.reason), 'unhandledrejection');
  event.preventDefault();
});

// ============================================
// Utility Functions
// ============================================

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

// Safe fetch wrapper with error handling
async function safeFetch(url, options = {}) {
  try {
    const headers = {
      ...options.headers
    };

    if (ChatState.csrfToken) {
      headers['X-CSRF-Token'] = ChatState.csrfToken;
    }

    const response = await fetch(url, {
      ...options,
      headers
    });

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}));
      const errorMsg = errorData.message || `HTTP ${response.status}`;
      notify.error(errorMsg);
      throw new Error(errorMsg);
    }

    return await response.json();
  } catch (error) {
    if (error instanceof TypeError && error.message.includes('fetch')) {
      notify.error('Network error. Please check your connection.');
    }
    console.error('API Error:', error);
    throw error;
  }
}

// Toast notification system
function notify(message, type = 'info', duration = 3000) {
  let container = document.getElementById('toast-container');
  if (!container) {
    container = document.createElement('div');
    container.id = 'toast-container';
    container.style.cssText = 'position:fixed;bottom:20px;right:20px;z-index:9999;display:flex;flex-direction:column;gap:8px;';
    document.body.appendChild(container);
  }

  const toast = document.createElement('div');
  toast.style.cssText = `padding:12px 20px;border-radius:8px;color:white;font-size:14px;box-shadow:0 4px 12px rgba(0,0,0,0.15);animation:slideIn 0.3s ease;background:${type==='error'?'#ef4444':type==='success'?'#22c55e':'#4f39f6'};`;
  toast.textContent = message;
  container.appendChild(toast);

  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateX(100%)';
    toast.style.transition = 'all 0.3s ease';
    setTimeout(() => toast.remove(), 300);
  }, duration);
}

notify.error = (msg) => notify(msg, 'error');
notify.success = (msg) => notify(msg, 'success');

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
  safeFetch('/api/settings/theme', {
    method: 'PUT',
    body: JSON.stringify({ value: next })
  }).catch(() => {});
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

// MCP Tool Management
let mcpTools = [];
let selectedMCPTool = null;

async function loadMCPTools() {
  const selector = document.getElementById('mcpToolSelector');
  if (!selector) {
    console.warn('MCP tool selector element not found');
    return;
  }

  try {
    console.log('Fetching MCP tools from /api/mcp/servers/tools');
    const res = await fetch('/api/mcp/servers/tools');
    console.log('MCP tools response status:', res.status);
    if (!res.ok) {
      console.warn('MCP tools request failed:', res.status);
      selector.classList.remove('visible');
      return;
    }

    const data = await res.json();
    console.log('MCP tools response data:', data);
    mcpTools = data.tools || [];
    console.log('Loaded MCP tools:', mcpTools.length, mcpTools);

    if (mcpTools.length === 0) {
      console.log('No MCP tools found');
      selector.classList.remove('visible');
      return;
    }

    selector.classList.add('visible');
    console.log('Made selector visible');
    const select = document.getElementById('mcpToolSelect');
    select.innerHTML = '<option value="">-- Select Tool --</option>' +
      mcpTools.map(tool =>
        `<option value="${escapeHtml(tool.name)}">${escapeHtml(tool.name)}</option>`
      ).join('');
  } catch (err) {
    console.error('Error loading MCP tools:', err);
    selector.classList.remove('visible');
  }
}

function onMCPToolSelect(e) {
  const toolName = e.target.value;

  if (!toolName) {
    selectedMCPTool = null;
    return;
  }

  selectedMCPTool = mcpTools.find(t => t.name === toolName);
  if (!selectedMCPTool) {
    return;
  }

  showMCPToolModal();
}

function showMCPToolModal() {
  const modal = document.getElementById('mcpToolModal');
  const form = document.getElementById('mcpToolParamsForm');
  renderToolParamsForm(selectedMCPTool, form);
  modal.classList.add('show');
  modal.style.display = 'block';

  // Create new backdrop specifically for this modal
  let backdrop = document.getElementById('mcpToolBackdrop');
  if (backdrop) {
    backdrop.remove();
  }
  backdrop = document.createElement('div');
  backdrop.id = 'mcpToolBackdrop';
  backdrop.className = 'modal-backdrop show';
  backdrop.style.zIndex = '1049';
  modal.style.zIndex = '1050';
  backdrop.onclick = function(e) {
    // Only close if clicking directly on backdrop, not if event propagated
    if (e.target === backdrop) {
      closeMCPToolModal();
    }
  };
  document.body.appendChild(backdrop);

  // Handle Escape key
  const handleEsc = (e) => {
    if (e.key === 'Escape') {
      closeMCPToolModal();
    }
  };
  document.addEventListener('keydown', handleEsc);

  // Store handler on modal to remove later
  modal._escHandler = handleEsc;

  // Focus first input
  setTimeout(() => {
    const firstInput = modal.querySelector('input, textarea, select');
    if (firstInput) firstInput.focus();
  }, 100);
}

function closeMCPToolModal() {
  const modal = document.getElementById('mcpToolModal');
  modal.classList.remove('show');
  modal.style.display = 'none';
  modal.style.zIndex = '';

  // Remove Escape handler
  if (modal._escHandler) {
    document.removeEventListener('keydown', modal._escHandler);
    delete modal._escHandler;
  }

  // Remove specific backdrop
  const backdrop = document.getElementById('mcpToolBackdrop');
  if (backdrop) {
    backdrop.remove();
  }

  // Reset form
  document.getElementById('mcpToolSelect').value = '';
  selectedMCPTool = null;
}

function renderToolParamsForm(tool, container) {
  const schema = tool.input_schema || {};
  const properties = schema.properties || {};
  const required = schema.required || [];

  // Update modal title and description
  document.getElementById('mcpToolModalTitle').textContent = `🔧 Run Tool: ${escapeHtml(tool.name)}`;
  document.getElementById('mcpToolDescription').textContent = tool.description || '';

  if (Object.keys(properties).length === 0) {
    container.innerHTML = '<p class="text-muted small">No parameters required</p>';
    return;
  }

  let html = '';
  for (const [name, prop] of Object.entries(properties)) {
    const isRequired = required.includes(name);
    const type = prop.type || 'string';
    const description = prop.description || '';
    const placeholder = prop.default || '';

    const inputType = type === 'integer' || type === 'number' ? 'number' : (type === 'boolean' ? 'checkbox' : 'text');
    const inputClass = type === 'boolean' ? '' : (type === 'string' && (prop.enum || name.length > 50)) ? 'mcp-param-textarea' : 'mcp-param-input';

    html += `<div class="mcp-param-group">
      <label class="mcp-param-label" for="mcp-param-${escapeHtml(name)}">
        ${escapeHtml(name)}${isRequired ? ' *' : ''}
      </label>
      ${type === 'boolean'
        ? `<input type="checkbox" id="mcp-param-${escapeHtml(name)}" name="${escapeHtml(name)}" class="mcp-param-input">`
        : `<input type="${inputType}" id="mcp-param-${escapeHtml(name)}" name="${escapeHtml(name)}"
          class="mcp-param-input" placeholder="${escapeHtml(placeholder)}"
          ${prop.minimum !== undefined ? `min="${prop.minimum}"` : ''}
          ${prop.maximum !== undefined ? `max="${prop.maximum}"` : ''}>`
      }
      ${description ? `<small class="text-muted">${escapeHtml(description)}</small>` : ''}
    </div>`;
  }

  container.innerHTML = html;
}

async function runMCPTool() {
  if (!selectedMCPTool) return;

  const runBtn = document.getElementById('mcpToolRunBtn');
  runBtn.disabled = true;
  runBtn.textContent = 'Running...';

  try {
    const schema = selectedMCPTool.input_schema || {};
    const properties = schema.properties || {};
    const required = schema.required || [];

    const arguments = {};
    for (const [name, prop] of Object.entries(properties)) {
      const input = document.getElementById(`mcp-param-${name}`);
      if (!input) continue;

      const type = prop.type || 'string';
      if (type === 'boolean') {
        arguments[name] = input.checked;
      } else if (type === 'integer') {
        arguments[name] = parseInt(input.value) || 0;
      } else if (type === 'number') {
        arguments[name] = parseFloat(input.value) || 0;
      } else {
        arguments[name] = input.value;
      }
    }

    // Validate required fields
    for (const field of required) {
      if (arguments[field] === undefined || arguments[field] === '' || arguments[field] === null) {
        alert(`Please fill in the required field: ${field}`);
        runBtn.disabled = false;
        runBtn.textContent = 'Run Tool';
        return;
      }
    }

    // Find server ID from tool name (format: servername_toolname)
    const serverId = mcpTools.find(t => t.name === selectedMCPTool.name)?.server_id;

    const res = await fetch('/api/mcp/servers/call', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        server_id: serverId,
        tool_name: selectedMCPTool.name,
        arguments
      })
    });

    if (!res.ok) {
      const error = await res.text();
      throw new Error(error);
    }

    const result = await res.json();

    // Display result in chat
    addMessage({
      role: 'assistant',
      content: `**MCP Tool Result (${selectedMCPTool.name}):**\n\n${result.result || 'No output'}`,
      done: true
    });

    // Close modal and reset after successful run
    closeMCPToolModal();

  } catch (err) {
    console.error('Error running MCP tool:', err);
    addMessage({
      role: 'assistant',
      content: `**Error running MCP tool:** ${err.message}`,
      done: true
    });
  } finally {
    runBtn.disabled = false;
    runBtn.textContent = 'Run Tool';
  }
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', async function () {
  initTheme();

  // Fetch CSRF token
  try {
    const csrfRes = await fetch('/api/csrf');
    if (csrfRes.ok) {
      const data = await csrfRes.json();
      csrfToken = data.token;
    }
  } catch (e) {
    console.warn('Could not fetch CSRF token:', e);
  }

  // Load models for dropdown
  await loadModels();

  // Load MCP tools
  await loadMCPTools();

  // Set up MCP tool run button
  const mcpRunBtn = document.getElementById('mcpToolRunBtn');
  if (mcpRunBtn) {
    mcpRunBtn.addEventListener('click', runMCPTool);
  }

  // Set up MCP tool select change handler
  const mcpToolSelect = document.getElementById('mcpToolSelect');
  if (mcpToolSelect) {
    mcpToolSelect.addEventListener('change', onMCPToolSelect);
  }

  // Load chats list and current chat
  await loadChatsList();
  await loadCurrentChat();

  // Set up lazy loading for large chats
  setupLazyLoading();

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
    saveBtn.addEventListener('click', showExportMenu);
  }

  // New chat button handler
  const newChatBtn = document.getElementById('btnNewChat');
  if (newChatBtn) {
    newChatBtn.addEventListener('click', startNewChat);
  }

  // Floating new chat button (mobile)
  const floatingNewChatBtn = document.getElementById('btnFloatingNewChat');
  if (floatingNewChatBtn) {
    floatingNewChatBtn.addEventListener('click', startNewChat);
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
    chatSearch.addEventListener('input', debounce(function () {
      filterChats(chatSearch.value);
    }, 300));
  }

  // Set up keyboard shortcuts
  setupKeyboardShortcuts();
});

function setupKeyboardShortcuts() {
  document.addEventListener('keydown', (e) => {
    const key = [
      e.ctrlKey ? 'Ctrl' : '',
      e.metaKey ? 'Cmd' : '',
      e.altKey ? 'Alt' : '',
      e.shiftKey ? 'Shift' : '',
      e.key
    ].filter(Boolean).join('+');

    if (KeyboardShortcuts[key]) {
      e.preventDefault();
      const action = KeyboardShortcutsActions[KeyboardShortcuts[key]];
      if (action) action(e);
    }
  });

  // Show keyboard shortcuts help
  document.addEventListener('keydown', (e) => {
    if (e.key === '?' && e.shiftKey) {
      e.preventDefault();
      showKeyboardShortcutsHelp();
    }
  });
}

function navigateChatSelection(e) {
  const chats = getChatsList();
  if (chats.length === 0) return;

  const activeChat = document.querySelector('.chat-item.active');
  const allChats = Array.from(document.querySelectorAll('.chat-item[data-chat-id]'));

  if (allChats.length === 0) return;

  const currentIndex = activeChat ? allChats.indexOf(activeChat) : -1;
  let newIndex = currentIndex;

  if (e.key === 'ArrowUp') {
    newIndex = Math.max(0, currentIndex - 1);
  } else if (e.key === 'ArrowDown') {
    newIndex = Math.min(allChats.length - 1, currentIndex + 1);
  }

  if (newIndex !== currentIndex) {
    const nextChat = chats[newIndex];
    if (nextChat) {
      selectChat(nextChat.id);
    }
  }
}

let selectedChatIdForAction = null;

function deleteSelectedChat() {
  if (selectedChatIdForAction) {
    confirmDeleteChat(selectedChatIdForAction);
    selectedChatIdForAction = null;
  }
}

function showKeyboardShortcutsHelp() {
  if (document.querySelector('.shortcuts-modal-overlay')) {
    return;
  }

  const shortcuts = [
    { keys: 'Ctrl + N', action: 'New chat', icon: '✨' },
    { keys: 'Ctrl + S', action: 'Export chat', icon: '💾' },
    { keys: 'Ctrl + /', action: 'Focus input', icon: '📝' },
    { keys: 'Ctrl + [', action: 'Close sidebar', icon: '◀' },
    { keys: 'Ctrl + ]', action: 'Open sidebar', icon: '▶' },
    { keys: '↑ / ↓', action: 'Navigate chats', icon: '🗂️' },
    { keys: 'Delete', action: 'Delete chat', icon: '🗑️' },
    { keys: 'Shift + ?', action: 'Show this help', icon: '❓' },
  ];

  const overlay = document.createElement('div');
  overlay.className = 'shortcuts-modal-overlay';
  overlay.onclick = (e) => {
    if (e.target === overlay) closeKeyboardShortcutsHelp();
  };

  overlay.innerHTML = `
    <div class="shortcuts-modal">
      <div class="shortcuts-header">
        <h3>Keyboard Shortcuts</h3>
        <button class="shortcuts-close" onclick="closeKeyboardShortcutsHelp()" aria-label="Close">×</button>
      </div>
      <div class="shortcuts-list">
        ${shortcuts.map(s => `
          <div class="shortcut-item">
            <span class="shortcut-keys">
              <kbd>${s.keys.split(' + ').join('</kbd><kbd>')}</kbd>
            </span>
            <span class="shortcut-action">${s.action}</span>
          </div>
        `).join('')}
      </div>
      <div class="shortcuts-footer">
        <p>Press <kbd>Esc</kbd> or click outside to close</p>
      </div>
    </div>
  `;

  document.body.appendChild(overlay);
  document.body.style.overflow = 'hidden';

  setTimeout(() => overlay.classList.add('show'), 10);

  const handleEsc = (e) => {
    if (e.key === 'Escape') {
      closeKeyboardShortcutsHelp();
      document.removeEventListener('keydown', handleEsc);
    }
  };
  document.addEventListener('keydown', handleEsc);
}

function closeKeyboardShortcutsHelp() {
  const overlay = document.querySelector('.shortcuts-modal-overlay');
  if (overlay) {
    overlay.classList.remove('show');
    document.body.style.overflow = '';
    setTimeout(() => overlay.remove(), 200);
  }
}

// Utility function - Debounce
function debounce(func, wait) {
  let timeout;
  return function executedFunction(...args) {
    const later = () => {
      clearTimeout(timeout);
      func(...args);
    };
    clearTimeout(timeout);
    timeout = setTimeout(later, wait);
  };
}

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

function openSidebar() {
  const sidebar = document.getElementById('sidebar');
  sidebar.classList.add('open');
  let overlay = document.querySelector('.sidebar-overlay');
  if (!overlay) {
    overlay = document.createElement('div');
    overlay.className = 'sidebar-overlay';
    document.body.appendChild(overlay);
  }
  overlay.classList.add('show');
}

function getChatsList() {
  return chatsList;
}

// Load chats list for sidebar
async function loadChatsList() {
  try {
    const res = await fetch('/api/chats');
    if (!res.ok) return;

    ChatState.chatsList = await res.json();
    renderChatsList();
  } catch (err) {
    console.log('Error loading chats:', err);
    document.getElementById('chatList').innerHTML =
      '<div class="chat-list-loading">Error loading chats</div>';
  }
}

function renderChatsList(filter = '') {
  const container = document.getElementById('chatList');
  const pinnedContainer = document.getElementById('pinnedChatList');

  if (!ChatState.chatsList || ChatState.chatsList.length === 0) {
    container.innerHTML = '<div class="chat-list-loading">No chats yet</div>';
    pinnedContainer.innerHTML = '';
    return;
  }

  const filteredChats = filter
    ? ChatState.chatsList.filter(chat => chat.title.toLowerCase().includes(filter.toLowerCase()))
    : ChatState.chatsList;

  if (filteredChats.length === 0) {
    container.innerHTML = '<div class="chat-list-loading">No matching chats</div>';
    pinnedContainer.innerHTML = '';
    return;
  }

  const pinnedChats = filteredChats.filter(chat => chat.is_pinned);
  const regularChats = filteredChats.filter(chat => !chat.is_pinned);

  pinnedContainer.innerHTML = pinnedChats.map(chat => createChatItemHtml(chat)).join('');
  container.innerHTML = regularChats.length > 0
    ? regularChats.map(chat => createChatItemHtml(chat)).join('')
    : '<div class="chat-list-loading">No chats yet</div>';

  updateChatItemListeners();
}

function createChatItemHtml(chat) {
  const isActive = String(chat.id) === String(ChatState.currentChatId);
  return `
    <div class="chat-item ${isActive ? 'active' : ''} ${chat.is_pinned ? 'pinned' : ''}"
         data-chat-id="${chat.id}" onclick="selectChat(${chat.id})">
      <div class="chat-item-content">
        <div class="chat-item-title">
            ${chat.is_pinned ? '<span class="pin-indicator" title="Pinned">📌</span> ' : ''}${escapeHtml(chat.title).replace(/^\/search\s+/, '<span class="search-pill">SEARCH</span> ')}
        </div>
      </div>
      <div class="chat-item-actions" onclick="event.stopPropagation()">
        <button class="chat-item-pin ${chat.is_pinned ? 'pinned' : ''}" onclick="togglePinChat(${chat.id}, ${!chat.is_pinned}, event)" title="${chat.is_pinned ? 'Unpin chat' : 'Pin chat'}">📌</button>
        <button class="chat-item-rename" onclick="renameChat(${chat.id}, event)" title="Rename chat">✏️</button>
        <button class="chat-item-delete" onclick="deleteChat(${chat.id}, event)" title="Delete chat">×</button>
      </div>
    </div>
  `;
}

function updateChatItemListeners() {
  document.querySelectorAll('.chat-item').forEach(item => {
    item.addEventListener('click', () => {
      selectedChatIdForAction = item.dataset.chatId;
    });
  });
}

// Filter chats by search query (searches titles and message content via API)
function filterChats(query) {
  const q = query.trim();

  if (!q) {
    // No query, show all chats from cache
    renderChatsList('');
    return;
  }

  // API call is now debounced at the event listener level
  fetch(`/api/chats/search?q=${encodeURIComponent(q)}`)
    .then(res => res.json())
    .then(results => renderSearchResults(results, q))
    .catch(err => {
      console.log('Search error:', err);
      renderChatsList(q);
    });
}

function renderSearchResults(results, query) {
  const container = document.getElementById('chatList');
  const pinnedContainer = document.getElementById('pinnedChatList');

  pinnedContainer.innerHTML = '';

  if (results.length === 0) {
    container.innerHTML = '<div class="chat-list-loading">No matching chats</div>';
    return;
  }

  container.innerHTML = results.map(chat => `
    <div class="chat-item ${String(chat.id) === String(ChatState.currentChatId) ? 'active' : ''}"
         data-chat-id="${chat.id}" onclick="selectChat(${chat.id})">
      <div class="chat-item-content">
        <div class="chat-item-title">${escapeHtml(chat.title)}</div>
        <div class="chat-item-date">${formatDate(chat.updated_at)}</div>
      </div>
      <div class="chat-item-actions" onclick="event.stopPropagation()">
        <button class="chat-item-rename" onclick="renameChat(${chat.id}, event)" title="Rename chat">✏️</button>
        <button class="chat-item-delete" onclick="deleteChat(${chat.id}, event)" title="Delete chat">×</button>
      </div>
    </div>
  `).join('');

  updateChatItemListeners();
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

// Render messages from chat data - shared between selectChat and loadCurrentChat
function renderMessages(chat, options = {}) {
  const printout = document.getElementById('printout');
  if (!printout) return;

  printout.innerHTML = '';

  if (!chat.messages || chat.messages.length === 0) {
    printout.innerHTML = `
      <div class="welcome-message" id="welcomeMessage">
        <div class="welcome-icon">🦙</div>
        <h2>Welcome to OllamaGoWeb</h2>
        <p>Start a conversation by typing a message below.</p>
        <p class="welcome-hint">Press <kbd>Ctrl</kbd> + <kbd>Enter</kbd> to send</p>
      </div>
    `;
    return;
  }

  const welcome = document.getElementById('welcomeMessage');
  if (welcome) welcome.style.display = 'none';

  const versionGroups = {};
  for (const msg of chat.messages) {
    if (msg.version_group) {
      if (!versionGroups[msg.version_group]) {
        versionGroups[msg.version_group] = [];
      }
      versionGroups[msg.version_group].push(msg);
    }
  }

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
      const versionCount = countVersions(chat.messages, msg.id);
      if (msg.role === 'user') {
        printout.insertAdjacentHTML('beforeend', createUserMessageHtml(msg.id, msg.content, versionCount));
      } else {
        const meta = {};
        if (msg.model_name) meta.model = msg.model_name;
        if (msg.tokens_used) meta.tokens = msg.tokens_used;
        printout.insertAdjacentHTML('beforeend', createAssistantMessageHtml(msg.id, msg.content, true, meta, versionCount));
      }
      i++;
    }
  }

  if (options.setupCopyButtons !== false) {
    setupCodeBlockCopyButtons();
    wrapTablesInMessage(printout);
  }
}

// Select a chat from sidebar
async function selectChat(chatId) {
  if (chatId === ChatState.currentChatId) {
    closeSidebar();
    return;
  }

  ChatState.messageOffset = 0;
  ChatState.hasMoreMessages = true;
  ChatState.isLoadingMessages = false;

  try {
    const res = await fetch(`/api/chats/${chatId}`);
    if (!res.ok) return;

    const chat = await res.json();
    ChatState.currentChatId = chat.id;

    document.querySelectorAll('.chat-item').forEach(item => {
      item.classList.toggle('active', String(item.dataset.chatId) === String(chatId));
    });

    renderMessages(chat);
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

  try {
    const res = await fetch(`/api/chats/${chatId}`, { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete');

    const chatToDelete = ChatState.chatsList.find(c => c.id === chatId);
    if (chatToDelete) {
      ChatState.deletedChatBuffer = { chat: chatToDelete, timestamp: Date.now() };
      showUndoNotification(chatToDelete.title);
    }

    ChatState.chatsList = ChatState.chatsList.filter(c => c.id !== chatId);
    renderChatsList();

    if (chatId === ChatState.currentChatId) {
      if (ChatState.chatsList.length > 0) {
        await selectChat(ChatState.chatsList[0].id);
      } else {
        await startNewChat();
      }
    }
  } catch (err) {
    console.log('Error deleting chat:', err);
    notify.error('Failed to delete chat');
  }
}

function showUndoNotification(chatTitle) {
  if (ChatState.deletedChatTimer) {
    clearTimeout(ChatState.deletedChatTimer);
  }

  const container = document.getElementById('toast-container') || createToastContainer();
  const toast = document.createElement('div');
  toast.className = 'undo-toast';
  toast.innerHTML = `
    <span>Chat "${escapeHtml(chatTitle)}" deleted</span>
    <button onclick="undoDeleteChat()">Undo</button>
  `;
  container.appendChild(toast);

  ChatState.deletedChatTimer = setTimeout(() => {
    toast.remove();
    ChatState.deletedChatBuffer = null;
  }, UNDO_TIMEOUT);
}

function createToastContainer() {
  const container = document.createElement('div');
  container.id = 'toast-container';
  container.style.cssText = 'position:fixed;bottom:80px;left:20px;z-index:9999;display:flex;flex-direction:column;gap:8px;';
  document.body.appendChild(container);
  return container;
}

async function undoDeleteChat() {
  if (!ChatState.deletedChatBuffer) return;

  try {
    const chat = ChatState.deletedChatBuffer.chat;
    const res = await fetch('/api/chats', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: chat.title })
    });

    if (res.ok) {
      const data = await res.json();
      ChatState.chatsList.unshift({ id: data.id, title: chat.title, updated_at: chat.updated_at });
      renderChatsList();
      notify.success('Chat restored');
    }
  } catch (err) {
    console.error('Failed to restore chat:', err);
  }

  ChatState.deletedChatBuffer = null;
  if (ChatState.deletedChatTimer) {
    clearTimeout(ChatState.deletedChatTimer);
    ChatState.deletedChatTimer = null;
  }

  const toast = document.querySelector('.undo-toast');
  if (toast) toast.remove();
}

// Delete chat with confirmation
async function confirmDeleteChat(chatId) {
  if (!confirm('Delete this chat?')) return;
  const event = { stopPropagation: () => {} };
  await deleteChat(chatId, event);
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

// Toggle chat pinned status
async function togglePinChat(chatId, isPinned, event) {
  event.stopPropagation();
  try {
    const res = await fetch(`/api/chats/${chatId}/pin`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ is_pinned: isPinned })
    });

    if (res.ok) {
      // Reload list to re-sort
      loadChatsList();
    }
  } catch (err) {
    console.log('Error pinning chat:', err);
  }
}

// Load current/most recent chat
async function loadCurrentChat() {
  ChatState.messageOffset = 0;
  ChatState.hasMoreMessages = true;
  ChatState.isLoadingMessages = false;

  try {
    const res = await fetch('/api/chats/current');
    if (!res.ok) return;

    const chat = await res.json();
    ChatState.currentChatId = chat.id;

    renderChatsList();
    renderMessages(chat);

    if (chat.messages && chat.messages.length > 0) {
      updateProgressBar(chat.messages.length);
    }
    scrollToBottom();
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
      ChatState.currentChatId = data.id;

      ChatState.chatsList.unshift({ id: data.id, title: data.title, updated_at: new Date().toISOString() });
      renderChatsList();

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

// ============================================
// Keyboard Shortcuts
// ============================================

const KeyboardShortcuts = {
  'Ctrl+N': 'startNewChat',
  'Ctrl+S': 'exportChat',
  'Ctrl+/': 'focusPrompt',
  'Ctrl+[': 'closeSidebar',
  'Ctrl+]': 'openSidebar',
  'Escape': 'closeModal',
  'ArrowUp': 'navigateChatSelection',
  'ArrowDown': 'navigateChatSelection',
  'Delete': 'deleteSelectedChat',
};

const KeyboardShortcutsActions = {
  startNewChat,
  exportChat,
  focusPrompt: () => document.getElementById('prompt')?.focus(),
  closeSidebar,
  openSidebar,
  closeModal: () => {
    const modal = document.getElementById('systemPromptModal');
    if (modal && modal.style.display !== 'none') {
      closeSystemPromptModal();
    }
  },
  navigateChatSelection,
  deleteSelectedChat,
};

function createUserMessageHtml(id, content, versionCount = 0) {
  const versionBadge = versionCount > 1
    ? `<span class="version-badge" onclick="showVersionHistory('${id}')" title="${versionCount - 1} earlier version${versionCount > 2 ? 's' : ''}">📜 ${versionCount}</span>`
    : '';
  return `
    <div id="msg-${id}" class="message-group user-message-group">
      <div class="prompt-message">
        ${versionBadge}
        <span class="message-content">${escapeHtml(content)}</span>
        <div class="message-actions">
           <button class="message-btn edit-btn" onclick="editMessage(${id}, 'user')" title="Edit message">✏️</button>
           <button class="message-btn delete-btn" onclick="deleteMessage(${id})" title="Delete message">🗑️</button>
        </div>
      </div>
    </div>
  `;
}

function createAssistantMessageHtml(id, content, isFormatted = false, meta = {}, versionCount = 0) {
  const formattedContent = isFormatted ? converter.makeHtml(content) : content;

  // Build metadata HTML
  let metaHtml = '';
  // Don't show controls for pending messages
  const showControls = id && !String(id).startsWith('pending');

  const hasMeta = meta.model || meta.tokens || meta.speed;
  if (hasMeta || showControls) {
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

    const versionBadge = versionCount > 1
      ? `<span class="version-badge" onclick="showVersionHistory('${id}')" title="${versionCount - 1} earlier version${versionCount > 2 ? 's' : ''}">📜 ${versionCount}</span>`
      : '';

    if (versionBadge) {
      metaHtml += versionBadge;
    }

    if (showControls) {
      metaHtml += `<button class="message-btn regenerate-btn" onclick="regenerateResponse(${id})" title="Regenerate response"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10"/><path d="M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></button>`;
      metaHtml += `<button class="message-btn delete-btn" onclick="deleteMessage(${id})" title="Delete message"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path><line x1="10" y1="11" x2="10" y2="17"></line><line x1="14" y1="11" x2="14" y2="17"></line></svg></button>`;
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

// Count versions for a message
function countVersions(messages, msgId) {
  let count = 0;
  const msg = messages.find(m => m.id === msgId);
  if (!msg || !msg.version_group) return 0;

  const versionGroup = msg.version_group;
  count = messages.filter(m => m.version_group === versionGroup).length;
  return count;
}

// Show version history modal
async function showVersionHistory(msgId) {
  if (!ChatState.currentChatId) return;

  const modal = document.createElement('div');
  modal.className = 'version-modal-overlay';
  modal.innerHTML = `
    <div class="version-modal">
      <div class="version-modal-header">
        <h3>Version History</h3>
        <button class="version-modal-close" onclick="closeVersionModal()">×</button>
      </div>
      <div class="version-modal-content" id="versionModalContent">
        <div class="version-loading">Loading versions...</div>
      </div>
    </div>
  `;
  document.body.appendChild(modal);

  setTimeout(() => modal.classList.add('show'), 10);

  try {
    const res = await fetch(`/api/chats/${ChatState.currentChatId}`);
    if (!res.ok) throw new Error('Failed to load chat');
    const chat = await res.json();

    const versions = {};
    chat.messages.forEach(m => {
      if (m.version_group) {
        if (!versions[m.version_group]) versions[m.version_group] = [];
        versions[m.version_group].push(m);
      }
    });

    let versionGroup = null;
    const msg = chat.messages.find(m => m.id === msgId);
    if (msg && msg.version_group) {
      versionGroup = versions[msg.version_group];
    }

    if (!versionGroup || versionGroup.length === 0) {
      document.getElementById('versionModalContent').innerHTML = '<p>No version history available</p>';
      return;
    }

    versionGroup.sort((a, b) => a.id - b.id);

    let html = '<div class="version-list">';
    versionGroup.forEach((v, idx) => {
      const isLatest = idx === versionGroup.length - 1;
      const roleLabel = v.role === 'user' ? 'You' : 'Assistant';
      const roleClass = v.role === 'user' ? 'version-role-user' : 'version-role-assistant';
      html += `
        <div class="version-item-modal ${isLatest ? 'latest' : ''}">
          <div class="version-header">
            <span class="version-badge-label ${roleClass}">${roleLabel}</span>
            <span class="version-number">v${idx + 1}${isLatest ? ' (current)' : ''}</span>
            <span class="version-time">${new Date(v.created_at || Date.now()).toLocaleString()}</span>
          </div>
          <div class="version-content">${escapeHtml(v.content)}</div>
          ${isLatest ? '' : `<button class="version-restore-btn" onclick="restoreVersion(${v.id})">Restore this version</button>`}
        </div>
      `;
    });
    html += '</div>';
    document.getElementById('versionModalContent').innerHTML = html;
  } catch (err) {
    document.getElementById('versionModalContent').innerHTML = `<p class="version-error">Error loading versions: ${err.message}</p>`;
  }
}

function closeVersionModal() {
  const modal = document.querySelector('.version-modal-overlay');
  if (modal) {
    modal.classList.remove('show');
    setTimeout(() => modal.remove(), 300);
  }
}

async function restoreVersion(msgId) {
  if (!confirm('Restore this version? Your current version will be added to history.')) return;

  const printout = document.getElementById('printout');
  if (!printout) return;

  closeVersionModal();

  const userMsg = document.querySelector('.version-item-modal .version-role-user');
  if (userMsg) {
    const content = userMsg.nextElementSibling?.textContent;
    if (content) {
      await editMessage(msgId, 'user');
      setTimeout(async () => {
        const textarea = document.querySelector('.inline-edit-textarea');
        if (textarea) {
          textarea.value = content;
          textarea.dataset.originalContent = content;
          const saveBtn = document.querySelector('.inline-edit-btn.save');
          if (saveBtn) saveBtn.click();
        }
      }, 100);
    }
  }
}

document.addEventListener('click', (e) => {
  if (e.target.classList.contains('version-modal-overlay')) {
    closeVersionModal();
  }
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeVersionModal();
});

// Copy to clipboard functionality
async function copyToClipboard(text, buttonElement = null) {
  try {
    await navigator.clipboard.writeText(text);
    notify.success('Copied to clipboard');

    // Visual feedback on button
    if (buttonElement) {
      const originalContent = buttonElement.innerHTML;
      buttonElement.innerHTML = '✓';
      setTimeout(() => {
        buttonElement.innerHTML = originalContent;
      }, 1500);
    }
  } catch (err) {
    notify.error('Failed to copy');
    console.error('Copy failed:', err);
  }
}

// Add copy buttons to code blocks
function setupCodeBlockCopyButtons() {
  document.querySelectorAll('.response-message pre').forEach(pre => {
    // Check if already has copy button
    if (pre.parentElement.querySelector('.copy-code-btn')) return;

    const button = document.createElement('button');
    button.className = 'copy-code-btn';
    button.innerHTML = '📋';
    button.title = 'Copy code';
    button.setAttribute('aria-label', 'Copy code to clipboard');

    button.addEventListener('click', () => {
      const code = pre.querySelector('code');
      const text = code ? code.textContent : pre.textContent;
      copyToClipboard(text, button);
    });

    pre.style.position = 'relative';
    pre.appendChild(button);
  });
}

// Delete a message
async function deleteMessage(id) {
  if (!confirm('Are you sure you want to delete this message?')) return;

  try {
    const res = await fetch(`/api/messages/${id}`, { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete message');

    // Remove from UI
    const msgElement = document.getElementById(`msg-${id}`) || document.querySelector(`[data-msg-id="${id}"]`);
    if (msgElement) {
      // If it's part of a version group, we might need to handle it differently, 
      // but simple removal works for now as basic implementation.
      // If it's a version item, remove the item.
      if (msgElement.closest('.version-item')) {
        msgElement.closest('.version-item').remove();
        // Refesh version nav... simplified for now: just reload chat if complex
      } else {
        msgElement.remove();
      }
    }
  } catch (err) {
    console.error('Error deleting message:', err);
    alert('Failed to delete message');
  }
}

async function sendMessage() {
  const textarea = document.getElementById('prompt');
  const prompt = textarea.value.trim();

  if (!prompt) return;

  if (!ChatState.currentChatId) {
    const res = await fetch('/api/chats', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: 'New Chat' })
    });
    if (res.ok) {
      const data = await res.json();
      ChatState.currentChatId = data.id;
      ChatState.chatsList.unshift({ id: data.id, title: data.title, updated_at: new Date().toISOString() });
      renderChatsList();
    } else {
      console.error('Failed to create chat');
      return;
    }
  }

  const welcome = document.getElementById('welcomeMessage');
  if (welcome) welcome.style.display = 'none';

  textarea.value = '';
  textarea.style.height = 'auto';

  let userMsgId;
  try {
    const res = await fetch(`/api/chats/${ChatState.currentChatId}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ role: 'user', content: prompt })
    });
    if (res.ok) {
      const data = await res.json();
      userMsgId = data.id;

      const titlePreview = prompt.length > 30 ? prompt.substring(0, 30) + '...' : prompt;
      const chatItem = ChatState.chatsList.find(c => c.id === ChatState.currentChatId);
      if (chatItem && chatItem.title === 'New Chat') {
        chatItem.title = titlePreview;
        renderChatsList();
      }
    }
  } catch (err) {
    console.log('Error saving user message:', err);
  }

  const printout = document.getElementById('printout');
  printout.insertAdjacentHTML('beforeend', createUserMessageHtml(userMsgId || Date.now(), prompt));
  announce('Message sent. Waiting for AI response...');

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
      body: JSON.stringify({ input: prompt, chat_id: ChatState.currentChatId })
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
    setupCodeBlockCopyButtons();

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
      await fetch(`/api/chats/${ChatState.currentChatId}/messages`, {
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
    fetch(`/api/chats/${ChatState.currentChatId}`)
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

// Announce to screen readers
function announce(message) {
  const region = document.getElementById('a11y-announcements');
  if (region) {
    region.textContent = message;
    // Clear after announcement
    setTimeout(() => { region.textContent = ''; }, 1000);
  }
}

// Export menu handlers
function showExportMenu(event) {
  event?.stopPropagation?.();
  const menu = document.getElementById('exportMenu');
  if (menu) {
    menu.style.display = '';
    menu.classList.add('show');
  }
}

function closeExportMenu() {
  const menu = document.getElementById('exportMenu');
  if (menu) {
    menu.classList.remove('show');
    menu.style.display = 'none';
  }
}

// Close export menu when clicking outside
document.addEventListener('click', (e) => {
  const menu = document.getElementById('exportMenu');
  const menuContent = menu?.querySelector('.modal-content');
  if (menu && menu.classList.contains('show') && !menuContent.contains(e.target)) {
    closeExportMenu();
  }
});

// ============================================
// Skeleton Loading UI
// ============================================

function createUserSkeleton() {
  return `
    <div class="loading-skeleton user-message-group">
      <div class="skeleton skeleton-avatar"></div>
      <div class="skeleton-content">
        <div class="skeleton skeleton-text"></div>
        <div class="skeleton skeleton-text short"></div>
      </div>
    </div>
  `;
}

function createAssistantSkeleton() {
  return `
    <div class="loading-skeleton assistant-message-group">
      <div class="skeleton skeleton-avatar"></div>
      <div class="skeleton-content">
        <div class="skeleton skeleton-text"></div>
        <div class="skeleton skeleton-text"></div>
        <div class="skeleton skeleton-text medium"></div>
      </div>
    </div>
  `;
}

function showLoadingSkeleton(count = 3) {
  const printout = document.getElementById('printout');
  if (!printout) return;

  let html = '';
  for (let i = 0; i < count; i++) {
    html += createAssistantSkeleton();
  }
  printout.innerHTML = html;
}

function removeLoadingSkeleton() {
  const printout = document.getElementById('printout');
  if (!printout) return;

  const skeletons = printout.querySelectorAll('.loading-skeleton');
  skeletons.forEach(s => s.remove());
}

// ============================================
// Lazy Loading for Large Chats
// ============================================

async function loadMoreMessages() {
  if (ChatState.isLoadingMessages || !ChatState.hasMoreMessages || !ChatState.currentChatId) return;

  ChatState.isLoadingMessages = true;
  const nextOffset = ChatState.messageOffset + MESSAGE_PAGE_SIZE;

  try {
    const res = await fetch(`/api/chats/${ChatState.currentChatId}?limit=${MESSAGE_PAGE_SIZE}&offset=${nextOffset}`);
    if (!res.ok) {
      ChatState.hasMoreMessages = false;
      return;
    }

    const chat = await res.json();
    if (!chat.messages || chat.messages.length === 0) {
      ChatState.hasMoreMessages = false;
      return;
    }

    const printout = document.getElementById('printout');
    if (printout && chat.messages.length > 0) {
      const tempDiv = document.createElement('div');
      for (const msg of chat.messages) {
        const versionCount = countVersions(chat.messages, msg.id);
        const msgHtml = msg.role === 'user'
          ? createUserMessageHtml(msg.id, msg.content, versionCount)
          : createAssistantMessageHtml(msg.id, msg.content, false, { model: msg.model_name, tokens: msg.tokens_used }, versionCount);
        tempDiv.innerHTML += msgHtml;
      }

      const loadingIndicator = printout.querySelector('.loading-more-indicator');
      if (loadingIndicator) {
        printout.insertBefore(tempDiv.firstElementChild, loadingIndicator);
      } else {
        printout.insertBefore(tempDiv.firstElementChild, printout.firstChild);
      }

      ChatState.messageOffset = nextOffset;
      if (chat.messages.length < MESSAGE_PAGE_SIZE) {
        ChatState.hasMoreMessages = false;
      }

      setupCodeBlockCopyButtons();
    }
  } catch (err) {
    console.warn('Failed to load more messages:', err);
    ChatState.hasMoreMessages = false;
  } finally {
    ChatState.isLoadingMessages = false;
  }
}

// Set up scroll listener for lazy loading
function setupLazyLoading() {
  const messagesContainer = document.getElementById('chatMessages');
  if (!messagesContainer) return;

  messagesContainer.removeEventListener('scroll', handleMessagesScroll);
  messagesContainer.addEventListener('scroll', handleMessagesScroll, { passive: true });
}

function handleMessagesScroll(e) {
  const container = e.target;
  if (container.scrollTop < 100 && ChatState.hasMoreMessages && !ChatState.isLoadingMessages) {
    loadMoreMessages();
  }
}

function exportChat(format = 'html') {
  const printout = document.getElementById('printout');
  if (!printout) return;

  const date = new Date();
  const timestamp = `${date.getFullYear()}${String(date.getMonth() + 1).padStart(2, '0')}${String(date.getDate()).padStart(2, '0')}_${String(date.getHours()).padStart(2, '0')}${String(date.getMinutes()).padStart(2, '0')}`;
  const fileName = `chat_${timestamp}`;

  if (format === 'json') {
    exportChatJSON(fileName);
    return;
  }

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
  <h1>Chat Export</h1>
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

  announce('Chat exported as HTML');
}

function exportChatJSON(fileName) {
  const printout = document.getElementById('printout');
  if (!printout) return;

  const messages = [];
  printout.querySelectorAll('.message-group').forEach(group => {
    const isUser = group.classList.contains('user-message-group');
    const content = group.querySelector('.message-content, .response-message');
    messages.push({
      role: isUser ? 'user' : 'assistant',
      content: content ? content.textContent.trim() : ''
    });
  });

  const exportData = {
    version: 1,
    exportedAt: new Date().toISOString(),
    messages
  };

  const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${fileName}.json`;
  a.click();
  URL.revokeObjectURL(url);

  announce('Chat exported as JSON');
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
    const userRes = await fetch(`/api/chats/${ChatState.currentChatId}/messages`, {
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
      body: JSON.stringify({ input: prompt, chat_id: ChatState.currentChatId })
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
      const saveRes = await fetch(`/api/chats/${ChatState.currentChatId}/messages`, {
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
      body: JSON.stringify({ input: prompt, chat_id: ChatState.currentChatId })
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
      const saveRes = await fetch(`/api/chats/${ChatState.currentChatId}/messages`, {
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
  if (!ChatState.currentChatId) return;

  try {
    const res = await fetch(`/api/chats/${ChatState.currentChatId}/system-prompt`);
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
  if (!textarea || !ChatState.currentChatId) return;

  const newPrompt = textarea.value.trim();

  try {
    const res = await fetch(`/api/chats/${ChatState.currentChatId}/system-prompt`, {
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

function wrapTablesInMessage(container) {
  if (!container) return;
  container.querySelectorAll('table').forEach(table => {
    if (table.parentElement.classList.contains('table-wrapper')) return;
    const wrapper = document.createElement('div');
    wrapper.className = 'table-wrapper';
    table.parentNode.insertBefore(wrapper, table);
    wrapper.appendChild(table);
  });
}
