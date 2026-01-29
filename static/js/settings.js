// Settings page JavaScript

let providers = [];
let selectedModels = [];
let editingProviderId = null;
let fetchedModels = []; // Store fetched models for filtering

// Initialize page
document.addEventListener('DOMContentLoaded', function () {
    initTheme();
    loadProviders();
    loadSettings();

    // Temperature slider
    const tempSlider = document.getElementById('temperature');
    const tempValue = document.getElementById('temp-value');
    tempSlider.addEventListener('input', function () {
        tempValue.textContent = this.value;
    });
    tempSlider.addEventListener('change', function () {
        updateSetting('temperature', this.value);
    });

    // Max tokens
    document.getElementById('max-tokens').addEventListener('change', function () {
        updateSetting('max_tokens', this.value);
    });

    // Brave API Key
    document.getElementById('brave-api-key').addEventListener('change', function () {
        updateSetting('brave_api_key', this.value);
    });
});

// Theme management
function initTheme() {
    const savedTheme = localStorage.getItem('theme') || 'light';
    document.documentElement.setAttribute('data-theme', savedTheme);
    updateThemeButtons(savedTheme);
    updateThemeIcon(savedTheme);
}

function setTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('theme', theme);
    updateThemeButtons(theme);
    updateThemeIcon(theme);
    updateSetting('theme', theme);
}

function toggleTheme() {
    const current = document.documentElement.getAttribute('data-theme') || 'light';
    setTheme(current === 'dark' ? 'light' : 'dark');
}

function updateThemeButtons(theme) {
    document.querySelectorAll('.theme-btn').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.theme === theme);
    });
}

function updateThemeIcon(theme) {
    const btn = document.getElementById('theme-toggle');
    if (btn) btn.textContent = theme === 'dark' ? '☀️' : '🌙';
}

// Load settings from server
async function loadSettings() {
    try {
        const tempRes = await fetch('/api/settings/temperature');
        if (tempRes.ok) {
            const data = await tempRes.json();
            document.getElementById('temperature').value = data.value;
            document.getElementById('temp-value').textContent = data.value;
        }

        const tokensRes = await fetch('/api/settings/max_tokens');
        if (tokensRes.ok) {
            const data = await tokensRes.json();
            document.getElementById('max-tokens').value = data.value;
        }

        const themeRes = await fetch('/api/settings/theme');
        if (themeRes.ok) {
            const data = await themeRes.json();
            setTheme(data.value);
        }

        const braveRes = await fetch('/api/settings/brave_api_key');
        if (braveRes.ok) {
            const data = await braveRes.json();
            document.getElementById('brave-api-key').value = data.value;
        }
    } catch (err) {
        console.error('Error loading settings:', err);
    }
}

