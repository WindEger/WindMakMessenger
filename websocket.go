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

var (
	GetMessagesLimit = 200
)

// Upgrader

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Типы сообщений

type IncomingMessage struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

type OutgoingMessage struct {
	Action  string      `json:"action"`
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// Payloads
type PayloadRoomID struct {
	RoomID int `json:"room_id"`
}

// Hub

var hub = &Hub{
	clients: make(map[int][]*Client),
}

type Client struct {
	conn   *websocket.Conn
	userID int
	send   chan OutgoingMessage
	mu     sync.Mutex
}

type Hub struct {
	mu      sync.RWMutex
	clients map[int][]*Client
}

func (c *Client) emit(msg OutgoingMessage) {
	select {
	case c.send <- msg:
	default:
		log.Printf("[WS] send buffer full for user %d, dropping message", c.userID)
	}
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c.userID] = append(h.clients[c.userID], c)
	log.Printf("[WS] user %d connected (sessions: %d)", c.userID, len(h.clients[c.userID]))
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
	close(c.send)
	c.conn.Close()
	log.Printf("[WS] user %d disconnected", c.userID)
}

func (h *Hub) sendToUser(userID int, msg OutgoingMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients[userID] {
		c.emit(msg)
	}
}

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

// WS Handler
func WSHandler(w http.ResponseWriter, r *http.Request) {
	userID, err := checkTokenAndGetUserIDFromRequest(r)
	if err != nil {
		SendError(w, err.Error(), http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("[WS] upgrade error:", err)
		return
	}

	client := &Client{
		conn:   conn,
		userID: userID,
		send:   make(chan OutgoingMessage, 128),
	}

	hub.register(client)
	defer hub.unregister(client)

	// Горутина записи
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

	go sendInitialData(client)

	// Keepalive ping
	ticker := time.NewTicker(25 * time.Second)
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

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var incoming IncomingMessage
		if err := conn.ReadJSON(&incoming); err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[WS] unexpected close for user %d: %v", userID, err)
			}
			break
		}
		handleAction(client, incoming)
	}
}

// Начальные данные

func sendInitialData(c *Client) {
	c.getRoomsAction()
	//rooms, err := getUserRoomsWithMeta(c.userID)
	//if err != nil {
	//	c.emit(OutgoingMessage{Action: "error", Success: false, Error: "failed to load rooms"})
	//	return
	//}
	//c.emit(OutgoingMessage{
	//	Action:  "init",
	//	Success: true,
	//	Data:    map[string]interface{}{"rooms": rooms},
	//})
}

// Actions

func (c *Client) getRoomsAction() {
	rooms, err := getUserRoomsWithMeta(c.userID)
	if err != nil {
		c.emit(errMsg("get_rooms", err))
		return
	}
	c.emit(OutgoingMessage{Action: "rooms", Success: true, Data: rooms})
}

