// UI элементы и управление приложением
let roomsList = null;
let messagesContainer = null;
let currentRoomNameSpan = null;
let messageInput = null;
let sendButton = null;

// Состояние
let currentMembers = [];
let availableUsers = [];
let selectedUsers = [];
let currentUserLogin = null;

// Инициализация UI
document.addEventListener('DOMContentLoaded', () => {
    // Получаем ссылки на элементы
    roomsList = document.getElementById('roomsList');
    messagesContainer = document.getElementById('messagesContainer');
    currentRoomNameSpan = document.getElementById('currentRoomName');
    messageInput = document.getElementById('messageInput');
    sendButton = document.getElementById('sendMessageBtn');
    
    // Инициализируем обработчики
    initEventHandlers();
    initModals();
});

function initEventHandlers() {
    // Отправка сообщения по кнопке
    if (sendButton) {
        sendButton.addEventListener('click', () => {
            const content = messageInput ? messageInput.value : '';
            if (content && content.trim() && typeof sendMessage === 'function') {
                if (sendMessage(content)) {
                    if (messageInput) {
                        messageInput.value = '';
                        autoResizeTextarea(messageInput);
                    }
                }
            }
        });
    }
    
    // Отправка сообщения по Enter
    if (messageInput) {
        messageInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                if (sendButton) sendButton.click();
            }
        });
        
        messageInput.addEventListener('input', function() {
            autoResizeTextarea(this);
        });
    }
}

function autoResizeTextarea(textarea) {
    textarea.style.height = 'auto';
    textarea.style.height = Math.min(textarea.scrollHeight, 100) + 'px';
}

function displayRooms(rooms) {
    if (!roomsList) return;
    
    roomsList.innerHTML = '';
    
    if (!rooms || rooms.length === 0) {
        roomsList.innerHTML = '<div class="empty-state">📭 Нет комнат<br>Создайте первую!</div>';
        return;
    }
    
    rooms.forEach(room => {
        const roomElement = createRoomElement(room);
        roomsList.appendChild(roomElement);
    });
}

function createRoomElement(room) {
    const roomElement = document.createElement('div');
    roomElement.className = 'room-item';
    if (currentRoomId === room.id) {
        roomElement.classList.add('active');
    }
    roomElement.setAttribute('data-room-id', room.id);
    
    roomElement.innerHTML = `
        <div class="room-name">${escapeHtml(room.name)}</div>
        <div class="room-badge"></div>
    `;
    
    roomElement.onclick = () => {
        if (typeof joinRoom === 'function') {
            joinRoom(room.id);
            setActiveRoom(room.id);
            updateRoomHeader(room.name);
            
            // Убираем подсветку новых сообщений
            roomElement.classList.remove('has-new-message');
        }
    };
    
    return roomElement;
}

function setActiveRoom(roomId) {
    document.querySelectorAll('.room-item').forEach(item => {
        if (parseInt(item.getAttribute('data-room-id')) === roomId) {
            item.classList.add('active');
        } else {
            item.classList.remove('active');
        }
    });
}

function updateRoomHeader(roomName) {
    if (currentRoomNameSpan) {
        currentRoomNameSpan.textContent = roomName || 'Выберите комнату';
    }
}

function displayMessages(messages) {
    if (!messagesContainer) return;
    
    messagesContainer.innerHTML = '';
    
    if (!messages || messages.length === 0) {
        messagesContainer.innerHTML = '<div class="welcome-message"><p>💬 Нет сообщений<br>Напишите что-нибудь!</p></div>';
        return;
    }
    
    messages.forEach(msg => {
        addMessageToChat(msg, false);
    });
    
    scrollToBottom();
}

function addMessageToChat(message, scroll = true) {
    if (!messagesContainer) return;
    
    // Убираем приветственное сообщение если оно есть
    const welcomeMsg = messagesContainer.querySelector('.welcome-message');
    if (welcomeMsg && messagesContainer.children.length === 1) {
        welcomeMsg.remove();
    }
    
    const currentUserId = typeof getCurrentUserId === 'function' ? getCurrentUserId() : null;
    const isOwnMessage = message.user_id === currentUserId;
    
    const messageElement = document.createElement('div');
    messageElement.className = `message ${isOwnMessage ? 'sent' : 'received'}`;
    
    const timestamp = message.created_at ? new Date(message.created_at).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) : '';
    
    if (!isOwnMessage) {
        messageElement.innerHTML = `
            <div class="message-content">
                <div class="message-username">${escapeHtml(message.username)}</div>
                <div class="message-text">${escapeHtml(message.content || message.text)}</div>
                ${timestamp ? `<div class="message-time">${timestamp}</div>` : ''}
            </div>
        `;
    } else {
        messageElement.innerHTML = `
            <div class="message-content">
                <div class="message-text">${escapeHtml(message.content || message.text)}</div>
                ${timestamp ? `<div class="message-time">${timestamp}</div>` : ''}
            </div>
        `;
    }
    
    messagesContainer.appendChild(messageElement);
    
    if (scroll) {
        scrollToBottom();
    }
}

