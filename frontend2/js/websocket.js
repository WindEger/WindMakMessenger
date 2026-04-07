// WebSocket управление
let ws = null;
let currentRoomId = null;
let reconnectAttempts = 0;
const maxReconnectAttempts = 10;
let reconnectTimeout = null;

function initWebSocket(token) {
    if (!token) {
        console.error('No token provided for WebSocket');
        return;
    }
    
    // Определяем протокол WebSocket
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/hub?token=${token}`;
    
    console.log('Connecting to WebSocket:', wsUrl);
    
    try {
        ws = new WebSocket(wsUrl);
        
        ws.onopen = () => {
            console.log('WebSocket connected successfully');
            reconnectAttempts = 0;
            updateConnectionStatus(true);
        };
        
        ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                handleWebSocketMessage(data);
            } catch (e) {
                console.error('Failed to parse WebSocket message:', e);
            }
        };
        
        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            updateConnectionStatus(false);
        };
        
        ws.onclose = (event) => {
            console.log('WebSocket disconnected:', event.code, event.reason);
            updateConnectionStatus(false);
            
            // Попытка переподключения
            if (reconnectAttempts < maxReconnectAttempts) {
                const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), 30000);
                console.log(`Reconnecting in ${delay}ms... (attempt ${reconnectAttempts + 1}/${maxReconnectAttempts})`);
                
                if (reconnectTimeout) clearTimeout(reconnectTimeout);
                reconnectTimeout = setTimeout(() => {
                    reconnectAttempts++;
                    if (currentToken) {
                        initWebSocket(currentToken);
                    }
                }, delay);
            } else {
                console.error('Max reconnection attempts reached');
                showConnectionError('Потеряно соединение с сервером. Пожалуйста, обновите страницу.');
            }
        };
    } catch (error) {
        console.error('Failed to create WebSocket:', error);
    }
}

function handleWebSocketMessage(data) {
    console.log('Received WebSocket message:', data.type, data);
    
    switch(data.type) {
        case 'rooms':
            if (typeof displayRooms === 'function') {
                displayRooms(data.rooms);
            }
            break;
            
        case 'room_data':
            if (typeof displayMessages === 'function') {
                displayMessages(data.messages);
            }
            // Обновляем заголовок комнаты
            if (data.room_id && data.room_name && typeof updateRoomHeader === 'function') {
                updateRoomHeader(data.room_name);
            }
            break;
            
        case 'room_members':
            if (typeof displayMembers === 'function') {
                displayMembers(data.users);
            }
            break;
            
        case 'message':
            handleNewMessage(data);
            break;
            
        default:
            console.log('Unknown message type:', data.type);
    }
}

function handleNewMessage(message) {
    // Если сообщение из текущей комнаты, добавляем в чат
    if (message.room_id === currentRoomId) {
        if (typeof addMessageToChat === 'function') {
            addMessageToChat(message);
        }
    } else {
        // Сообщение из другой комнаты - показываем уведомление
        showMessageNotification(message);
    }
}

function showMessageNotification(message) {
    // Подсвечиваем комнату в списке
    const roomElement = document.querySelector(`.room-item[data-room-id="${message.room_id}"]`);
    if (roomElement) {
        roomElement.classList.add('has-new-message');
    }
    
    // Браузерное уведомление
    if (Notification.permission === 'granted' && document.hidden) {
        new Notification(`Новое сообщение от ${message.username}`, {
            body: message.content.length > 100 ? message.content.substring(0, 100) + '...' : message.content,
            icon: '/favicon.ico',
            silent: false
        });
    }
    
    // Воспроизводим звук (опционально)
    playNotificationSound();
}

function playNotificationSound() {
    try {
        const audio = new Audio('/assets/notification.mp3');
        audio.volume = 0.3;
        audio.play().catch(e => console.log('Audio play failed:', e));
    } catch (e) {
        // Звук не доступен
    }
}

function updateConnectionStatus(connected) {
    const statusElement = document.getElementById('connectionStatus');
    if (statusElement) {
        if (connected) {
            statusElement.textContent = '● Онлайн';
            statusElement.style.color = '#4caf50';
        } else {
            statusElement.textContent = '○ Офлайн';
            statusElement.style.color = '#f44336';
        }
    }
}

function showConnectionError(message) {
    const errorDiv = document.createElement('div');
    errorDiv.className = 'connection-error';
    errorDiv.textContent = message;
    errorDiv.style.cssText = `
        position: fixed;
        top: 20px;
        left: 50%;
        transform: translateX(-50%);
        background: #f44336;
        color: white;
        padding: 12px 24px;
        border-radius: 8px;
        z-index: 10000;
        box-shadow: 0 4px 12px rgba(0,0,0,0.3);
        animation: slideDown 0.3s ease;
    `;
    
    document.body.appendChild(errorDiv);
    
    setTimeout(() => {
        errorDiv.remove();
    }, 5000);
}

function sendWebSocketMessage(message) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(message));
        return true;
    } else {
        console.error('WebSocket is not connected. State:', ws?.readyState);
        showConnectionError('Нет соединения с сервером. Сообщение не отправлено.');
        return false;
    }
}

function joinRoom(roomId) {
    if (currentRoomId === roomId) {
        console.log('Already in this room');
        return;
    }
    
    currentRoomId = roomId;
    sendWebSocketMessage({
        type: 'join_room',
        room_id: roomId
    });
    
    // Активируем поле ввода
    const messageInput = document.getElementById('messageInput');
    const sendButton = document.getElementById('sendMessageBtn');
    
    if (messageInput) {
        messageInput.disabled = false;
        messageInput.focus();
    }
    if (sendButton) {
        sendButton.disabled = false;
    }
}

function sendMessage(content) {
    if (!currentRoomId) {
        console.error('No room selected');
        showConnectionError('Выберите комнату для отправки сообщения');
        return false;
    }
    
    if (!content || content.trim() === '') {
        return false;
    }
    
    return sendWebSocketMessage({
        type: 'send_message',
        content: content.trim()
    });
}

function createRoom(name, users) {
    if (!name || !name.trim()) {
        console.error('Room name is required');
        return false;
    }
    
    if (!users || users.length === 0) {
        console.error('At least one user is required');
        return false;
    }
    
    return sendWebSocketMessage({
        type: 'create_room',
        name: name.trim(),
        users: users
    });
}

function leaveRoom() {
    if (currentRoomId) {
        sendWebSocketMessage({
            type: 'leave_room'
        });
        currentRoomId = null;
        
        // Деактивируем поле ввода
        const messageInput = document.getElementById('messageInput');
        const sendButton = document.getElementById('sendMessageBtn');
        
        if (messageInput) {
            messageInput.disabled = true;
            messageInput.value = '';
        }
        if (sendButton) {
            sendButton.disabled = true;
        }
    }
}

function markMessagesAsRead(roomId) {
    sendWebSocketMessage({
        type: 'mark_read',
        room_id: roomId
    });
}

// Запрашиваем разрешение на уведомления
if (Notification.permission === 'default') {
    Notification.requestPermission();
}

// Экспортируем функции для глобального использования
window.initWebSocket = initWebSocket;
window.joinRoom = joinRoom;
window.sendMessage = sendMessage;
window.createRoom = createRoom;
window.leaveRoom = leaveRoom;
window.markMessagesAsRead = markMessagesAsRead;
window.getCurrentRoomId = () => currentRoomId;