/* ═══════════════════════════════════════════════════════
   STATE
═══════════════════════════════════════════════════════ */
let token = localStorage.getItem('token') || '';
let me    = JSON.parse(localStorage.getItem('me') || 'null');
let ws    = null;
let wsReady = false;

let rooms        = [];
let activeRoomID = null;
let msgCache     = {};
let msgOffset    = {};
let noMoreMsgs   = {};

/* ═══════════════════════════════════════════════════════
   AUTH
═══════════════════════════════════════════════════════ */
function switchTab(tab) {
  document.getElementById('tab-login').classList.toggle('active', tab === 'login');
  document.getElementById('tab-register').classList.toggle('active', tab === 'register');
  document.getElementById('form-login').classList.toggle('hidden', tab !== 'login');
  document.getElementById('form-register').classList.toggle('hidden', tab !== 'register');
}

async function apiPost(url, body) {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  return res.json();
}

async function doLogin(e) {
  e.preventDefault();
  const errEl = document.getElementById('login-err');
  errEl.classList.remove('show');
  try {
    const data = await apiPost('/login', {
      login:    document.getElementById('l-login').value.trim(),
      password: document.getElementById('l-pass').value
    });
    if (!data.success) throw new Error(data.error || 'Ошибка входа');
    token = data.data.token;
    me    = { login: data.data.login, nickname: data.data.nickname };
    localStorage.setItem('token', token);
    localStorage.setItem('me', JSON.stringify(me));
    startApp();
  } catch (err) {
    errEl.textContent = err.message;
    errEl.classList.add('show');
  }
}

async function doRegister(e) {
  e.preventDefault();
  const errEl = document.getElementById('reg-err');
  errEl.classList.remove('show');
  try {
    const data = await apiPost('/register', {
      login:          document.getElementById('r-login').value.trim(),
      nickname:       document.getElementById('r-nick').value.trim(),
      password:       document.getElementById('r-pass').value,
      repeatPassword: document.getElementById('r-pass2').value
    });
    if (!data.success) throw new Error(data.error || 'Ошибка регистрации');
    toast('Аккаунт создан — войдите');
    switchTab('login');
    document.getElementById('l-login').value = document.getElementById('r-login').value;
  } catch (err) {
    errEl.textContent = err.message;
    errEl.classList.add('show');
  }
}

function doLogout() {
  localStorage.removeItem('token');
  localStorage.removeItem('me');
  token = ''; me = null;
  if (ws) { ws.onclose = null; ws.close(); ws = null; }
  wsReady = false; rooms = []; msgCache = {}; activeRoomID = null;
  document.getElementById('app').classList.remove('visible');
  document.getElementById('auth-screen').style.display = 'flex';
  document.getElementById('chat-main').style.display = 'none';
  document.getElementById('chat-placeholder').style.display = 'flex';
}

/* ═══════════════════════════════════════════════════════
   APP START
═══════════════════════════════════════════════════════ */
function startApp() {
  document.getElementById('auth-screen').style.display = 'none';
  document.getElementById('app').classList.add('visible');
  const nick = me.nickname || me.login;
  document.getElementById('me-nick').textContent = nick;
  document.getElementById('me-login').textContent = '@' + me.login;
  document.getElementById('me-avatar').textContent = nick[0].toUpperCase();
  connectWS();
  setupInputs();
  setupTopbarButtons();
}

/* ═══════════════════════════════════════════════════════
   WEBSOCKET
═══════════════════════════════════════════════════════ */
function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(`${proto}://${location.host}/ws?token=${encodeURIComponent(token)}`);
  setDot('connecting');

  ws.onopen  = () => { wsReady = true;  setDot('connected'); };
  ws.onerror = () => { setDot('error'); };
  ws.onclose = () => {
    wsReady = false;
    setDot('disconnected');
    setTimeout(() => { if (token) connectWS(); }, 3000);
  };
  ws.onmessage = (e) => {
    try { handleMsg(JSON.parse(e.data)); } catch(err) { console.error('[WS] parse error', err); }
  };
}