func (c *Client) GetMessagesAction(inc IncomingMessage) {
	type PayloadGetMessages struct {
		RoomID int `json:"room_id"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	var p PayloadGetMessages
	if err := json.Unmarshal(inc.Payload, &p); err != nil {
		c.emit(errMsg("get_messages", fmt.Errorf("invalid payload")))
		return
	}
	if p.Limit <= 0 || p.Limit > GetMessagesLimit {
		p.Limit = 50
	}

	inRoom, err := IsUserInRoom(p.RoomID, c.userID)
	if err != nil {
		c.emit(errMsg("get_messages", fmt.Errorf("db error: %w", err)))
		return
	}
	if !inRoom {
		c.emit(errMsg("get_messages", fmt.Errorf("access denied")))
		return
	}

	msgs, err := GetRoomMessagesWithStatusOfRead(p.RoomID, c.userID, p.Limit, p.Offset)
	if err != nil {
		c.emit(errMsg("get_messages", err))
		return
	}
	if msgs == nil {
		msgs = []Message{}
	}

	broadcast := OutgoingMessage{
		Action:  "messages_read",
		Success: true,
		Data:    map[string]interface{}{"room_id": p.RoomID, "user_id": c.userID},
	}
	hub.broadcastToRoom(p.RoomID, c.userID, broadcast)

	c.emit(OutgoingMessage{
		Action:  "messages",
		Success: true,
		Data: map[string]interface{}{
			"room_id":  p.RoomID,
			"messages": msgs,
			"limit":    p.Limit,
			"offset":   p.Offset,
		},
	})
}

func (c *Client) sendMessagesAction(inc IncomingMessage) {
	type PayloadSendMessage struct {
		RoomID  int    `json:"room_id"`
		Content string `json:"content"`
	}
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

	outMsg := map[string]interface{}{
		"id":         int(msgID),
		"room_id":    p.RoomID,
		"user_id":    c.userID,
		"content":    p.Content,
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"nickname":   nickname,
		"is_read":    false,
	}

	broadcast := OutgoingMessage{
		Action:  "new_message",
		Success: true,
		Data:    outMsg,
	}

	c.emit(broadcast)
	hub.broadcastToRoom(p.RoomID, c.userID, broadcast)
}

func (c *Client) CreateRoomAction(inc IncomingMessage) {
	type PayloadCreateRoom struct {
		RoomName string `json:"room_name"`
	}
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
	log.Printf("User %d create room: id: %d, roomName: %s", c.userID, roomID, p.RoomName)

	rooms, err := getUserRoomsWithMeta(c.userID)
	if err != nil {
		c.emit(errMsg("create_room", err))
		return
	}

	c.emit(OutgoingMessage{
		Action:  "room_created",
		Success: true,
		Data: map[string]interface{}{
			"new_room_id": int(roomID),
			"rooms":       rooms,
		},
	})
}

func (c *Client) LeaveRoomAction(inc IncomingMessage) {
	var p PayloadRoomID
	if err := json.Unmarshal(inc.Payload, &p); err != nil {
		c.emit(errMsg("leave_room", fmt.Errorf("invalid payload")))
		return
	}

	if err := LeaveRoom(p.RoomID, c.userID); err != nil {
		c.emit(errMsg("leave_room", err))
		return
	}

	c.emit(OutgoingMessage{
		Action:  "left_room",
		Success: true,
		Data:    map[string]interface{}{"room_id": p.RoomID},
	})

	hub.broadcastToRoom(p.RoomID, 0, OutgoingMessage{
		Action:  "member_left",
		Success: true,
		Data:    map[string]interface{}{"room_id": p.RoomID, "user_id": c.userID},
	})
}

func (c *Client) AddMemberAction(inc IncomingMessage) {
	type PayloadAddMember struct {
		RoomID int    `json:"room_id"`
		Login  string `json:"login"`
	}
	var p PayloadAddMember
	if err := json.Unmarshal(inc.Payload, &p); err != nil {
		c.emit(errMsg("add_member", fmt.Errorf("invalid payload")))
		return
	}

	inRoom, err := IsUserInRoom(p.RoomID, c.userID)
	if err != nil || !inRoom {
		c.emit(errMsg("add_member", fmt.Errorf("access denied")))
		return
	}

	target, err := GetUserByLogin(p.Login)
	if err != nil {
		c.emit(errMsg("add_member", fmt.Errorf("user not found")))
		return
	}

	alreadyIn, _ := IsUserInRoom(p.RoomID, target.ID)
	if alreadyIn {
		c.emit(errMsg("add_member", fmt.Errorf("user already in room")))
		return
	}

	if err := AddMemberToRoom(p.RoomID, target.ID); err != nil {
		c.emit(errMsg("add_member", err))
		return
	}

	room, _ := GetRoomByID(p.RoomID)

	hub.sendToUser(target.ID, OutgoingMessage{
		Action:  "added_to_room",
		Success: true,
		Data:    room,
	})

	hub.broadcastToRoom(p.RoomID, 0, OutgoingMessage{
		Action:  "member_joined",
		Success: true,
		Data:    map[string]interface{}{"room_id": p.RoomID, "user": target},
	})

	c.emit(OutgoingMessage{
		Action:  "member_added",
		Success: true,
		Data:    map[string]interface{}{"room_id": p.RoomID, "user": target},
	})
}

func (c *Client) RenameRoomAction(inc IncomingMessage) {
	type PayloadRenameRoom struct {
		RoomID   int    `json:"room_id"`
		RoomName string `json:"room_name"`
	}

	var p PayloadRenameRoom
	if err := json.Unmarshal(inc.Payload, &p); err != nil {
		c.emit(errMsg("rename_room", fmt.Errorf("invalid payload")))
		return
	}

	if p.RoomName == "" {
		c.emit(errMsg("rename_room", fmt.Errorf("name cannot be empty")))
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
		Data:    map[string]interface{}{"room_id": p.RoomID, "room_name": p.RoomName},
	})
}
func (c *Client) GetRoomMembersAction(inc IncomingMessage) {
	var p PayloadRoomID
	if err := json.Unmarshal(inc.Payload, &p); err != nil {
		c.emit(errMsg("get_members", fmt.Errorf("invalid payload")))
		return
	}

	inRoom, err := IsUserInRoom(p.RoomID, c.userID)
	if err != nil || !inRoom {
		c.emit(errMsg("get_members", fmt.Errorf("access denied")))
		return
	}

	members, err := GetRoomMembers(p.RoomID)
	if err != nil {
		c.emit(errMsg("get_members", err))
		return
	}
	if members == nil {
		members = []User{}
	}

	c.emit(OutgoingMessage{
		Action:  "members",
		Success: true,
		Data:    map[string]interface{}{"room_id": p.RoomID, "members": members},
	})
}

func (c *Client) MarkReadAction(inc IncomingMessage) {
	var p PayloadRoomID
	if err := json.Unmarshal(inc.Payload, &p); err != nil {
		c.emit(errMsg("mark_read", fmt.Errorf("invalid payload")))
		return
	}

	if err := MarkRoomMessagesAsRead(p.RoomID, c.userID); err != nil {
		log.Printf("[WS] mark_read error: %v", err)
	}

	hub.broadcastToRoom(p.RoomID, c.userID, OutgoingMessage{
		Action:  "messages_read",
		Success: true,
		Data:    map[string]interface{}{"room_id": p.RoomID, "user_id": c.userID},
	})

	c.emit(OutgoingMessage{Action: "mark_read_ok", Success: true})

}

// Диспетчер
func handleAction(c *Client, inc IncomingMessage) {
	switch inc.Action {
	// Получить чаты
	case "get_rooms":
		c.getRoomsAction()
	// Получить сообщения
	case "get_messages":
		c.GetMessagesAction(inc)
	// Отправить сообщение
	case "send_message":
		c.sendMessagesAction(inc)
	//Создать комнату
	case "create_room":
		c.CreateRoomAction(inc)
	// Покинуть комнату
	case "leave_room":
		c.LeaveRoomAction(inc)
	// Добавить участника по логину
	case "add_member":
		c.AddMemberAction(inc)
	// Переименовать комнату
	case "rename_room":
		c.RenameRoomAction(inc)
	// Получить участников комнаты
	case "get_members":
		c.GetRoomMembersAction(inc)
	// Отметить комнату как прочитанную
	case "mark_read":
		c.MarkReadAction(inc)
	// Ping
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

func errMsg(action string, err error) OutgoingMessage {
	log.Printf("[WS] action=%s error=%v", action, err)
	return OutgoingMessage{
		Action:  action + "_error",
		Success: false,
		Error:   err.Error(),
	}
}
