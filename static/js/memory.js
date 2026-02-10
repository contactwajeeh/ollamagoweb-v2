let memories = [];
let filteredMemories = [];
let editingMemory = null;

function openMemoryModal() {
  const modal = document.getElementById('memoryModal');
  if (modal) {
    modal.style.display = 'flex';
    loadMemories();
  }
}

function closeMemoryModal() {
  const modal = document.getElementById('memoryModal');
  if (modal) {
    modal.style.display = 'none';
    editingMemory = null;
    resetMemoryForm();
  }
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
    filterMemories();
  } catch (error) {
    console.error('Error loading memories:', error);
    container.innerHTML = '<div class="loading-memories" style="padding: 20px; text-align: center; color: var(--color-error);">Failed to load memories</div>';
    memories = [];
    filteredMemories = [];
    renderMemories();
  }
}

function renderMemories() {
  const container = document.getElementById('memoryListContainer');

  if (!filteredMemories || !Array.isArray(filteredMemories) || filteredMemories.length === 0) {
    const searchTerm = document.getElementById('memorySearchInput').value;
    const categoryFilter = document.getElementById('memoryCategoryFilter').value;

    if (searchTerm || categoryFilter) {
      container.innerHTML = `
        <div class="memory-empty-state">
          <div class="memory-empty-icon">🔍</div>
          <div class="memory-empty-text">No memories match your search</div>
          <div class="memory-empty-hint">Try clearing the search or filters</div>
        </div>
      `;
    } else {
      container.innerHTML = `
        <div class="memory-empty-state">
          <div class="memory-empty-icon">🧠</div>
          <div class="memory-empty-text">No memories stored yet</div>
          <div class="memory-empty-hint">Add memories above to personalize your AI experience</div>
        </div>
      `;
    }
    return;
  }

  const escapeHtml = window.escapeHtml || ((text) => {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  });

  container.innerHTML = filteredMemories.map(memory => `
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

function filterMemories() {
  const searchTerm = document.getElementById('memorySearchInput').value.toLowerCase();
  const categoryFilter = document.getElementById('memoryCategoryFilter').value.toLowerCase();

  filteredMemories = memories.filter(memory => {
    const matchesSearch = !searchTerm ||
      memory.key.toLowerCase().includes(searchTerm) ||
      memory.value.toLowerCase().includes(searchTerm);

    const matchesCategory = !categoryFilter ||
      memory.category.toLowerCase() === categoryFilter;

    return matchesSearch && matchesCategory;
  });

  renderMemories();
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
    let response;
    let successMessage;

    if (editingMemory) {
      // Update existing memory - the key is disabled so it stays the same
      response = await fetch('/api/memories', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: editingMemory, value, category })
      });
      successMessage = 'Memory updated successfully';
    } else {
      // Add new memory
      response = await fetch('/api/memories', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, value, category })
      });
      successMessage = 'Memory added successfully';
    }

    if (response.ok) {
      notify(successMessage, 'success');
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
  if (e.key === 'Escape') {
    const modal = document.getElementById('memoryModal');
    if (modal && modal.style.display === 'flex') {
      closeMemoryModal();
    }
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

async function testMemoryExtraction() {
  const testMessage = prompt('Enter a test message to extract memories from:\n\nExample: "Remind me about my meeting with Ram at 5 PM EST"');

  if (!testMessage || !testMessage.trim()) {
    return;
  }

  try {
    const response = await fetch('/api/memories/extract', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: testMessage.trim() })
    });

    if (response.ok) {
      const result = await response.json();
      notify('Memory extraction test completed! Check server logs for details.', 'success');
      console.log('Extraction test result:', result);

      // Reload memories to show any new ones
      setTimeout(() => {
        loadMemories();
      }, 1000);
    } else {
      const error = await response.json();
      notify(error.message || 'Extraction test failed', 'error');
    }
  } catch (error) {
    notify('Failed to run extraction test', 'error');
    console.error('Error testing extraction:', error);
  }
}