function wsSend(action, payload) {
  if (ws && wsReady) {
    ws.send(JSON.stringify({ action, payload: payload || {} }));
  } else {
    toast('Нет соединения', true);
  }
}

function setDot(state) {
  const d = document.getElementById('conn-dot');
  d.className = 'conn-dot';
  if (state === 'connected') d.classList.add('connected');
  if (state === 'error')     d.classList.add('error');
}

/* ═══════════════════════════════════════════════════════
   SERVER MESSAGES
═══════════════════════════════════════════════════════ */
let globalInterceptor = null;

function handleMsg(msg) {
  if (globalInterceptor && globalInterceptor(msg)) return;

  if (!msg.success && msg.error) {
    toast('⚠ ' + msg.error, true);
    return;
  }

  switch (msg.action) {
    case 'init':
      applyRooms(msg.data.rooms || []);
      break;
    case 'rooms':
      applyRooms(msg.data || []);
      break;
    case 'room_created':
      applyRooms(msg.data.rooms || []);
      if (msg.data.new_room_id) openRoom(msg.data.new_room_id);
      break;
    case 'messages':
      onMessages(msg.data);
      break;
    case 'new_message':
      onNewMessage(msg.data);
      break;
    case 'left_room':
      rooms = rooms.filter(r => r.id !== msg.data.room_id);
      delete msgCache[msg.data.room_id];
      if (activeRoomID === msg.data.room_id) {
        activeRoomID = null;
        showPlaceholder();
      }
      renderRoomsList();
      toast('Вы покинули чат');
      break;
    case 'added_to_room':
      wsSend('get_rooms');
      toast('Вас добавили в чат: ' + (msg.data.roomName || msg.data.RoomName));
      break;
    case 'room_renamed':
      const r = rooms.find(x => x.id === msg.data.room_id);
      if (r) {
        r.roomName = msg.data.room_name;
        renderRoomsList();
        if (activeRoomID === msg.data.room_id)
          document.getElementById('chat-title').textContent = msg.data.room_name;
      }
      break;
    case 'messages_read':
      const room = rooms.find(x => x.id === msg.data.room_id);
      if (room) { room.unread_count = 0; renderRoomsList(); }
      break;
    case 'members':
      renderMembersList(msg.data.members || []);
      break;
    case 'member_added':
      toast('Участник добавлен');
      break;
    default:
      console.log('[WS]', msg.action, msg);
  }
}

/* ═══════════════════════════════════════════════════════
   ROOMS
═══════════════════════════════════════════════════════ */
function applyRooms(data) {
  rooms = data;
  renderRoomsList();
}

function renderRoomsList() {
  const el = document.getElementById('rooms-list');
  if (!rooms.length) {
    el.innerHTML = '<div class="rooms-empty">нет чатов<br><span style="font-size:10px;margin-top:6px;display:block">нажмите ＋ чтобы создать</span></div>';
    return;
  }
  el.innerHTML = rooms.map(r => {
    const letter = (r.roomName || r.RoomName || '?')[0].toUpperCase();
    const name   = escHtml(r.roomName || r.RoomName || '');
    const lm     = r.last_message;
    const lastText = lm
      ? escHtml((lm.nickname || lm.UserNickname || '') + ': ' + (lm.content || lm.Content || '')).slice(0, 45)
      : '';
    const badge = r.unread_count > 0
      ? `<span class="unread-badge">${r.unread_count > 99 ? '99+' : r.unread_count}</span>` : '';
    const active = r.id === activeRoomID ? ' active' : '';
    return `<div class="room-item${active}" data-id="${r.id}" onclick="openRoom(${r.id})">
      <div class="room-avatar">${letter}</div>
      <div class="room-info">
        <div class="room-name">${name}</div>
        <div class="room-last">${lastText || '—'}</div>
      </div>
      ${badge}
    </div>`;
  }).join('');
}

