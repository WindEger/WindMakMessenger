package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ─── Upgrader ────────────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // в продакшене можно ограничить по Origin
	},
}

// ─── Типы входящих сообщений (action) ────────────────────────────────────────

// Все сообщения от клиента имеют поле "action"
type IncomingMessage struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

// Исходящее сообщение серверу → клиенту
type OutgoingMessage struct {
	Action  string      `json:"action"`
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ─── Payloads от клиента ──────────────────────────────────────────────────────

type PayloadSendMessage struct {
	RoomID  int    `json:"room_id"`
	Content string `json:"content"`
}

type PayloadGetMessages struct {
	RoomID int `json:"room_id"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type PayloadCreateRoom struct {
	RoomName string `json:"room_name"`
}

type PayloadJoinRoom struct {
	RoomID int `json:"room_id"`
}

type PayloadLeaveRoom struct {
	RoomID int `json:"room_id"`
}

type PayloadAddMember struct {
	RoomID int `json:"room_id"`
	Login  string `json:"login"`
}

type PayloadRenameRoom struct {
	RoomID   int    `json:"room_id"`
	RoomName string `json:"room_name"`
}

type PayloadMarkRead struct {
	RoomID int `json:"room_id"`
}

// ─── Hub — центральный менеджер соединений ────────────────────────────────────

type Client struct {
	conn   *websocket.Conn
	userID int
	send   chan OutgoingMessage
	mu     sync.Mutex
}

// Отправить сообщение клиенту (потокобезопасно)
func (c *Client) emit(msg OutgoingMessage) {
	select {
	case c.send <- msg:
	default:
		// буфер переполнен — клиент слишком медленный, пропускаем
	}
}

type Hub struct {
	mu      sync.RWMutex
	clients map[int][]*Client // userID → список соединений (несколько вкладок)
}

var hub = &Hub{
	clients: make(map[int][]*Client),
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c.userID] = append(h.clients[c.userID], c)
	log.Printf("[WS] user %d connected (total sessions: %d)", c.userID, len(h.clients[c.userID]))
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.clients[c.userID]
	for i, cl := range list {
		if cl == c {
			h.clients[c.userID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(h.clients[c.userID]) == 0 {
		delete(h.clients, c.userID)
	}
	log.Printf("[WS] user %d disconnected", c.userID)
}

// Разослать событие всем сессиям конкретного пользователя
func (h *Hub) sendToUser(userID int, msg OutgoingMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients[userID] {
		c.emit(msg)
	}
}

// Разослать событие всем участникам комнаты (кроме себя, если excludeSelf > 0)
func (h *Hub) broadcastToRoom(roomID, excludeUserID int, msg OutgoingMessage) {
	members, err := GetRoomMembers(roomID)
	if err != nil {
		return
	}
	for _, m := range members {
		if excludeUserID > 0 && m.ID == excludeUserID {
			continue
		}
		h.sendToUser(m.ID, msg)
	}
}

// ─── WebSocket Handler ────────────────────────────────────────────────────────

func WSHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Авторизация: токен из query-параметра или заголовка Authorization
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
	}
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	claims, err := ValidateToken(token)
	if err != nil {
		http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
		return
	}

	rawID, ok := claims["user_id"]
	if !ok {
		http.Error(w, "invalid token claims", http.StatusUnauthorized)
		return
	}
	// JWT хранит числа как float64
	userIDFloat, ok := rawID.(float64)
	if !ok {
		http.Error(w, "invalid user_id in token", http.StatusUnauthorized)
		return
	}
	userID := int(userIDFloat)

	// 2. Апгрейд до WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("[WS] upgrade error:", err)
		return
	}

	client := &Client{
		conn:   conn,
		userID: userID,
		send:   make(chan OutgoingMessage, 64),
	}

	hub.register(client)
	defer hub.unregister(client)

	// 3. Горутина для отправки исходящих сообщений
	go func() {
		for msg := range client.send {
			client.mu.Lock()
			err := conn.WriteJSON(msg)
			client.mu.Unlock()
			if err != nil {
				log.Printf("[WS] write error for user %d: %v", userID, err)
				conn.Close()
				return
			}
		}
	}()

	// 4. Отправляем приветствие с актуальными чатами
	go sendInitialData(client)

	// 5. Читаем входящие сообщения
	conn.SetReadDeadline(time.Time{}) // без дедлайна (ping/pong ниже)
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Ping каждые 30 сек для keepalive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			client.mu.Lock()
			err := conn.WriteMessage(websocket.PingMessage, nil)
			client.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	for {
		var incoming IncomingMessage
		err := conn.ReadJSON(&incoming)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[WS] unexpected close for user %d: %v", userID, err)
			}
			break
		}
		handleAction(client, incoming)
	}
}

// ─── Начальные данные после подключения ──────────────────────────────────────

func sendInitialData(c *Client) {
	rooms, err := getUserRoomsWithMeta(c.userID)
	if err != nil {
		c.emit(OutgoingMessage{Action: "error", Success: false, Error: "failed to load rooms"})
		return
	}
	c.emit(OutgoingMessage{
		Action:  "init",
		Success: true,
		Data:    map[string]interface{}{"rooms": rooms},
	})
}

// ─── Диспетчер действий ───────────────────────────────────────────────────────

func handleAction(c *Client, inc IncomingMessage) {
	switch inc.Action {

	// Получить список своих чатов
	case "get_rooms":
		rooms, err := getUserRoomsWithMeta(c.userID)
		if err != nil {
			c.emit(errMsg("get_rooms", err))
			return
		}
		c.emit(OutgoingMessage{Action: "rooms", Success: true, Data: rooms})

	// Получить сообщения комнаты
	case "get_messages":
		var p PayloadGetMessages
		if err := json.Unmarshal(inc.Payload, &p); err != nil {
			c.emit(errMsg("get_messages", fmt.Errorf("invalid payload")))
			return
		}
		if p.Limit <= 0 {
			p.Limit = 50
		}
		if p.Limit > 200 {
			p.Limit = 200
		}

		ok, err := IsUserInRoom(p.RoomID, c.userID)
		if err != nil || !ok {
			c.emit(errMsg("get_messages", fmt.Errorf("access denied")))
			return
		}

		messages, err := GetRoomMessagesWithReadStatus(p.RoomID, c.userID, p.Limit, p.Offset)
		if err != nil {
			c.emit(errMsg("get_messages", err))
			return
		}

		// Уведомляем других участников, что сообщения прочитаны
		hub.broadcastToRoom(p.RoomID, c.userID, OutgoingMessage{
			Action:  "messages_read",
			Success: true,
			Data: map[string]interface{}{
				"room_id": p.RoomID,
				"user_id": c.userID,
			},
		})

		c.emit(OutgoingMessage{
			Action:  "messages",
			Success: true,
			Data: map[string]interface{}{
				"room_id":  p.RoomID,
				"messages": messages,
				"limit":    p.Limit,
				"offset":   p.Offset,
			},
		})

	// Отправить сообщение
	case "send_message":
		var p PayloadSendMessage
		if err := json.Unmarshal(inc.Payload, &p); err != nil {
			c.emit(errMsg("send_message", fmt.Errorf("invalid payload")))
			return
		}

		msgID, err := CreateMessage(p.RoomID, c.userID, p.Content)
		if err != nil {
			c.emit(errMsg("send_message", err))
			return
		}

		user, _ := GetUserByID(c.userID)
		nickname := ""
		if user != nil {
			nickname = user.Nickname
		}

		newMsg := Message{
			ID:              int(msgID),
			RoomID:          p.RoomID,
			UserID:          c.userID,
			Content:         p.Content,
			CreatedDateTime: time.Now(),
			UserNickname:    nickname,
		}

		broadcast := OutgoingMessage{
			Action:  "new_message",
			Success: true,
			Data:    newMsg,
		}

		// Себе подтверждение
		c.emit(broadcast)
		// Остальным участникам
		hub.broadcastToRoom(p.RoomID, c.userID, broadcast)

	// Создать комнату
	case "create_room":
		var p PayloadCreateRoom
		if err := json.Unmarshal(inc.Payload, &p); err != nil {
			c.emit(errMsg("create_room", fmt.Errorf("invalid payload")))
			return
		}
		if p.RoomName == "" {
			p.RoomName = "New Room"
		}

		roomID, err := CreateRoom(p.RoomName, c.userID)
		if err != nil {
			c.emit(errMsg("create_room", err))
			return
		}

		room, _ := GetRoomByID(int(roomID))
		c.emit(OutgoingMessage{
			Action:  "room_created",
			Success: true,
			Data:    room,
		})

	// Вступить в существующую комнату
	case "join_room":
		var p PayloadJoinRoom
		if err := json.Unmarshal(inc.Payload, &p); err != nil {
			c.emit(errMsg("join_room", fmt.Errorf("invalid payload")))
			return
		}

		if err := AddMemberToRoom(p.RoomID, c.userID); err != nil {
			c.emit(errMsg("join_room", err))
			return
		}

		room, _ := GetRoomByID(p.RoomID)
		user, _ := GetUserByID(c.userID)

		c.emit(OutgoingMessage{Action: "joined_room", Success: true, Data: room})

		// Уведомляем других участников
		hub.broadcastToRoom(p.RoomID, c.userID, OutgoingMessage{
			Action:  "member_joined",
			Success: true,
			Data: map[string]interface{}{
				"room_id": p.RoomID,
				"user":    user,
			},
		})

	// Покинуть комнату
	case "leave_room":
		var p PayloadLeaveRoom
		if err := json.Unmarshal(inc.Payload, &p); err != nil {
			c.emit(errMsg("leave_room", fmt.Errorf("invalid payload")))
			return
		}

		if err := LeaveRoom(p.RoomID, c.userID); err != nil {
			c.emit(errMsg("leave_room", err))
			return
		}

		c.emit(OutgoingMessage{Action: "left_room", Success: true, Data: map[string]interface{}{
			"room_id": p.RoomID,
		}})

		// Уведомляем оставшихся
		hub.broadcastToRoom(p.RoomID, c.userID, OutgoingMessage{
			Action:  "member_left",
			Success: true,
			Data: map[string]interface{}{
				"room_id": p.RoomID,
				"user_id": c.userID,
			},
		})

	// Добавить другого пользователя в комнату (по логину)
	case "add_member":
		var p PayloadAddMember
		if err := json.Unmarshal(inc.Payload, &p); err != nil {
			c.emit(errMsg("add_member", fmt.Errorf("invalid payload")))
			return
		}

		// Только участник комнаты может добавлять
		ok, err := IsUserInRoom(p.RoomID, c.userID)
		if err != nil || !ok {
			c.emit(errMsg("add_member", fmt.Errorf("access denied")))
			return
		}

		target, err := GetUserByLogin(p.Login)
		if err != nil {
			c.emit(errMsg("add_member", fmt.Errorf("user not found")))
			return
		}

		if err := AddMemberToRoom(p.RoomID, target.ID); err != nil {
			c.emit(errMsg("add_member", err))
			return
		}

		// Уведомляем нового участника (если онлайн)
		room, _ := GetRoomByID(p.RoomID)
		hub.sendToUser(target.ID, OutgoingMessage{
			Action:  "added_to_room",
			Success: true,
			Data:    room,
		})

		// Уведомляем комнату
		hub.broadcastToRoom(p.RoomID, 0, OutgoingMessage{
			Action:  "member_joined",
			Success: true,
			Data: map[string]interface{}{
				"room_id": p.RoomID,
				"user":    target,
			},
		})

		c.emit(OutgoingMessage{Action: "member_added", Success: true, Data: map[string]interface{}{
			"room_id": p.RoomID,
			"user":    target,
		}})

	// Переименовать комнату (только создатель)
	case "rename_room":
		var p PayloadRenameRoom
		if err := json.Unmarshal(inc.Payload, &p); err != nil {
			c.emit(errMsg("rename_room", fmt.Errorf("invalid payload")))
			return
		}

		room, err := GetRoomByID(p.RoomID)
		if err != nil {
			c.emit(errMsg("rename_room", fmt.Errorf("room not found")))
			return
		}
		if room.CreatedBy != c.userID {
			c.emit(errMsg("rename_room", fmt.Errorf("only the room creator can rename it")))
			return
		}

		if err := UpdateRoomName(p.RoomID, p.RoomName); err != nil {
			c.emit(errMsg("rename_room", err))
			return
		}

		hub.broadcastToRoom(p.RoomID, 0, OutgoingMessage{
			Action:  "room_renamed",
			Success: true,
			Data: map[string]interface{}{
				"room_id":   p.RoomID,
				"room_name": p.RoomName,
			},
		})

	// Получить участников комнаты
	case "get_members":
		var p PayloadJoinRoom // { room_id }
		if err := json.Unmarshal(inc.Payload, &p); err != nil {
			c.emit(errMsg("get_members", fmt.Errorf("invalid payload")))
			return
		}

		ok, err := IsUserInRoom(p.RoomID, c.userID)
		if err != nil || !ok {
			c.emit(errMsg("get_members", fmt.Errorf("access denied")))
			return
		}

		members, err := GetRoomMembers(p.RoomID)
		if err != nil {
			c.emit(errMsg("get_members", err))
			return
		}

		c.emit(OutgoingMessage{
			Action:  "members",
			Success: true,
			Data: map[string]interface{}{
				"room_id": p.RoomID,
				"members": members,
			},
		})

	// Отметить комнату как прочитанную
	case "mark_read":
		var p PayloadMarkRead
		if err := json.Unmarshal(inc.Payload, &p); err != nil {
			c.emit(errMsg("mark_read", fmt.Errorf("invalid payload")))
			return
		}
		MarkRoomMessagesAsRead(p.RoomID, c.userID)

		hub.broadcastToRoom(p.RoomID, c.userID, OutgoingMessage{
			Action:  "messages_read",
			Success: true,
			Data: map[string]interface{}{
				"room_id": p.RoomID,
				"user_id": c.userID,
			},
		})

		c.emit(OutgoingMessage{Action: "mark_read_ok", Success: true})

	// Пинг от клиента
	case "ping":
		c.emit(OutgoingMessage{Action: "pong", Success: true})

	default:
		c.emit(OutgoingMessage{
			Action:  "error",
			Success: false,
			Error:   fmt.Sprintf("unknown action: %s", inc.Action),
		})
	}
}

// ─── Вспомогательные функции ──────────────────────────────────────────────────

func errMsg(action string, err error) OutgoingMessage {
	return OutgoingMessage{
		Action:  action + "_error",
		Success: false,
		Error:   err.Error(),
	}
}

// Комнаты с количеством непрочитанных сообщений и последним сообщением
type RoomWithMeta struct {
	Room
	UnreadCount int      `json:"unread_count"`
	LastMessage *Message `json:"last_message,omitempty"`
}

func getUserRoomsWithMeta(userID int) ([]RoomWithMeta, error) {
	rooms, err := GetUserRooms(userID)
	if err != nil {
		return nil, err
	}

	result := make([]RoomWithMeta, 0, len(rooms))
	for _, r := range rooms {
		meta := RoomWithMeta{Room: r}

		unread, _ := GetUnreadCount(r.ID, userID)
		meta.UnreadCount = unread

		msgs, err := GetRoomMessagesWithReadStatus(r.ID, userID, 1, 0)
		if err == nil && len(msgs) > 0 {
			m := msgs[0]
			meta.LastMessage = &m
		}

		result = append(result, meta)
	}
	return result, nil
}