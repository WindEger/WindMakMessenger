// ===== State Management =====
const state = {
    token: localStorage.getItem('token') || null,
    user: JSON.parse(localStorage.getItem('user') || 'null'),
    ws: null,
    currentRoomId: null,
    rooms: [],
    messages: {},
    members: {},
    reconnectAttempts: 0,
    maxReconnectAttempts: 5
};

// ===== DOM Elements =====
const elements = {
    authScreen: document.getElementById('auth-screen'),
    appScreen: document.getElementById('app-screen'),
    loginForm: document.getElementById('login-form'),
    registerForm: document.getElementById('register-form'),
    showRegister: document.getElementById('show-register'),
    showLogin: document.getElementById('show-login'),
    loginSubmit: document.getElementById('login-submit'),
    registerSubmit: document.getElementById('register-submit'),
    loginError: document.getElementById('login-error'),
    registerError: document.getElementById('register-error'),
    currentUserNickname: document.getElementById('current-user-nickname'),
    logoutBtn: document.getElementById('logout-btn'),
    roomsList: document.getElementById('rooms-list'),
    createRoomBtn: document.getElementById('create-room-btn'),
    noRoomSelected: document.getElementById('no-room-selected'),
    chatContainer: document.getElementById('chat-container'),
    currentRoomName: document.getElementById('current-room-name'),
    roomMembersCount: document.getElementById('room-members-count'),
    messagesContainer: document.getElementById('messages-container'),
    messageInput: document.getElementById('message-input'),
    sendMessageForm: document.getElementById('send-message-form'),
    showMembersBtn: document.getElementById('show-members-btn'),
    renameRoomBtn: document.getElementById('rename-room-btn'),
    leaveRoomBtn: document.getElementById('leave-room-btn'),
    createRoomModal: document.getElementById('create-room-modal'),
    renameRoomModal: document.getElementById('rename-room-modal'),
    addMemberModal: document.getElementById('add-member-modal'),
    membersModal: document.getElementById('members-modal'),
    membersList: document.getElementById('members-list'),
    addMemberBtn: document.getElementById('add-member-btn')
};

// ===== Initialization =====
function init() {
    setupEventListeners();
    
    if (state.token && state.user) {
        showApp();
        connectWebSocket();
    } else {
        showAuth();
    }
}

function setupEventListeners() {
    // Auth
    elements.showRegister.addEventListener('click', (e) => {
        e.preventDefault();
        elements.loginForm.classList.remove('active');
        elements.registerForm.classList.add('active');
        elements.loginError.textContent = '';
        elements.registerError.textContent = '';
    });

    elements.showLogin.addEventListener('click', (e) => {
        e.preventDefault();
        elements.registerForm.classList.remove('active');
        elements.loginForm.classList.add('active');
        elements.loginError.textContent = '';
        elements.registerError.textContent = '';
    });

    elements.loginSubmit.addEventListener('submit', handleLogin);
    elements.registerSubmit.addEventListener('submit', handleRegister);
    elements.logoutBtn.addEventListener('click', handleLogout);

    // Rooms
    elements.createRoomBtn.addEventListener('click', () => openModal(elements.createRoomModal));
    document.getElementById('create-room-form').addEventListener('submit', handleCreateRoom);

    // Chat
    elements.sendMessageForm.addEventListener('submit', handleSendMessage);
    elements.showMembersBtn.addEventListener('click', () => {
        loadMembers(state.currentRoomId);
        openModal(elements.membersModal);
    });
    elements.renameRoomBtn.addEventListener('click', () => openModal(elements.renameRoomModal));
    elements.leaveRoomBtn.addEventListener('click', handleLeaveRoom);

    // Members
    elements.addMemberBtn.addEventListener('click', () => {
        closeModal(elements.membersModal);
        openModal(elements.addMemberModal);
    });
    document.getElementById('add-member-form').addEventListener('submit', handleAddMember);
    document.getElementById('rename-room-form').addEventListener('submit', handleRenameRoom);

    // Modal close handlers
    document.querySelectorAll('.modal-close, .modal-cancel').forEach(btn => {
        btn.addEventListener('click', function() {
            closeModal(this.closest('.modal'));
        });
    });

    // Close modal on outside click
    document.querySelectorAll('.modal').forEach(modal => {
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                closeModal(modal);
            }
        });
    });
}