function scrollToBottom() {
    if (messagesContainer) {
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }
}

function displayMembers(users) {
    currentMembers = users || [];
    const membersList = document.getElementById('membersList');
    
    if (membersList) {
        membersList.innerHTML = '';
        
        if (currentMembers.length === 0) {
            membersList.innerHTML = '<div class="empty-state">👥 Нет участников</div>';
            return;
        }
        
        const currentUserId = typeof getCurrentUserId === 'function' ? getCurrentUserId() : null;
        
        currentMembers.forEach(user => {
            const memberElement = document.createElement('div');
            memberElement.className = 'member-item';
            memberElement.innerHTML = `
                <span>👤 ${escapeHtml(user.login)}</span>
                ${user.id === currentUserId ? '<span class="member-badge">Вы</span>' : ''}
            `;
            membersList.appendChild(memberElement);
        });
    }
}

function initModals() {
    // Модальное окно создания комнаты
    const createRoomBtn = document.getElementById('createRoomBtn');
    const createRoomModal = document.getElementById('createRoomModal');
    const confirmCreateBtn = document.getElementById('confirmCreateRoom');
    const cancelCreateBtn = document.getElementById('cancelCreateRoom');
    const closeModalBtns = document.querySelectorAll('.close');
    
    if (createRoomBtn) {
        createRoomBtn.onclick = () => {
            if (createRoomModal) {
                createRoomModal.style.display = 'block';
                if (typeof loadAvailableUsers === 'function') {
                    loadAvailableUsers();
                }
            }
        };
    }
    
    if (confirmCreateBtn) {
        confirmCreateBtn.onclick = () => {
            const roomNameInput = document.getElementById('roomName');
            const roomName = roomNameInput ? roomNameInput.value.trim() : '';
            
            if (!roomName) {
                alert('Введите название комнаты');
                return;
            }
            
            if (selectedUsers.length === 0) {
                alert('Добавьте хотя бы одного участника');
                return;
            }
            
            if (typeof createRoom === 'function') {
                createRoom(roomName, selectedUsers);
            }
            
            if (createRoomModal) {
                createRoomModal.style.display = 'none';
            }
            resetCreateRoomForm();
        };
    }
    
    if (cancelCreateBtn) {
        cancelCreateBtn.onclick = () => {
            if (createRoomModal) createRoomModal.style.display = 'none';
            resetCreateRoomForm();
        };
    }
    
    // Модальное окно участников
    const roomInfoBtn = document.getElementById('roomInfoBtn');
    const membersModal = document.getElementById('membersModal');
    
    if (roomInfoBtn) {
        roomInfoBtn.onclick = () => {
            if (membersModal) membersModal.style.display = 'block';
        };
    }
    
    // Закрытие модальных окон
    closeModalBtns.forEach(closeBtn => {
        closeBtn.onclick = () => {
            if (createRoomModal) createRoomModal.style.display = 'none';
            if (membersModal) membersModal.style.display = 'none';
            resetCreateRoomForm();
        };
    });
    
    // Закрытие по клику вне модального окна
    window.onclick = (event) => {
        if (event.target === createRoomModal) {
            createRoomModal.style.display = 'none';
            resetCreateRoomForm();
        }
        if (event.target === membersModal) {
            membersModal.style.display = 'none';
        }
    };
    
    // Поиск пользователей
    const userSearch = document.getElementById('userSearch');
    if (userSearch) {
        userSearch.addEventListener('input', (e) => {
            const query = e.target.value;
            if (typeof searchUsers === 'function') {
                searchUsers(query);
            }
        });
    }
}

async function loadAvailableUsers() {
    const token = typeof getCurrentToken === 'function' ? getCurrentToken() : null;
    
    if (!token) {
        console.error('No token available');
        return;
    }
    
    try {
        const response = await fetch('/api/users', {
            headers: {
                'Authorization': `Bearer ${token}`
            }
        });
        
        if (response.ok) {
            availableUsers = await response.json();
            console.log('Loaded users:', availableUsers);
        } else {
            console.error('Failed to load users:', response.status);
        }
    } catch (error) {
        console.error('Failed to load users:', error);
    }
}

