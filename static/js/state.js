// ============================================
// State Management Module
// ============================================

const ChatState = new (class {
  constructor() {
    this.state = {
      currentChatId: null,
      chatsList: [],
      messages: new Map(),
      settings: {
        theme: 'light',
        temperature: 0.7,
        maxTokens: 4096
      },
      ui: {
        sidebarOpen: true,
        currentModel: '',
        availableModels: []
      }
    };

    this.listeners = new Set();
    this.history = [];
    this.historyIndex = -1;
  }

  get(key) {
    return key.split('.').reduce((obj, k) => obj?.[k], this.state);
  }

  set(key, value) {
    const keys = key.split('.');
    const lastKey = keys.pop();
    const target = keys.reduce((obj, k) => {
      if (!obj[k]) obj[k] = {};
      return obj[k];
    }, this.state);

    const oldValue = target[lastKey];
    target[lastKey] = value;

    this.notify({ key, oldValue, newValue: value });
  }

  subscribe(fn) {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  notify(change) {
    this.listeners.forEach(fn => fn(change));
  }

  pushState() {
    const snapshot = JSON.stringify(this.state);
    this.history = this.history.slice(0, this.historyIndex + 1);
    this.history.push(snapshot);
    this.historyIndex = this.history.length - 1;
  }

  undo() {
    if (this.historyIndex > 0) {
      this.historyIndex--;
      const snapshot = this.history[this.historyIndex];
      this.state = JSON.parse(snapshot);
      this.notify({ key: 'state', oldValue: null, newValue: this.state });
      return true;
    }
    return false;
  }

  redo() {
    if (this.historyIndex < this.history.length - 1) {
      this.historyIndex++;
      const snapshot = this.history[this.historyIndex];
      this.state = JSON.parse(snapshot);
      this.notify({ key: 'state', oldValue: null, newValue: this.state });
      return true;
    }
    return false;
  }

  canUndo() {
    return this.historyIndex > 0;
  }

  canRedo() {
    return this.historyIndex < this.history.length - 1;
  }

  reset() {
    this.state = {
      currentChatId: null,
      chatsList: [],
      messages: new Map(),
      settings: {
        theme: 'light',
        temperature: 0.7,
        maxTokens: 4096
      },
      ui: {
        sidebarOpen: true,
        currentModel: '',
        availableModels: []
      }
    };
    this.history = [];
    this.historyIndex = -1;
    this.notify({ key: 'reset' });
  }
})();

// ============================================
// State-aware helper functions
// ============================================

function getCurrentChatId() {
  return ChatState.state.currentChatId;
}

function setCurrentChatId(id) {
  ChatState.pushState();
  ChatState.set('currentChatId', id);
}

function getChatsList() {
  return ChatState.state.chatsList;
}

function setChatsList(chats) {
  ChatState.set('chatsList', chats);
}

function addChatToList(chat) {
  ChatState.pushState();
  const chats = [...ChatState.state.chatsList];
  chats.unshift(chat);
  ChatState.set('chatsList', chats);
}

function removeChatFromList(chatId) {
  ChatState.pushState();
  const chats = ChatState.state.chatsList.filter(c => c.id !== chatId);
  ChatState.set('chatsList', chats);
}

function updateChatInList(chatId, updates) {
  ChatState.pushState();
  const chats = ChatState.state.chatsList.map(c =>
    c.id === chatId ? { ...c, ...updates } : c
  );
  ChatState.set('chatsList', chats);
}

function getMessages(chatId) {
  return ChatState.state.messages.get(chatId) || [];
}

function setMessages(chatId, messages) {
  ChatState.pushState();
  const messagesMap = new Map(ChatState.state.messages);
  messagesMap.set(chatId, messages);
  ChatState.set('messages', messagesMap);
}

function addMessageToState(chatId, message) {
  ChatState.pushState();
  const messagesMap = new Map(ChatState.state.messages);
  const currentMessages = messagesMap.get(chatId) || [];
  messagesMap.set(chatId, [...currentMessages, message]);
  ChatState.set('messages', messagesMap);
}

function updateMessageInState(chatId, messageId, updates) {
  ChatState.pushState();
  const messagesMap = new Map(ChatState.state.messages);
  const currentMessages = messagesMap.get(chatId) || [];
  messagesMap.set(chatId, currentMessages.map(m =>
    m.id === messageId ? { ...m, ...updates } : m
  ));
  ChatState.set('messages', messagesMap);
}

function removeMessageFromState(chatId, messageId) {
  ChatState.pushState();
  const messagesMap = new Map(ChatState.state.messages);
  const currentMessages = messagesMap.get(chatId) || [];
  messagesMap.set(chatId, currentMessages.filter(m => m.id !== messageId));
  ChatState.set('messages', messagesMap);
}

function getSetting(key) {
  return ChatState.state.settings[key];
}

function setSetting(key, value) {
  ChatState.set(`settings.${key}`, value);
}

function getTheme() {
  return ChatState.state.settings.theme;
}

function setTheme(theme) {
  ChatState.set('settings.theme', theme);
}

function isSidebarOpen() {
  return ChatState.state.ui.sidebarOpen;
}

function setSidebarOpenState(open) {
  ChatState.set('ui.sidebarOpen', open);
}

function getCurrentModel() {
  return ChatState.state.ui.currentModel;
}

function setCurrentModelState(model) {
  ChatState.set('ui.currentModel', model);
}

function getAvailableModels() {
  return ChatState.state.ui.availableModels;
}

function setAvailableModelsState(models) {
  ChatState.set('ui.availableModels', models);
}