/* ═══════════════════════════════════════════════════════
   OPEN ROOM
═══════════════════════════════════════════════════════ */
function openRoom(roomID) {
  activeRoomID = roomID;
  renderRoomsList();

  const room = rooms.find(r => r.id === roomID);
  document.getElementById('chat-title').textContent = room ? (room.roomName || room.RoomName) : '—';

  document.getElementById('chat-placeholder').style.display = 'none';
  document.getElementById('chat-main').style.display = 'flex';

  msgCache[roomID]  = [];
  msgOffset[roomID] = 0;
  noMoreMsgs[roomID] = false;

  const area = document.getElementById('messages-area');
  area.innerHTML = '<div class="load-more-wrap" id="load-more-wrap"><button class="load-more-btn" id="load-more-btn" onclick="loadMore()">Загрузить ещё</button></div>';
  document.getElementById('load-more-wrap').style.display = 'none';

  wsSend('get_messages', { room_id: roomID, limit: 50, offset: 0 });

  const r = rooms.find(x => x.id === roomID);
  if (r) { r.unread_count = 0; renderRoomsList(); }
}

function showPlaceholder() {
  document.getElementById('chat-placeholder').style.display = 'flex';
  document.getElementById('chat-main').style.display = 'none';
}

/* ═══════════════════════════════════════════════════════
   MESSAGES
═══════════════════════════════════════════════════════ */
function onMessages(data) {
  const roomID  = data.room_id;
  const msgs    = data.messages || [];
  const isFirst = data.offset === 0;

  if (roomID !== activeRoomID) return;

  const ordered = [...msgs].reverse();

  if (isFirst) {
    msgCache[roomID]  = ordered;
    msgOffset[roomID] = ordered.length;
  } else {
    msgCache[roomID]  = [...ordered, ...msgCache[roomID]];
    msgOffset[roomID] += ordered.length;
  }

  const hasMore = msgs.length >= data.limit;
  noMoreMsgs[roomID] = !hasMore;

  if (isFirst) {
    renderAllMessages(roomID);
    scrollBottom();
  } else {
    prependMessages(roomID, ordered);
  }
}

function renderAllMessages(roomID) {
  const area = document.getElementById('messages-area');
  const lmw  = document.getElementById('load-more-wrap');

  Array.from(area.children).forEach(el => {
    if (!el.classList.contains('load-more-wrap')) el.remove();
  });

  lmw.style.display = noMoreMsgs[roomID] ? 'none' : 'block';

  const msgs = msgCache[roomID] || [];
  msgs.forEach(m => area.appendChild(buildBubble(m)));
}

function prependMessages(roomID, newMsgs) {
  const area = document.getElementById('messages-area');
  const lmw  = document.getElementById('load-more-wrap');
  const prevHeight = area.scrollHeight;

  newMsgs.forEach(m => {
    lmw.insertAdjacentElement('afterend', buildBubble(m));
  });

  lmw.style.display = noMoreMsgs[roomID] ? 'none' : 'block';
  area.scrollTop = area.scrollHeight - prevHeight;
}

function onNewMessage(msg) {
  const roomID   = msg.room_id;
  if (!msgCache[roomID]) msgCache[roomID] = [];
  msgCache[roomID].push(msg);
  if (msgOffset[roomID] !== undefined) msgOffset[roomID]++;

  const r = rooms.find(x => x.id === roomID);
  if (r) {
    r.last_message = msg;
    if (roomID !== activeRoomID) r.unread_count = (r.unread_count || 0) + 1;
    rooms = [r, ...rooms.filter(x => x.id !== roomID)];
    renderRoomsList();
  }

  if (roomID === activeRoomID) {
    const area = document.getElementById('messages-area');
    area.appendChild(buildBubble(msg));
    scrollBottom();
    wsSend('mark_read', { room_id: roomID });
  }
}

