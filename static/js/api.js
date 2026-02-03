// ============================================
// API Client Module
// ============================================

const API_BASE = '/api';

// Endpoints
const Endpoints = {
  CHATS: `${API_BASE}/chats`,
  CHAT_CURRENT: `${API_BASE}/chats/current`,
  CHAT_BY_ID: (id) => `${API_BASE}/chats/${id}`,
  CHAT_SEARCH: (q) => `${API_BASE}/chats/search?q=${encodeURIComponent(q)}`,
  CHAT_CREATE: `${API_BASE}/chats`,
  CHAT_UPDATE: (id) => `${API_BASE}/chats/${id}`,
  CHAT_DELETE: (id) => `${API_BASE}/chats/${id}`,
  CHAT_RENAME: (id) => `${API_BASE}/chats/${id}/rename`,
  CHAT_PIN: (id) => `${API_BASE}/chats/${id}/pin`,
  PROVIDERS: `${API_BASE}/providers`,
  PROVIDER_BY_ID: (id) => `${API_BASE}/providers/${id}`,
  PROVIDER_ACTIVATE: (id) => `${API_BASE}/providers/${id}/activate`,
  PROVIDER_FETCH_MODELS: (id) => `${API_BASE}/providers/${id}/fetch-models`,
  MODELS: `${API_BASE}/models`,
  MODEL_BY_ID: (id) => `${API_BASE}/models/${id}`,
  SETTINGS: (key) => `${API_BASE}/settings/${key}`,
  SETTINGS_UPDATE: (key) => `${API_BASE}/settings/${key}`,
  CSRF: `${API_BASE}/csrf`,
  MESSAGES: `${API_BASE}/messages`,
  MESSAGE_BY_ID: (id) => `${API_BASE}/messages/${id}`,
};

// Get CSRF token from global state
function getCsrfToken() {
  return typeof csrfToken !== 'undefined' ? csrfToken : null;
}

// Chat APIs
const ChatAPI = {
  async getAll(limit = 50) {
    const res = await fetch(`${Endpoints.CHATS}?limit=${limit}`);
    if (!res.ok) throw new Error('Failed to fetch chats');
    return res.json();
  },

  async getCurrent() {
    const res = await fetch(Endpoints.CHAT_CURRENT);
    if (!res.ok) return null;
    return res.json();
  },

  async getById(id, limit = 100, offset = 0) {
    const url = new URL(Endpoints.CHAT_BY_ID(id), window.location.origin);
    url.searchParams.set('limit', limit);
    url.searchParams.set('offset', offset);
    const res = await fetch(url);
    if (!res.ok) throw new Error('Failed to fetch chat');
    return res.json();
  },

  async search(query) {
    const res = await fetch(Endpoints.CHAT_SEARCH(query));
    if (!res.ok) throw new Error('Search failed');
    return res.json();
  },

  async create(title = 'New Chat') {
    const res = await fetch(Endpoints.CHAT_CREATE, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title })
    });
    if (!res.ok) throw new Error('Failed to create chat');
    return res.json();
  },

  async update(id, data) {
    const res = await fetch(Endpoints.CHAT_UPDATE(id), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data)
    });
    if (!res.ok) throw new Error('Failed to update chat');
    return res.json();
  },

  async delete(id) {
    const res = await fetch(Endpoints.CHAT_DELETE(id), { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete chat');
    return res.json();
  },

  async rename(id, title) {
    const res = await fetch(Endpoints.CHAT_RENAME(id), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title })
    });
    if (!res.ok) throw new Error('Failed to rename chat');
    return res.json();
  },

  async togglePin(id, isPinned) {
    const res = await fetch(Endpoints.CHAT_PIN(id), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ is_pinned: isPinned })
    });
    if (!res.ok) throw new Error('Failed to toggle pin');
    return res.json();
  }
};