function searchUsers(query) {
    const searchResults = document.getElementById('searchResults');
    if (!searchResults) return;
    
    if (!query || query.trim() === '') {
        searchResults.innerHTML = '';
        return;
    }
    
    const currentUserId = typeof getCurrentUserId === 'function' ? getCurrentUserId() : null;
    const filtered = availableUsers.filter(user => 
        user.id !== currentUserId &&
        !selectedUsers.includes(user.id) &&
        user.login.toLowerCase().includes(query.toLowerCase())
    );
    
    searchResults.innerHTML = '';
    
    if (filtered.length === 0) {
        searchResults.innerHTML = '<div class="search-no-results">Пользователи не найдены</div>';
        return;
    }
    
    filtered.forEach(user => {
        const resultItem = document.createElement('div');
        resultItem.className = 'search-result-item';
        resultItem.innerHTML = `👤 ${escapeHtml(user.login)}`;
        resultItem.onclick = () => addSelectedUser(user);
        searchResults.appendChild(resultItem);
    });
}

function addSelectedUser(user) {
    if (!selectedUsers.includes(user.id)) {
        selectedUsers.push(user.id);
        updateSelectedUsersDisplay();
        
        // Очищаем поиск
        const userSearch = document.getElementById('userSearch');
        const searchResults = document.getElementById('searchResults');
        if (userSearch) userSearch.value = '';
        if (searchResults) searchResults.innerHTML = '';
    }
}

function updateSelectedUsersDisplay() {
    const selectedUsersDiv = document.getElementById('selectedUsers');
    if (!selectedUsersDiv) return;
    
    selectedUsersDiv.innerHTML = '';
    
    selectedUsers.forEach(userId => {
        const user = availableUsers.find(u => u.id === userId);
        if (user) {
            const tag = document.createElement('div');
            tag.className = 'selected-user-tag';
            tag.innerHTML = `
                ${escapeHtml(user.login)}
                <span class="remove" onclick="window.removeSelectedUser(${userId})">×</span>
            `;
            selectedUsersDiv.appendChild(tag);
        }
    });
}

function removeSelectedUser(userId) {
    selectedUsers = selectedUsers.filter(id => id !== userId);
    updateSelectedUsersDisplay();
}

function resetCreateRoomForm() {
    const roomNameInput = document.getElementById('roomName');
    const userSearch = document.getElementById('userSearch');
    const searchResults = document.getElementById('searchResults');
    
    if (roomNameInput) roomNameInput.value = '';
    if (userSearch) userSearch.value = '';
    if (searchResults) searchResults.innerHTML = '';
    
    selectedUsers = [];
    updateSelectedUsersDisplay();
}

function resetChatState() {
    currentRoomId = null;
    currentMembers = [];
    
    if (roomsList) roomsList.innerHTML = '';
    if (messagesContainer) {
        messagesContainer.innerHTML = '<div class="welcome-message"><p>✨ Добро пожаловать в Messenger ✨</p><p>Выберите комнату или создайте новую, чтобы начать общение</p></div>';
    }
    if (currentRoomNameSpan) currentRoomNameSpan.textContent = 'Выберите комнату';
    if (messageInput) {
        messageInput.disabled = true;
        messageInput.value = '';
    }
    if (sendButton) sendButton.disabled = true;
}

function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Экспортируем функции для глобального использования
window.displayRooms = displayRooms;
window.displayMessages = displayMessages;
window.addMessageToChat = addMessageToChat;
window.displayMembers = displayMembers;
window.updateRoomHeader = updateRoomHeader;
window.loadAvailableUsers = loadAvailableUsers;
window.searchUsers = searchUsers;
window.addSelectedUser = addSelectedUser;
window.removeSelectedUser = removeSelectedUser;
window.resetChatState = resetChatState;
window.escapeHtml = escapeHtml;

// Добавляем стили для новых элементов
const style = document.createElement('style');
style.textContent = `
    .room-badge {
        display: none;
    }
    .message-time {
        font-size: 10px;
        opacity: 0.7;
        margin-top: 4px;
        text-align: right;
    }
    .member-badge {
        background: #667eea;
        color: white;
        padding: 2px 8px;
        border-radius: 12px;
        font-size: 11px;
        margin-left: 8px;
    }
    .search-no-results {
        padding: 10px;
        text-align: center;
        color: #999;
        font-size: 13px;
    }
    .connection-error {
        animation: slideDown 0.3s ease;
    }
    @keyframes slideDown {
        from {
            transform: translateX(-50%) translateY(-100%);
            opacity: 0;
        }
        to {
            transform: translateX(-50%) translateY(0);
            opacity: 1;
        }
    }
`;
document.head.appendChild(style);