function buildBubble(msg) {
  const myID   = getMyID();
  const userID = msg.user_id  !== undefined ? msg.user_id  : msg.UserID;
  const isMe   = userID === myID;
  const nick   = msg.nickname || msg.UserNickname || '?';
  const text   = msg.content  || msg.Content  || '';
  const time   = formatTime(msg.created_at || msg.CreatedDateTime);

  const div = document.createElement('div');
  div.className = 'msg-group' + (isMe ? ' me' : '');
  div.innerHTML = `
    ${!isMe ? `<div class="msg-author">${escHtml(nick)}</div>` : ''}
    <div class="msg-bubble">${escHtml(text)}</div>
    <div class="msg-time">${time}</div>
  `;
  return div;
}

function loadMore() {
  if (!activeRoomID || noMoreMsgs[activeRoomID]) return;
  wsSend('get_messages', {
    room_id: activeRoomID,
    limit:   50,
    offset:  msgOffset[activeRoomID] || 0
  });
}

function scrollBottom() {
  const area = document.getElementById('messages-area');
  area.scrollTop = area.scrollHeight;
}

/* ═══════════════════════════════════════════════════════
   SEND MESSAGE
═══════════════════════════════════════════════════════ */
function sendMessage() {
  if (!activeRoomID) return;
  const input = document.getElementById('msg-input');
  const text  = input.value.trim();
  if (!text) return;
  wsSend('send_message', { room_id: activeRoomID, content: text });
  input.value = '';
  input.style.height = 'auto';
}

function setupInputs() {
  const input = document.getElementById('msg-input');
  input.addEventListener('input', () => {
    input.style.height = 'auto';
    input.style.height = Math.min(input.scrollHeight, 130) + 'px';
  });
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
  });
  document.getElementById('send-btn').addEventListener('click', sendMessage);
}

/* ═══════════════════════════════════════════════════════
   TOPBAR BUTTONS
═══════════════════════════════════════════════════════ */
function setupTopbarButtons() {
  document.getElementById('btn-members').addEventListener('click', openMembers);
  document.getElementById('btn-add-member').addEventListener('click', openAddMember);
  document.getElementById('btn-rename').addEventListener('click', openRename);
  document.getElementById('btn-leave').addEventListener('click', confirmLeave);
  document.getElementById('load-more-btn') && document.getElementById('load-more-btn').addEventListener('click', loadMore);
}

/* ═══════════════════════════════════════════════════════
   MODALS
═══════════════════════════════════════════════════════ */
function openModal(id) { document.getElementById(id).classList.remove('hidden'); }
function closeModal(id) { document.getElementById(id).classList.add('hidden'); }

document.querySelectorAll('.modal-overlay').forEach(o =>
  o.addEventListener('click', e => { if (e.target === o) o.classList.add('hidden'); })
);
document.addEventListener('keydown', e => {
  if (e.key === 'Escape')
    document.querySelectorAll('.modal-overlay').forEach(o => o.classList.add('hidden'));
});

function openCreateRoom() {
  document.getElementById('new-room-name').value = '';
  openModal('modal-create-room');
  setTimeout(() => document.getElementById('new-room-name').focus(), 50);
}

function doCreateRoom() {
  const name = document.getElementById('new-room-name').value.trim();
  if (!name) return;
  closeModal('modal-create-room');
  wsSend('create_room', { room_name: name });
}

document.getElementById('new-room-name').addEventListener('keydown', e => {
  if (e.key === 'Enter') doCreateRoom();
});

function openAddMember() {
  if (!activeRoomID) return;
  document.getElementById('add-member-login').value = '';
  openModal('modal-add-member');
  setTimeout(() => document.getElementById('add-member-login').focus(), 50);
}

function doAddMember() {
  const login = document.getElementById('add-member-login').value.trim();
  if (!login || !activeRoomID) return;
  closeModal('modal-add-member');
  wsSend('add_member', { room_id: activeRoomID, login });
}