// ===== Auth Functions =====
async function handleLogin(e) {
    e.preventDefault();
    elements.loginError.textContent = '';

    const username = document.getElementById('login-username').value;
    const password = document.getElementById('login-password').value;

    try {
        const response = await fetch('/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ login: username, password })
        });

        const data = await response.json();

        if (data.success) {
            state.token = data.data.token;
            state.user = {
                login: data.data.login,
                nickname: data.data.nickname
            };
            localStorage.setItem('token', state.token);
            localStorage.setItem('user', JSON.stringify(state.user));
            
            showApp();
            connectWebSocket();
        } else {
            elements.loginError.textContent = data.error || 'Login failed';
        }
    } catch (error) {
        elements.loginError.textContent = 'Network error. Please try again.';
        console.error('Login error:', error);
    }
}

async function handleRegister(e) {
    e.preventDefault();
    elements.registerError.textContent = '';

    const username = document.getElementById('register-username').value;
    const nickname = document.getElementById('register-nickname').value;
    const password = document.getElementById('register-password').value;
    const repeatPassword = document.getElementById('register-repeat-password').value;

    if (password !== repeatPassword) {
        elements.registerError.textContent = 'Passwords do not match';
        return;
    }

    try {
        const response = await fetch('/register', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                login: username,
                nickname: nickname || username,
                password,
                repeatPassword
            })
        });

        const data = await response.json();

        if (data.success) {
            // Auto-login after registration
            document.getElementById('login-username').value = username;
            document.getElementById('login-password').value = password;
            elements.registerForm.classList.remove('active');
            elements.loginForm.classList.add('active');
            
            // Show success message
            const successMsg = document.createElement('div');
            successMsg.style.cssText = 'color: #2ecc71; font-size: 0.875rem; margin-top: 0.75rem;';
            successMsg.textContent = 'Registration successful! Please sign in.';
            elements.loginForm.querySelector('form').appendChild(successMsg);
            setTimeout(() => successMsg.remove(), 3000);
        } else {
            elements.registerError.textContent = data.error || 'Registration failed';
        }
    } catch (error) {
        elements.registerError.textContent = 'Network error. Please try again.';
        console.error('Register error:', error);
    }
}

function handleLogout() {
    if (state.ws) {
        state.ws.close();
    }
    
    state.token = null;
    state.user = null;
    state.currentRoomId = null;
    state.rooms = [];
    state.messages = {};
    
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    
    showAuth();
}