// Message APIs
const MessageAPI = {
  async delete(id) {
    const res = await fetch(Endpoints.MESSAGE_BY_ID(id), { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete message');
    return res.json();
  },

  async update(id, content, versionGroup = '') {
    const res = await fetch(Endpoints.MESSAGE_BY_ID(id), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content, version_group: versionGroup })
    });
    if (!res.ok) throw new Error('Failed to update message');
    return res.json();
  },

  async create(chatId, role, content, versionGroup = '') {
    const res = await fetch(Endpoints.MESSAGES, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        chat_id: chatId,
        role,
        content,
        version_group: versionGroup
      })
    });
    if (!res.ok) throw new Error('Failed to create message');
    return res.json();
  }
};

// Provider APIs
const ProviderAPI = {
  async getAll() {
    const res = await fetch(Endpoints.PROVIDERS);
    if (!res.ok) throw new Error('Failed to fetch providers');
    return res.json();
  },

  async create(data) {
    const res = await fetch(Endpoints.PROVIDERS, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data)
    });
    if (!res.ok) throw new Error('Failed to create provider');
    return res.json();
  },

  async update(id, data) {
    const res = await fetch(Endpoints.PROVIDER_BY_ID(id), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data)
    });
    if (!res.ok) throw new Error('Failed to update provider');
    return res.json();
  },

  async delete(id) {
    const res = await fetch(Endpoints.PROVIDER_BY_ID(id), { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete provider');
    return res.json();
  },

  async activate(id) {
    const res = await fetch(Endpoints.PROVIDER_ACTIVATE(id), { method: 'POST' });
    if (!res.ok) throw new Error('Failed to activate provider');
    return res.json();
  },

  async fetchModels(id) {
    const res = await fetch(Endpoints.PROVIDER_FETCH_MODELS(id), { method: 'POST' });
    if (!res.ok) throw new Error('Failed to fetch models');
    return res.json();
  },

  async getActive() {
    const res = await fetch(`${API_BASE}/active-provider`);
    if (!res.ok) throw new Error('Failed to get active provider');
    return res.json();
  },

  async switchModel(model) {
    const res = await fetch(`${API_BASE}/switch-model`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model })
    });
    if (!res.ok) throw new Error('Failed to switch model');
    return res.json();
  }
};

// Model APIs
const ModelAPI = {
  async create(data) {
    const res = await fetch(Endpoints.MODELS, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data)
    });
    if (!res.ok) throw new Error('Failed to create model');
    return res.json();
  },

  async delete(id) {
    const res = await fetch(Endpoints.MODEL_BY_ID(id), { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete model');
    return res.json();
  },

  async setDefault(id) {
    const res = await fetch(Endpoints.MODEL_BY_ID(id) + '/set-default', { method: 'POST' });
    if (!res.ok) throw new Error('Failed to set default model');
    return res.json();
  },

  async getForProvider(providerId) {
    const res = await fetch(Endpoints.MODELS.replace('/models', `/models/${providerId}`));
    if (!res.ok) throw new Error('Failed to get models');
    return res.json();
  }
};

// Settings APIs
const SettingsAPI = {
  async get(key) {
    const res = await fetch(Endpoints.SETTINGS(key));
    if (!res.ok) return null;
    return res.json();
  },

  async set(key, value) {
    const res = await fetch(Endpoints.SETTINGS_UPDATE(key), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: String(value) })
    });
    if (!res.ok) throw new Error('Failed to update setting');
    return res.json();
  },

  async getTheme() {
    return this.get('theme');
  },

  async setTheme(theme) {
    return this.set('theme', theme);
  },

  async getTemperature() {
    return this.get('temperature');
  },

  async setTemperature(value) {
    return this.set('temperature', value);
  }
};

// Auth APIs
const AuthAPI = {
  async getSession() {
    const res = await fetch(`${API_BASE}/auth/session`);
    if (!res.ok) throw new Error('Failed to get session');
    return res.json();
  },

  async login(username, password) {
    const res = await fetch(`${API_BASE}/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password })
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.message || 'Login failed');
    }
    return res.json();
  },

  async logout() {
    const res = await fetch(`${API_BASE}/auth/logout`, { method: 'POST' });
    if (!res.ok) throw new Error('Failed to logout');
    return res.json();
  }
};

// Metrics APIs
const MetricsAPI = {
  async get() {
    const res = await fetch(`${API_BASE}/metrics`);
    if (!res.ok) throw new Error('Failed to get metrics');
    return res.json();
  }
};
