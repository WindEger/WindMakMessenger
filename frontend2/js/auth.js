// Управление авторизацией
let currentToken = null;
let currentUser = null;
let currentUserId = null;

function switchTab(tab) {
    const loginForm = document.getElementById('loginForm');
    const registerForm = document.getElementById('registerForm');
    const tabs = document.querySelectorAll('.tab-btn');
    
    if (tab === 'login') {
        loginForm.classList.add('active');
        registerForm.classList.remove('active');
        tabs[0].classList.add('active');
        tabs[1].classList.remove('active');
    } else {
        loginForm.classList.remove('active');
        registerForm.classList.add('active');
        tabs[0].classList.remove('active');
        tabs[1].classList.add('active');
    }
    
    // Очищаем ошибки
    const errorDiv = document.getElementById('authError');
    if (errorDiv) errorDiv.textContent = '';
}

async function handleLogin(event) {
    event.preventDefault();
    const login = document.getElementById('loginLogin').value.trim();
    const password = document.getElementById('loginPassword').value;
    
    if (!login || !password) {
        showAuthError('Пожалуйста, заполните все поля');
        return;
    }
    
    try {
        const response = await fetch('/login', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ login, password })
        });
        
        if (response.ok) {
            const data = await response.json();
            currentToken = data.token;
            localStorage.setItem('token', currentToken);
            localStorage.setItem('userLogin', login);
            
            // Декодируем токен для получения user_id
            try {
                const payload = JSON.parse(atob(currentToken.split('.')[1]));
                currentUserId = payload.user_id;
                localStorage.setItem('userId', currentUserId);
            } catch (e) {
                console.error('Failed to decode token:', e);
            }
            
            await loginSuccess(login);
        } else {
            const error = await response.text();
            showAuthError(error || 'Неверный логин или пароль');
        }
    } catch (error) {
        console.error('Login error:', error);
        showAuthError('Ошибка соединения с сервером');
    }
}

async function handleRegister(event) {
    event.preventDefault();
    
    // Получаем значения напрямую из полей ввода
    const loginInput = document.getElementById('regLogin');
    const passwordInput = document.getElementById('regPassword');
    
    const login = loginInput ? loginInput.value.trim() : '';
    const password = passwordInput ? passwordInput.value : '';
    
    console.log('Register attempt:', { login: login, passwordLength: password.length });
    
    if (!login || !password) {
        showAuthError('Пожалуйста, заполните все поля');
        return;
    }
    
    if (login.length < 3) {
        showAuthError('Логин должен быть не менее 3 символов');
        return;
    }
    
    if (password.length < 6) {
        showAuthError('Пароль должен быть не менее 6 символов');
        return;
    }
    
    // Создаем объект для отправки
    const requestData = {
        login: login,
        password: password
    };
    
    console.log('Sending data:', requestData);
    
    try {
        const response = await fetch('/register', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'application/json'
            },
            body: JSON.stringify(requestData)
        });
        
        console.log('Response status:', response.status);
        
        if (response.status === 201) {
            showAuthError('Регистрация успешна! Теперь войдите', 'success');
            if (loginInput) loginInput.value = '';
            if (passwordInput) passwordInput.value = '';
            setTimeout(() => switchTab('login'), 1500);
        } else {
            const error = await response.text();
            console.error('Register error:', error);
            showAuthError(error || 'Ошибка регистрации');
        }
    } catch (error) {
        console.error('Network error:', error);
        showAuthError('Ошибка соединения с сервером');
    }
}

function showAuthError(message, type = 'error') {
    const errorDiv = document.getElementById('authError');
    if (!errorDiv) return;
    
    errorDiv.textContent = message;
    errorDiv.style.background = type === 'error' ? '#fee' : '#e8f5e9';
    errorDiv.style.color = type === 'error' ? '#c33' : '#2e7d32';
    errorDiv.style.display = 'block';
    
    setTimeout(() => {
        errorDiv.style.display = 'none';
        errorDiv.textContent = '';
    }, 3000);
}

async function loginSuccess(login) {
    currentUser = login;
    
    // Скрываем экран авторизации
    const authScreen = document.getElementById('authScreen');
    const chatApp = document.getElementById('chatApp');
    
    if (authScreen) authScreen.style.display = 'none';
    if (chatApp) chatApp.style.display = 'flex';
    
    // Обновляем информацию о пользователе
    const userLoginSpan = document.getElementById('currentUserLogin');
    if (userLoginSpan) userLoginSpan.textContent = login;
    
    // Инициализируем WebSocket соединение
    if (typeof initWebSocket === 'function') {
        await initWebSocket(currentToken);
    }
    
    // Загружаем список пользователей для создания комнат
    if (typeof loadAvailableUsers === 'function') {
        await loadAvailableUsers();
    }
}

function logout() {
    // Закрываем WebSocket соединение
    if (typeof ws !== 'undefined' && ws) {
        ws.close();
    }
    
    // Очищаем localStorage
    localStorage.removeItem('token');
    localStorage.removeItem('userLogin');
    localStorage.removeItem('userId');
    
    // Сбрасываем глобальные переменные
    currentToken = null;
    currentUser = null;
    currentUserId = null;
    
    // Показываем экран авторизации
    const authScreen = document.getElementById('authScreen');
    const chatApp = document.getElementById('chatApp');
    
    if (authScreen) authScreen.style.display = 'flex';
    if (chatApp) chatApp.style.display = 'none';
    
    // Очищаем поля ввода
    const loginInput = document.getElementById('loginLogin');
    const passwordInput = document.getElementById('loginPassword');
    if (loginInput) loginInput.value = '';
    if (passwordInput) passwordInput.value = '';
    
    // Сбрасываем состояние чата
    if (typeof resetChatState === 'function') {
        resetChatState();
    }
}

// Проверяем сохраненный токен при загрузке
async function checkSavedToken() {
    const token = localStorage.getItem('token');
    const userLogin = localStorage.getItem('userLogin');
    const userId = localStorage.getItem('userId');
    
    if (token && userLogin) {
        currentToken = token;
        currentUser = userLogin;
        currentUserId = userId ? parseInt(userId) : null;
        
        // Если нет userId, пытаемся декодировать из токена
        if (!currentUserId) {
            try {
                const payload = JSON.parse(atob(token.split('.')[1]));
                currentUserId = payload.user_id;
                localStorage.setItem('userId', currentUserId);
            } catch (e) {
                console.error('Failed to decode token:', e);
            }
        }
        
        await loginSuccess(userLogin);
    }
}

// Экспортируем функции для глобального использования
window.switchTab = switchTab;
window.handleLogin = handleLogin;
window.handleRegister = handleRegister;
window.logout = logout;
window.getCurrentUserId = () => currentUserId;
window.getCurrentToken = () => currentToken;

// Инициализация обработчиков событий
document.addEventListener('DOMContentLoaded', () => {
    const loginForm = document.getElementById('loginForm');
    const registerForm = document.getElementById('registerForm');
    const logoutBtn = document.getElementById('logoutBtn');
    
    if (loginForm) {
        loginForm.addEventListener('submit', handleLogin);
    }
    
    if (registerForm) {
        registerForm.addEventListener('submit', handleRegister);
    }
    
    if (logoutBtn) {
        logoutBtn.addEventListener('click', logout);
    }
    
    // Проверяем сохраненный токен
    checkSavedToken();
});