// Update setting on server
async function updateSetting(key, value) {
    try {
        await fetch(`/api/settings/${key}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ value: String(value) })
        });
    } catch (err) {
        console.error('Error updating setting:', err);
    }
}

// Provider management
async function loadProviders() {
    try {
        const res = await fetch('/api/providers');
        providers = await res.json();
        renderProviders();
    } catch (err) {
        console.error('Error loading providers:', err);
        document.getElementById('providers-list').innerHTML =
            '<p class="text-danger">Error loading providers</p>';
    }
}

function renderProviders() {
    const container = document.getElementById('providers-list');

    if (!providers || providers.length === 0) {
        container.innerHTML = '<p class="text-muted">No providers configured. Add one to get started!</p>';
        return;
    }

    container.innerHTML = providers.map(p => `
        <div class="provider-card ${p.is_active ? 'active' : ''}" data-id="${p.id}">
            <div class="provider-header">
                <div class="provider-name">
                    <input type="radio" name="active-provider" 
                           ${p.is_active ? 'checked' : ''} 
                           onchange="activateProvider(${p.id})"
                           style="margin-right: 8px;">
                    ${escapeHtml(p.name)}
                    <span class="provider-badge">${p.type === 'ollama' ? 'Ollama' : 'OpenAI'}</span>
                    ${p.is_active ? '<span class="badge bg-success ms-2">Active</span>' : ''}
                </div>
                <div class="provider-actions">
                    <button class="btn btn-sm btn-outline-secondary" onclick="editProvider(${p.id})">Edit</button>
                    <button class="btn btn-sm btn-outline-danger" onclick="deleteProvider(${p.id})">×</button>
                </div>
            </div>
            <div class="provider-models">
                Models: ${p.models && p.models.length > 0
            ? p.models.map(m => `<span class="${m.is_default ? 'fw-bold' : ''}">${escapeHtml(m.model_name)}${m.is_default ? ' ★' : ''}</span>`).join(', ')
            : '<em>None configured</em>'}
            </div>
        </div>
    `).join('');
}

function showAddProviderModal() {
    editingProviderId = null;
    selectedModels = [];
    document.getElementById('providerModalTitle').textContent = 'Add Provider';
    document.getElementById('provider-id').value = '';
    document.getElementById('provider-name').value = '';
    document.getElementById('provider-type').value = 'ollama';
    document.getElementById('provider-baseurl').value = '';
    document.getElementById('provider-apikey').value = '';
    document.getElementById('fetched-models-container').style.display = 'none';
    document.getElementById('fetched-models-container').innerHTML = '';
    toggleProviderFields();
    renderSelectedModels();

    const modal = new bootstrap.Modal(document.getElementById('providerModal'));
    modal.show();
}

function editProvider(id) {
    const provider = providers.find(p => p.id === id);
    if (!provider) return;

    editingProviderId = id;
    selectedModels = provider.models ? provider.models.map(m => ({
        name: m.model_name,
        isDefault: m.is_default
    })) : [];

    document.getElementById('providerModalTitle').textContent = 'Edit Provider';
    document.getElementById('provider-id').value = id;
    document.getElementById('provider-name').value = provider.name;
    document.getElementById('provider-type').value = provider.type;
    document.getElementById('provider-baseurl').value = provider.base_url || '';
    document.getElementById('provider-apikey').value = ''; // Don't show existing key
    document.getElementById('fetched-models-container').style.display = 'none';
    toggleProviderFields();
    renderSelectedModels();

    const modal = new bootstrap.Modal(document.getElementById('providerModal'));
    modal.show();
}

function toggleProviderFields() {
    const type = document.getElementById('provider-type').value;
    const openaiFields = document.getElementById('openai-fields');
    openaiFields.style.display = type === 'openai_compatible' ? 'block' : 'none';
}

async function fetchModels() {
    const btn = document.getElementById('fetch-models-btn');
    const container = document.getElementById('fetched-models-container');
    const type = document.getElementById('provider-type').value;

    // For new providers, we need to temporarily create or use existing provider info
    let providerId = editingProviderId;

    if (!providerId) {
        // For new providers, we need to save first or use a temp endpoint
        // For now, show a message
        if (type === 'openai_compatible') {
            const baseUrl = document.getElementById('provider-baseurl').value;
            const apiKey = document.getElementById('provider-apikey').value;
            if (!baseUrl || !apiKey) {
                alert('Please enter Base URL and API Key first to fetch models');
                return;
            }
        }
        alert('Please save the provider first, then edit it to fetch models');
        return;
    }

    btn.disabled = true;
    btn.innerHTML = '<span class="loading-spinner"></span> Fetching...';

    try {
        const res = await fetch(`/api/providers/${providerId}/fetch-models`, {
            method: 'POST'
        });

        if (!res.ok) {
            const error = await res.text();
            throw new Error(error);
        }

        const models = await res.json();
        fetchedModels = models; // Store for filtering

        container.style.display = 'block';
        if (models.length === 0) {
            container.innerHTML = '<p class="text-muted small">No models found</p>';
        } else {
            container.innerHTML = `
                <div class="mb-2">
                    <input type="text" class="form-control form-control-sm" 
                           id="model-filter" 
                           placeholder="Filter models..." 
                           oninput="filterFetchedModels(this.value)">
                </div>
                <p class="small text-muted-dark mb-2">Select models to add (<span id="models-count">${models.length}</span> available):</p>
                <div id="fetched-models-list">
                    ${renderFetchedModelsList(models)}
                </div>
            `;
        }
    } catch (err) {
        container.style.display = 'block';
        container.innerHTML = `<p class="text-danger small">Error: ${escapeHtml(err.message)}</p>`;
    } finally {
        btn.disabled = false;
        btn.innerHTML = '🔄 Fetch Available Models';
    }
}

function renderFetchedModelsList(models) {
    if (models.length === 0) {
        return '<p class="text-muted small">No models match the filter</p>';
    }
    return models.map(m => `
        <div class="form-check">
            <input class="form-check-input" type="checkbox" 
                   id="model-${escapeHtml(m.id)}" 
                   value="${escapeHtml(m.id)}"
                   ${selectedModels.some(sm => sm.name === m.id) ? 'checked' : ''}
                   onchange="toggleFetchedModel('${escapeHtml(m.id)}', this.checked)">
            <label class="form-check-label small" for="model-${escapeHtml(m.id)}">
                ${escapeHtml(m.id)}
                ${m.owned_by ? `<span class="text-muted">(${escapeHtml(m.owned_by)})</span>` : ''}
            </label>
        </div>
    `).join('');
}

function filterFetchedModels(query) {
    const listContainer = document.getElementById('fetched-models-list');
    const countSpan = document.getElementById('models-count');
    if (!listContainer) return;

    const q = query.toLowerCase().trim();
    const filtered = q
        ? fetchedModels.filter(m => m.id.toLowerCase().includes(q) || (m.owned_by && m.owned_by.toLowerCase().includes(q)))
        : fetchedModels;

    listContainer.innerHTML = renderFetchedModelsList(filtered);
    if (countSpan) {
        countSpan.textContent = filtered.length;
    }
}

function toggleFetchedModel(modelName, checked) {
    if (checked) {
        if (!selectedModels.some(m => m.name === modelName)) {
            selectedModels.push({ name: modelName, isDefault: selectedModels.length === 0 });
        }
    } else {
        selectedModels = selectedModels.filter(m => m.name !== modelName);
        // Ensure there's still a default
        if (selectedModels.length > 0 && !selectedModels.some(m => m.isDefault)) {
            selectedModels[0].isDefault = true;
        }
    }
    renderSelectedModels();
}

function addManualModel() {
    const input = document.getElementById('manual-model');
    const modelName = input.value.trim();

    if (!modelName) return;
    if (selectedModels.some(m => m.name === modelName)) {
        alert('Model already added');
        return;
    }

    selectedModels.push({ name: modelName, isDefault: selectedModels.length === 0 });
    input.value = '';
    renderSelectedModels();
}

function renderSelectedModels() {
    const container = document.getElementById('selected-models');

    if (selectedModels.length === 0) {
        container.innerHTML = '<p class="text-muted small mb-0">No models added yet</p>';
        return;
    }

    container.innerHTML = selectedModels.map((m, i) => `
        <div class="model-item ${m.isDefault ? 'default' : ''}">
            <span>
                ${escapeHtml(m.name)}
                ${m.isDefault ? '<span class="badge bg-primary ms-1">Default</span>' : ''}
            </span>
            <div>
                ${!m.isDefault ? `<button class="btn btn-sm btn-link p-0 me-2" onclick="setDefaultModel(${i})">Set Default</button>` : ''}
                <button class="btn btn-sm btn-link text-danger p-0" onclick="removeModel(${i})">×</button>
            </div>
        </div>
    `).join('');
}

function setDefaultModel(index) {
    selectedModels.forEach((m, i) => m.isDefault = i === index);
    renderSelectedModels();
}

function removeModel(index) {
    const wasDefault = selectedModels[index].isDefault;
    selectedModels.splice(index, 1);
    if (wasDefault && selectedModels.length > 0) {
        selectedModels[0].isDefault = true;
    }
    renderSelectedModels();
}

async function saveProvider() {
    const name = document.getElementById('provider-name').value.trim();
    const type = document.getElementById('provider-type').value;
    const baseUrl = document.getElementById('provider-baseurl').value.trim();
    const apiKey = document.getElementById('provider-apikey').value.trim();

    if (!name) {
        alert('Please enter a provider name');
        return;
    }

    if (type === 'openai_compatible' && !editingProviderId) {
        if (!baseUrl || !apiKey) {
            alert('Base URL and API Key are required for OpenAI-compatible providers');
            return;
        }
    }

    const data = {
        name,
        type,
        base_url: baseUrl,
        api_key: apiKey,
        models: selectedModels.map(m => m.name)
    };

    try {
        let res;
        if (editingProviderId) {
            res = await fetch(`/api/providers/${editingProviderId}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data)
            });

            // Update models separately
            // First, delete existing models and add new ones
            const existingProvider = providers.find(p => p.id === editingProviderId);
            if (existingProvider && existingProvider.models) {
                for (const m of existingProvider.models) {
                    await fetch(`/api/models/${m.id}`, { method: 'DELETE' });
                }
            }
            for (const m of selectedModels) {
                await fetch('/api/models', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        provider_id: editingProviderId,
                        model_name: m.name,
                        is_default: m.isDefault
                    })
                });
            }
        } else {
            res = await fetch('/api/providers', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data)
            });
        }

        if (!res.ok) {
            const error = await res.text();
            throw new Error(error);
        }

        bootstrap.Modal.getInstance(document.getElementById('providerModal')).hide();
        await loadProviders();
    } catch (err) {
        alert('Error saving provider: ' + err.message);
    }
}

async function deleteProvider(id) {
    if (!confirm('Are you sure you want to delete this provider?')) return;

    try {
        const res = await fetch(`/api/providers/${id}`, { method: 'DELETE' });
        if (!res.ok) {
            const error = await res.text();
            throw new Error(error);
        }
        await loadProviders();
    } catch (err) {
        alert('Error deleting provider: ' + err.message);
    }
}

async function activateProvider(id) {
    try {
        const res = await fetch(`/api/providers/${id}/activate`, { method: 'POST' });
        if (!res.ok) {
            const error = await res.text();
            throw new Error(error);
        }
        await loadProviders();
    } catch (err) {
        alert('Error activating provider: ' + err.message);
    }
}

// Utility
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
