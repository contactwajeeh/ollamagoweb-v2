let memories = [];
let editingMemory = null;

function openMemoryModal() {
  document.getElementById('memoryModal').classList.add('show');
  loadMemories();
}

function closeMemoryModal() {
  document.getElementById('memoryModal').classList.remove('show');
  editingMemory = null;
  resetMemoryForm();
}

async function loadMemories() {
  const container = document.getElementById('memoryListContainer');
  container.innerHTML = '<div class="loading-memories" style="padding: 20px; text-align: center; color: var(--text-muted);">Loading memories...</div>';

  try {
    const response = await fetch('/api/memories');

    if (!response.ok) {
      const errorText = await response.text().catch(() => 'Unknown error');
      throw new Error(`HTTP ${response.status}: ${errorText}`);
    }

    const data = await response.json();
    memories = Array.isArray(data) ? data : [];
    renderMemories();
  } catch (error) {
    console.error('Error loading memories:', error);
    container.innerHTML = '<div class="loading-memories" style="padding: 20px; text-align: center; color: var(--color-error);">Failed to load memories</div>';
    memories = [];
    renderMemories();
  }
}

function renderMemories() {
  const container = document.getElementById('memoryListContainer');

  if (!memories || !Array.isArray(memories) || memories.length === 0) {
    container.innerHTML = `
      <div class="memory-empty-state">
        <div class="memory-empty-icon">🧠</div>
        <div class="memory-empty-text">No memories stored yet</div>
        <div class="memory-empty-hint">Add memories above to personalize your AI experience</div>
      </div>
    `;
    return;
  }

  const escapeHtml = window.escapeHtml || ((text) => {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  });

  container.innerHTML = memories.map(memory => `
    <div class="memory-item" data-key="${escapeHtml(memory.key)}">
      <div class="memory-item-content">
        <div class="memory-item-key">${escapeHtml(memory.key)}</div>
        <div class="memory-item-value">${escapeHtml(memory.value)}</div>
        <div class="memory-item-meta">
          <span class="memory-item-category">${escapeHtml(memory.category)}</span>
          <span class="memory-item-confidence">Confidence: ${memory.confidence}%</span>
        </div>
      </div>
      <div class="memory-item-actions">
        <button class="memory-action-btn" onclick="editMemory('${escapeHtml(memory.key)}')" title="Edit">✏️</button>
        <button class="memory-action-btn delete" onclick="deleteMemory('${escapeHtml(memory.key)}')" title="Delete">🗑️</button>
      </div>
    </div>
  `).join('');
}

async function addMemory() {
  const keyInput = document.getElementById('memoryKeyInput');
  const valueInput = document.getElementById('memoryValueInput');
  const categoryInput = document.getElementById('memoryCategoryInput');

  const key = keyInput.value.trim();
  const value = valueInput.value.trim();
  const category = categoryInput.value;

  if (!key || !value) {
    notify('Please fill in both key and value', 'error');
    return;
  }

  try {
    const response = await fetch('/api/memories', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key, value, category })
    });

    if (response.ok) {
      notify(editingMemory ? 'Memory updated successfully' : 'Memory added successfully', 'success');
      resetMemoryForm();
      loadMemories();
    } else {
      const error = await response.json();
      notify(error.message || 'Failed to save memory', 'error');
    }
  } catch (error) {
    notify('Failed to save memory', 'error');
    console.error('Error saving memory:', error);
  }
}

function editMemory(key) {
  if (!memories || !Array.isArray(memories)) return;

  const memory = memories.find(m => m.key === key);
  if (!memory) return;

  editingMemory = key;
  document.getElementById('memoryKeyInput').value = memory.key;
  document.getElementById('memoryValueInput').value = memory.value;
  document.getElementById('memoryCategoryInput').value = memory.category;

  document.querySelector('#memoryModal .modal-btn.primary').textContent = 'Update';
  document.getElementById('memoryKeyInput').disabled = true;
}

async function deleteMemory(key) {
  if (!confirm('Are you sure you want to delete this memory?')) {
    return;
  }

  try {
    const response = await fetch('/api/memories', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key })
    });

    if (response.ok) {
      notify('Memory deleted successfully', 'success');
      loadMemories();
      if (editingMemory === key) {
        resetMemoryForm();
      }
    } else {
      const error = await response.json();
      notify(error.message || 'Failed to delete memory', 'error');
    }
  } catch (error) {
    notify('Failed to delete memory', 'error');
    console.error('Error deleting memory:', error);
  }
}

function resetMemoryForm() {
  editingMemory = null;
  document.getElementById('memoryKeyInput').value = '';
  document.getElementById('memoryValueInput').value = '';
  document.getElementById('memoryCategoryInput').value = 'preference';
  document.getElementById('memoryKeyInput').disabled = false;
  document.querySelector('#memoryModal .modal-btn.primary').textContent = 'Add';
}

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && document.getElementById('memoryModal').classList.contains('show')) {
    closeMemoryModal();
  }
});

document.getElementById('memoryKeyInput').addEventListener('keypress', (e) => {
  if (e.key === 'Enter') {
    document.getElementById('memoryValueInput').focus();
  }
});

document.getElementById('memoryValueInput').addEventListener('keypress', (e) => {
  if (e.key === 'Enter') {
    addMemory();
  }
});