// ===== WebSocket Functions =====
function connectWebSocket() {
    if (state.ws) {
        state.ws.close();
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${state.token}`;

    state.ws = new WebSocket(wsUrl);

    state.ws.onopen = () => {
        console.log('WebSocket connected');
        state.reconnectAttempts = 0;
    };

    state.ws.onmessage = (event) => {
        try {
            const message = JSON.parse(event.data);
            handleWebSocketMessage(message);
        } catch (error) {
            console.error('Error parsing WebSocket message:', error);
        }
    };

    state.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
    };

    state.ws.onclose = () => {
        console.log('WebSocket disconnected');
        
        // Attempt to reconnect
        if (state.token && state.reconnectAttempts < state.maxReconnectAttempts) {
            state.reconnectAttempts++;
            setTimeout(() => {
                console.log(`Reconnecting... Attempt ${state.reconnectAttempts}`);
                connectWebSocket();
            }, 2000 * state.reconnectAttempts);
        }
    };
}

function sendWebSocketMessage(action, payload = {}) {
    if (state.ws && state.ws.readyState === WebSocket.OPEN) {
        state.ws.send(JSON.stringify({ action, payload }));
    } else {
        console.error('WebSocket is not connected');
    }
}

function handleWebSocketMessage(message) {
    console.log('WS message:', message);

    switch (message.action) {
        case 'rooms':
            state.rooms = message.data || [];
            renderRooms();
            break;

        case 'messages':
            const { room_id, messages } = message.data;
            state.messages[room_id] = messages || [];
            if (state.currentRoomId === room_id) {
                renderMessages();
            }
            break;

        case 'new_message':
            const msg = message.data;
            if (!state.messages[msg.room_id]) {
                state.messages[msg.room_id] = [];
            }
            state.messages[msg.room_id].unshift(msg);
            
            if (state.currentRoomId === msg.room_id) {
                renderMessages();
                markRoomAsRead(msg.room_id);
            }
            
            // Update room list
            sendWebSocketMessage('get_rooms');
            break;

        case 'room_created':
            state.rooms = message.data.rooms || [];
            renderRooms();
            if (message.data.new_room_id) {
                selectRoom(message.data.new_room_id);
            }
            break;

        case 'left_room':
            if (state.currentRoomId === message.data.room_id) {
                state.currentRoomId = null;
                showNoRoomSelected();
            }
            sendWebSocketMessage('get_rooms');
            break;

        case 'room_renamed':
            const renamedRoom = state.rooms.find(r => r.ID === message.data.room_id);
            if (renamedRoom) {
                renamedRoom.RoomName = message.data.room_name;
                renderRooms();
                if (state.currentRoomId === message.data.room_id) {
                    elements.currentRoomName.textContent = message.data.room_name;
                }
            }
            break;

        case 'members':
            state.members[message.data.room_id] = message.data.members || [];
            if (elements.membersModal.classList.contains('active')) {
                renderMembers(message.data.room_id);
            }
            updateMembersCount(message.data.room_id);
            break;

        case 'member_added':
        case 'member_joined':
            sendWebSocketMessage('get_members', { room_id: state.currentRoomId });
            break;

        case 'member_left':
            sendWebSocketMessage('get_members', { room_id: state.currentRoomId });
            break;

        case 'added_to_room':
            sendWebSocketMessage('get_rooms');
            break;

        case 'messages_read':
            // Update UI to show messages as read
            sendWebSocketMessage('get_rooms');
            break;

        case 'pong':
            // Response to ping
            break;

        default:
            if (!message.success && message.error) {
                console.error('WebSocket error:', message.error);
                showNotification(message.error, 'error');
            }
    }
}

// ===== Room Functions =====
function renderRooms() {
    elements.roomsList.innerHTML = '';

    if (state.rooms.length === 0) {
        elements.roomsList.innerHTML = `
            <div class="empty-state">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
                    <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
                </svg>
                <p>No rooms yet. Create one to start!</p>
            </div>
        `;
        return;
    }

    state.rooms.forEach(room => {
        const roomEl = document.createElement('div');
        roomEl.className = 'room-item';
        if (state.currentRoomId === room.ID) {
            roomEl.classList.add('active');
        }

        const lastMessage = room.LastMessage;
        const lastMessageText = lastMessage 
            ? `${lastMessage.nickname}: ${lastMessage.content}`
            : 'No messages yet';
        
        const lastMessageTime = lastMessage 
            ? formatTime(lastMessage.created_at)
            : '';

        roomEl.innerHTML = `
            <div class="room-header">
                <div class="room-name">${escapeHtml(room.RoomName)}</div>
                <div class="room-time">${lastMessageTime}</div>
            </div>
            <div class="room-last-message">${escapeHtml(lastMessageText)}</div>
            ${room.UnreadCount > 0 ? `<div class="unread-badge">${room.UnreadCount}</div>` : ''}
        `;

        roomEl.addEventListener('click', () => selectRoom(room.ID));
        elements.roomsList.appendChild(roomEl);
    });
}

function selectRoom(roomId) {
    state.currentRoomId = roomId;
    const room = state.rooms.find(r => r.ID === roomId);
    
    if (!room) return;

    // Update UI
    elements.currentRoomName.textContent = room.RoomName;
    showChatContainer();
    
    // Load messages
    sendWebSocketMessage('get_messages', {
        room_id: roomId,
        limit: 50,
        offset: 0
    });

    // Load members
    sendWebSocketMessage('get_members', { room_id: roomId });

    // Mark as read
    markRoomAsRead(roomId);

    // Update room list selection
    renderRooms();
}

function markRoomAsRead(roomId) {
    sendWebSocketMessage('mark_read', { room_id: roomId });
}

async function handleCreateRoom(e) {
    e.preventDefault();
    const roomName = document.getElementById('new-room-name').value;

    sendWebSocketMessage('create_room', { room_name: roomName });
    
    closeModal(elements.createRoomModal);
    document.getElementById('new-room-name').value = '';
}

async function handleRenameRoom(e) {
    e.preventDefault();
    const roomName = document.getElementById('rename-room-name').value;

    sendWebSocketMessage('rename_room', {
        room_id: state.currentRoomId,
        room_name: roomName
    });

    closeModal(elements.renameRoomModal);
    document.getElementById('rename-room-name').value = '';
}

async function handleLeaveRoom() {
    if (!confirm('Are you sure you want to leave this room?')) {
        return;
    }

    sendWebSocketMessage('leave_room', { room_id: state.currentRoomId });
}

// ===== Message Functions =====
function renderMessages() {
    const messages = state.messages[state.currentRoomId] || [];
    elements.messagesContainer.innerHTML = '';

    if (messages.length === 0) {
        elements.messagesContainer.innerHTML = `
            <div class="empty-state">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
                    <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
                </svg>
                <p>No messages yet. Start the conversation!</p>
            </div>
        `;
        return;
    }

    messages.forEach(msg => {
        const messageEl = document.createElement('div');
        messageEl.className = 'message';
        
        const isOwn = msg.user_id === state.user?.id || msg.nickname === state.user?.nickname;
        if (isOwn) {
            messageEl.classList.add('own');
        }

        const initial = msg.nickname ? msg.nickname.charAt(0).toUpperCase() : '?';

        messageEl.innerHTML = `
            <div class="message-avatar">${initial}</div>
            <div class="message-content">
                <div class="message-header">
                    <span class="message-author">${escapeHtml(msg.nickname || 'Unknown')}</span>
                    <span class="message-time">${formatTime(msg.created_at)}</span>
                </div>
                <div class="message-text">${escapeHtml(msg.content)}</div>
            </div>
        `;

        elements.messagesContainer.appendChild(messageEl);
    });

    // Scroll to bottom (since we're using flex-direction: column-reverse)
    elements.messagesContainer.scrollTop = 0;
}

function handleSendMessage(e) {
    e.preventDefault();
    const content = elements.messageInput.value.trim();

    if (!content || !state.currentRoomId) return;

    sendWebSocketMessage('send_message', {
        room_id: state.currentRoomId,
        content
    });

    elements.messageInput.value = '';
}

// ===== Member Functions =====
function loadMembers(roomId) {
    sendWebSocketMessage('get_members', { room_id: roomId });
}

function renderMembers(roomId) {
    const members = state.members[roomId] || [];
    elements.membersList.innerHTML = '';

    if (members.length === 0) {
        elements.membersList.innerHTML = '<div class="empty-state">No members found</div>';
        return;
    }

    members.forEach(member => {
        const memberEl = document.createElement('div');
        memberEl.className = 'member-item';

        const initial = member.Nickname ? member.Nickname.charAt(0).toUpperCase() : '?';

        memberEl.innerHTML = `
            <div class="member-avatar">${initial}</div>
            <div class="member-info">
                <div class="member-nickname">${escapeHtml(member.Nickname)}</div>
                <div class="member-login">@${escapeHtml(member.Login)}</div>
            </div>
        `;

        elements.membersList.appendChild(memberEl);
    });
}

function updateMembersCount(roomId) {
    const members = state.members[roomId] || [];
    if (state.currentRoomId === roomId) {
        elements.roomMembersCount.textContent = `${members.length} member${members.length !== 1 ? 's' : ''}`;
    }
}

async function handleAddMember(e) {
    e.preventDefault();
    const username = document.getElementById('member-username').value;

    sendWebSocketMessage('add_member', {
        room_id: state.currentRoomId,
        login: username
    });

    closeModal(elements.addMemberModal);
    document.getElementById('member-username').value = '';
}

// ===== UI Functions =====
function showAuth() {
    elements.authScreen.classList.add('active');
    elements.appScreen.classList.remove('active');
}

function showApp() {
    elements.authScreen.classList.remove('active');
    elements.appScreen.classList.add('active');
    elements.currentUserNickname.textContent = state.user.nickname;
}

function showChatContainer() {
    elements.noRoomSelected.style.display = 'none';
    elements.chatContainer.style.display = 'flex';
}

function showNoRoomSelected() {
    elements.noRoomSelected.style.display = 'flex';
    elements.chatContainer.style.display = 'none';
}

function openModal(modal) {
    modal.classList.add('active');
}

function closeModal(modal) {
    modal.classList.remove('active');
}

function showNotification(message, type = 'info') {
    // Simple console notification for now
    console.log(`[${type.toUpperCase()}]`, message);
}

// ===== Utility Functions =====
function formatTime(timestamp) {
    if (!timestamp) return '';
    
    const date = new Date(timestamp);
    const now = new Date();
    const diff = now - date;
    
    // Less than 1 minute
    if (diff < 60000) {
        return 'Just now';
    }
    
    // Less than 1 hour
    if (diff < 3600000) {
        const minutes = Math.floor(diff / 60000);
        return `${minutes}m ago`;
    }
    
    // Less than 24 hours
    if (diff < 86400000) {
        const hours = Math.floor(diff / 3600000);
        return `${hours}h ago`;
    }
    
    // Less than 7 days
    if (diff < 604800000) {
        const days = Math.floor(diff / 86400000);
        return `${days}d ago`;
    }
    
    // Format as date
    return date.toLocaleDateString();
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// ===== Start Application =====
document.addEventListener('DOMContentLoaded', init);