document.getElementById('add-member-login').addEventListener('keydown', e => {
  if (e.key === 'Enter') doAddMember();
});

function openRename() {
  if (!activeRoomID) return;
  const r = rooms.find(x => x.id === activeRoomID);
  document.getElementById('rename-input').value = r ? (r.roomName || r.RoomName || '') : '';
  openModal('modal-rename');
  setTimeout(() => document.getElementById('rename-input').focus(), 50);
}

function doRenameRoom() {
  const name = document.getElementById('rename-input').value.trim();
  if (!name || !activeRoomID) return;
  closeModal('modal-rename');
  wsSend('rename_room', { room_id: activeRoomID, room_name: name });
}

document.getElementById('rename-input').addEventListener('keydown', e => {
  if (e.key === 'Enter') doRenameRoom();
});

function openMembers() {
  if (!activeRoomID) return;
  document.getElementById('members-list').innerHTML =
    '<div style="color:var(--text3);font-size:12px;font-family:var(--font-mono)">Загрузка...</div>';
  openModal('modal-members');

  globalInterceptor = (msg) => {
    if (msg.action === 'members') {
      renderMembersList(msg.data.members || []);
      globalInterceptor = null;
      return true;
    }
    if (!msg.success && msg.error) {
      globalInterceptor = null;
      return false;
    }
    return false;
  };
  setTimeout(() => { globalInterceptor = null; }, 5000);

  wsSend('get_members', { room_id: activeRoomID });
}

function renderMembersList(members) {
  const el = document.getElementById('members-list');
  if (!members.length) {
    el.innerHTML = '<div style="color:var(--text3);font-size:12px">Нет участников</div>';
    return;
  }
  el.innerHTML = members.map(u => {
    const nick  = u.Nickname || u.nickname || u.Login || u.login || '?';
    const login = u.Login    || u.login    || '';
    return `<div class="member-row">
      <div class="member-avatar">${escHtml(nick[0].toUpperCase())}</div>
      <div>
        <div class="member-name">${escHtml(nick)}</div>
        <div class="member-login">@${escHtml(login)}</div>
      </div>
    </div>`;
  }).join('');
}

function confirmLeave() {
  if (!activeRoomID) return;
  const r = rooms.find(x => x.id === activeRoomID);
  const name = r ? (r.roomName || r.RoomName) : 'этот чат';
  if (!confirm(`Покинуть "${name}"?`)) return;
  wsSend('leave_room', { room_id: activeRoomID });
}

/* ═══════════════════════════════════════════════════════
   HELPERS
═══════════════════════════════════════════════════════ */
let _myID = null;
function getMyID() {
  if (_myID !== null) return _myID;
  try {
    const payload = JSON.parse(atob(token.split('.')[1].replace(/-/g,'+').replace(/_/g,'/')));
    _myID = payload.user_id;
  } catch { _myID = -1; }
  return _myID;
}

function escHtml(s) {
  return String(s)
    .replace(/&/g,'&amp;').replace(/</g,'&lt;')
    .replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function formatTime(dt) {
  if (!dt) return '';
  const d = new Date(dt);
  if (isNaN(d.getTime())) return '';
  const now = new Date();
  const isToday = d.toDateString() === now.toDateString();
  if (isToday) return d.toLocaleTimeString('ru', { hour: '2-digit', minute: '2-digit' });
  return d.toLocaleDateString('ru', { day: '2-digit', month: '2-digit' })
       + ' ' + d.toLocaleTimeString('ru', { hour: '2-digit', minute: '2-digit' });
}

let toastTimer = null;
function toast(msg, isError = false) {
  const el = document.getElementById('toast');
  el.textContent = msg;
  el.className = isError ? 'error' : '';
  el.classList.add('show');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.remove('show'), 3200);
}

/* ═══════════════════════════════════════════════════════
   BOOTSTRAP
═══════════════════════════════════════════════════════ */
if (token && me) startApp();