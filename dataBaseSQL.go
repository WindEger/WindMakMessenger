package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

var MinPasswordLength = 6

var (
	MinLenRoomName = 1
	MaxLenRoomName = 100
)

type User struct {
	ID             int
	Login          string
	Nickname       string
	LastTimeOnline time.Time
	RegisterDate   time.Time
}

type Room struct {
	ID              int
	RoomName        string
	CreatedBy       int
	CreatedDateTime time.Time
	UpdatedAt       time.Time
}

type RoomWithMeta struct {
	Room
	UnreadCount int
	LastMessage *Message
}

// `json:"-"`  // Никогда не будет в JSON
type Message struct {
	ID              int       `json:"id"`
	RoomID          int       `json:"room_id"`
	UserID          int       `json:"user_id"`
	Content         string    `json:"content"`
	CreatedDateTime time.Time `json:"created_at"`
	UserNickname    string    `json:"nickname"`
	IsRead          bool      `json:"is_read"`
}

var db *sql.DB

func init() {
	var err error
	dbDir := os.Getenv("DB_DIR")
	//dbDir := "./data"
	if dbDir == "" {
		dbDir = "./data"
		log.Println("DB_DIR not set, using default.")
	}
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatal("Failed to create database directory:", err)
	}
	dbPath := filepath.Join(dbDir, "messenger.db")

	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	// Пользователи
	createUsersTableSQL := `CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		login TEXT UNIQUE NOT NULL,
		nickname TEXT,
		password TEXT NOT NULL,
		registerdate DATETIME DEFAULT CURRENT_TIMESTAMP,
		lastTimeOnline DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	// Комнаты
	createRoomsTableSQL := `CREATE TABLE IF NOT EXISTS rooms (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		roomName TEXT DEFAULT 'Room',
		createdBy INTEGER NOT NULL,
		createdDateTime DATETIME DEFAULT CURRENT_TIMESTAMP,
		updatedAt DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (createdBy) REFERENCES users(id) ON DELETE CASCADE
	);`

	// Участники комнат
	createRoomMembersTableSQL := `CREATE TABLE IF NOT EXISTS room_members (
		roomID INTEGER NOT NULL,
		userID INTEGER NOT NULL,
		joinedAt DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (roomID, userID),
		FOREIGN KEY (roomID) REFERENCES rooms(id) ON DELETE CASCADE,
		FOREIGN KEY (userID) REFERENCES users(id) ON DELETE CASCADE
	);`

	// Сообщения
	createMessagesTableSQL := `CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		roomID INTEGER NOT NULL,
		userID INTEGER NOT NULL,
		content TEXT NOT NULL,
		createdDateTime DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (roomID) REFERENCES rooms(id) ON DELETE CASCADE,
		FOREIGN KEY (userID) REFERENCES users(id) ON DELETE CASCADE
	);`

	createMessageReadsTableSQL := `CREATE TABLE IF NOT EXISTS message_reads (
    messageID INTEGER NOT NULL,
    userID INTEGER NOT NULL,
    read_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (messageID, userID),
    FOREIGN KEY (messageID) REFERENCES messages(id) ON DELETE CASCADE,
    FOREIGN KEY (userID) REFERENCES users(id) ON DELETE CASCADE
	);`

	// Индексы для ускорения запросов
	createIndexesSQL := `
		CREATE INDEX IF NOT EXISTS idx_messages_roomID ON messages(roomID);
		CREATE INDEX IF NOT EXISTS idx_messages_createdDateTime ON messages(createdDateTime);
		CREATE INDEX IF NOT EXISTS idx_room_members_userID ON room_members(userID);
		CREATE INDEX IF NOT EXISTS idx_message_reads_user ON message_reads(userID);
	`

	updateLastTimeOnlineOnMessageTrigger := `
	CREATE TRIGGER IF NOT EXISTS update_user_last_online_on_message
	AFTER INSERT ON messages
	BEGIN
		UPDATE users 
		SET lastTimeOnline = CURRENT_TIMESTAMP 
		WHERE id = NEW.userID;
	END;`

	updateLastTimeOnlineOnJoinTrigger := `
	CREATE TRIGGER IF NOT EXISTS update_user_last_online_on_join
	AFTER INSERT ON room_members
	BEGIN
		UPDATE users 
		SET lastTimeOnline = CURRENT_TIMESTAMP 
		WHERE id = NEW.userID;
	END;`

	updateLastTimeOnlineOnLeaveTrigger := `
	CREATE TRIGGER IF NOT EXISTS update_user_last_online_on_leave
	AFTER DELETE ON room_members
	BEGIN
		UPDATE users 
		SET lastTimeOnline = CURRENT_TIMESTAMP 
		WHERE id = OLD.userID;
	END;`

	updateRoomActivityTrigger := `
    CREATE TRIGGER IF NOT EXISTS update_room_activity
    AFTER INSERT ON messages
    BEGIN
        UPDATE rooms SET updatedAt = CURRENT_TIMESTAMP WHERE id = NEW.roomID;
    END;`

	tables := []string{
		createUsersTableSQL,
		createRoomsTableSQL,
		createRoomMembersTableSQL,
		createMessagesTableSQL,
		createMessageReadsTableSQL,
		createIndexesSQL,
	}

	triggers := []string{updateRoomActivityTrigger,
		updateLastTimeOnlineOnMessageTrigger,
		updateLastTimeOnlineOnJoinTrigger,
		updateLastTimeOnlineOnLeaveTrigger}

	for _, tableSQL := range tables {
		if _, err = db.Exec(tableSQL); err != nil {
			log.Fatal(err)
		}
	}
	for _, triggersSQL := range triggers {
		if _, err = db.Exec(triggersSQL); err != nil {
			log.Fatal(err)
		}
	}

	log.Println("Database initialized successfully at:", dbPath)
}

// Users
func CreateUser(login, nickname, password string) (int64, error) {
	if login == "" {
		return 0, fmt.Errorf("login cannot be empty")
	}
	if len(password) < MinPasswordLength {
		return 0, fmt.Errorf("password must be at least 6 characters")
	}
	if nickname == "" {
		nickname = login
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("failed to hash password: %w", err)
	}

	query := `INSERT INTO users (login, nickname, password) VALUES (?, ?, ?)`
	result, err := db.Exec(query, login, nickname, hashedPassword)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("user with already exists")
		}
		return 0, fmt.Errorf("failed to create user: %w", err)
	}
	return result.LastInsertId()
}

func GetUserPasswordHash(login string) (string, error) {
	var passwordHash string
	query := `SELECT password FROM users WHERE login = ?`
	err := db.QueryRow(query, login).Scan(&passwordHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("user not found")
		}
		return "", err
	}
	return passwordHash, nil
}

func GetUserByID(userID int) (*User, error) {
	query := `SELECT id, login, nickname, registerdate, lastTimeOnline FROM users WHERE id = ?`
	var user User
	err := db.QueryRow(query, userID).Scan(&user.ID, &user.Login, &user.Nickname, &user.RegisterDate, &user.LastTimeOnline)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByLogin(login string) (*User, error) {
	var user User
	query := `SELECT id, login, nickname, registerdate, lastTimeOnline FROM users WHERE login = ?`
	err := db.QueryRow(query, login).Scan(&user.ID, &user.Login, &user.Nickname, &user.RegisterDate, &user.LastTimeOnline)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func UpdateUserNickName(userID int, nickname string) error {
	query := `UPDATE users SET nickname = ? WHERE id = ?`
	_, err := db.Exec(query, nickname, userID)
	return err
}

func UpdateUserPassword(userID int, newPassword string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	query := `UPDATE users SET password = ? WHERE id = ?`
	_, err = db.Exec(query, hashedPassword, userID)
	return err
}

func DeleteUserAndAllLink(userID int) error {
	query := `DELETE FROM users WHERE id = ?`
	_, err := db.Exec(query, userID)
	return err
}

func DeleteUserWithoutLink(userID int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE messages SET userID = NULL WHERE userID = ?`, userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM room_members WHERE userID = ?`, userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Rooms
func CreateRoom(roomName string, createdBy int) (int64, error) {
	if len(roomName) > MaxLenRoomName {
		return 0, fmt.Errorf("room name too long (max 100 characters)")
	}
	if len(roomName) < MinLenRoomName {
		roomName = "New Room"
	}

	if strings.ContainsAny(roomName, "\\/<>\"'|?*") {
		return 0, fmt.Errorf("room name contains invalid characters")
	}

	query := `INSERT INTO rooms (roomName, createdBy) VALUES (?, ?)`
	result, err := db.Exec(query, roomName, createdBy)
	if err != nil {
		return 0, fmt.Errorf("failed to create room: %w", err)
	}

	roomID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get room ID: %w", err)
	}

	err = AddMemberToRoom(int(roomID), createdBy)
	if err != nil {
		return 0, fmt.Errorf("failed to add creator to room: %w", err)
	}

	return roomID, nil
}

func AddMemberToRoom(roomID, userID int) error {
	query := `INSERT INTO room_members (roomID, userID) VALUES (?, ?)`
	_, err := db.Exec(query, roomID, userID)
	return err
}

func GetRoomByID(roomID int) (*Room, error) {
	query := `SELECT id, roomName, createdBy, createdDateTime, updatedAt FROM rooms WHERE id = ?`
	var room Room
	err := db.QueryRow(query, roomID).Scan(&room.ID, &room.RoomName, &room.CreatedBy, &room.CreatedDateTime, &room.UpdatedAt)
	return &room, err
}

func GetUserRooms(userID int) ([]Room, error) {
	query := `
        SELECT r.id, r.roomName, r.createdBy, r.createdDateTime, r.updatedAt 
        FROM rooms r
        JOIN room_members rm ON r.id = rm.roomID
        WHERE rm.userID = ?
        ORDER BY r.updatedAt DESC
    `
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []Room
	for rows.Next() {
		var room Room
		err := rows.Scan(&room.ID, &room.RoomName, &room.CreatedBy, &room.CreatedDateTime, &room.UpdatedAt)
		if err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}
	return rooms, nil
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

		msgs, err := GetRoomMessagesWithStatusOfRead(r.ID, userID, 1, 0)
		if err == nil && len(msgs) > 0 {
			m := msgs[0]
			meta.LastMessage = &m
		}

		result = append(result, meta)
	}
	return result, nil
}

func UpdateRoomName(roomID int, newName string) error {
	query := `UPDATE rooms SET roomName = ? WHERE id = ?`
	_, err := db.Exec(query, newName, roomID)
	return err
}

func DeleteRoomAndAllLink(roomID int) error {
	query := `DELETE FROM rooms WHERE id = ?`
	_, err := db.Exec(query, roomID)
	return err
}

func LeaveRoom(roomID, userID int) error {
	isMember, err := IsUserInRoom(roomID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return fmt.Errorf("user %d is not a member of room %d", userID, roomID)
	}

	err = RemoveMemberFromRoom(roomID, userID)
	if err != nil {
		return err
	}

	//memberCount, err := GetRoomMemberCount(roomID)
	hasAnyMembers, err := HasAnyMembers(roomID)
	if err != nil {
		return err
	}

	if !hasAnyMembers { //memberCount == 0 {
		err = DeleteRoomAndAllLink(roomID)
		if err != nil {
			return err
		}
		log.Printf("Room %d was deleted because no members left", roomID)
	} else {
		log.Printf("User %d left room %d", userID, roomID)
	}
	return nil
}

func RemoveMemberFromRoom(roomID, userID int) error {
	query := `DELETE FROM room_members WHERE roomID = ? AND userID = ?`
	_, err := db.Exec(query, roomID, userID)
	return err
}

func GetRoomMembers(roomID int) ([]User, error) {
	const limit = 1000
	const offset = 0

	query := `
		SELECT u.id, u.login, u.nickname, u.registerdate, u.lastTimeOnline
		FROM users u
		JOIN room_members rm ON u.id = rm.userID
		WHERE rm.roomID = ?
		ORDER BY u.nickname ASC
		LIMIT ? OFFSET ?
	`
	rows, err := db.Query(query, roomID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		err := rows.Scan(&user.ID, &user.Login, &user.Nickname,
			&user.RegisterDate, &user.LastTimeOnline)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

//func IsUserInRoom(roomID, userID int) (bool, error) {
//	var count int
//	query := `SELECT COUNT(*) FROM room_members WHERE roomID = ? AND userID = ?`
//	err := db.QueryRow(query, roomID, userID).Scan(&count)
//	return count > 0, err
//}

func IsUserInRoom(roomID, userID int) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM room_members WHERE roomID = ? AND userID = ?)`

	err := db.QueryRow(query, roomID, userID).Scan(&exists)
	return exists, err
}

func GetRoomMemberCount(roomID int) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM room_members WHERE roomID = ?`, roomID).Scan(&count)
	return count, err
}

func HasAnyMembers(roomID int) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM room_members WHERE roomID = ? LIMIT 1)`
	err := db.QueryRow(query, roomID).Scan(&exists)
	return exists, err
}

// Messages

const (
	MaxMessageLength = 5000
	MinMessageLength = 1
)

func CreateMessage(roomID, userID int, content string) (int64, error) {
	content = strings.TrimSpace(content)

	if len(content) < MinMessageLength {
		return 0, fmt.Errorf("message content cannot be empty")
	}
	if len(content) > MaxMessageLength {
		return 0, fmt.Errorf("message too long (max %d characters)", MaxMessageLength)
	}

	isMember, err := IsUserInRoom(roomID, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to check room membership: %w", err)
	}
	if !isMember {
		return 0, fmt.Errorf("user is not a member of this room")
	}

	query := `INSERT INTO messages (roomID, userID, content) VALUES (?, ?, ?)`
	result, err := db.Exec(query, roomID, userID, content)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func MarkMessageAsRead(messageID, userID int) error {
	var messageUserID int
	err := db.QueryRow(`SELECT userID FROM messages WHERE id = ?`, messageID).Scan(&messageUserID)
	if err != nil {
		return err
	}

	if messageUserID == userID {
		return nil
	}

	query := `
        INSERT OR IGNORE INTO message_reads (messageID, userID, read_at) 
        VALUES (?, ?, CURRENT_TIMESTAMP)
    `
	_, err = db.Exec(query, messageID, userID)
	return err
}

func MarkRoomMessagesAsRead(roomID, userID int) error {
	query := `
        INSERT OR IGNORE INTO message_reads (messageID, userID, read_at)
        SELECT id, ?, CURRENT_TIMESTAMP FROM messages 
        WHERE roomID = ? AND userID != ?
    `
	_, err := db.Exec(query, userID, roomID, userID)
	return err
}

func GetRoomMessagesWithStatusOfRead(roomID, currentUserID int, limit, offset int) ([]Message, error) {
	log.Printf("[DB] GetRoomMessagesWithStatusOfRead: roomID=%d, currentUserID=%d, limit=%d, offset=%d",
		roomID, currentUserID, limit, offset)

	err := MarkRoomMessagesAsRead(roomID, currentUserID)
	if err != nil {
		log.Printf("Warning: failed to mark messages as read: %v", err)
	}

	query := `
        SELECT 
            m.id,
            m.roomID,
            m.userID,
            m.content,
            m.createdDateTime,
            COALESCE(u.nickname, u.login) as nickname,
            CASE 
                WHEN m.userID = ? THEN 1  -- Свои сообщения всегда считаем прочитанными
                WHEN mr.userID IS NOT NULL THEN 1 
                ELSE 0 
            END as is_read
        FROM messages m
        JOIN users u ON m.userID = u.id
        LEFT JOIN message_reads mr ON m.id = mr.messageID AND mr.userID = ?
        WHERE m.roomID = ?
        ORDER BY m.createdDateTime DESC
        LIMIT ? OFFSET ?
    `

	rows, err := db.Query(query, currentUserID, currentUserID, roomID, limit, offset)
	if err != nil {
		log.Printf("[DB] Query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var nickname string
		var isRead int

		err := rows.Scan(
			&msg.ID,
			&msg.RoomID,
			&msg.UserID,
			&msg.Content,
			&msg.CreatedDateTime,
			&nickname,
			&isRead,
		)
		if err != nil {
			log.Printf("[DB] Scan error: %v", err)
			return nil, err
		}

		msg.UserNickname = nickname
		msg.IsRead = isRead == 1

		messages = append(messages, msg)
		log.Printf("[DB] Message: ID=%d, IsRead=%v", msg.ID, msg.IsRead)
	}

	log.Printf("[DB] Returning %d messages", len(messages))
	return messages, nil
}

func GetUnreadCount(roomID, userID int) (int, error) {
	query := `
        SELECT COUNT(*)
        FROM messages m
        LEFT JOIN message_reads mr ON m.id = mr.messageID AND mr.userID = ?
        WHERE m.roomID = ? AND m.userID != ? AND mr.userID IS NULL
    `
	var count int
	err := db.QueryRow(query, userID, roomID, userID).Scan(&count)
	return count, err
}

func GetTotalUnreadCount(userID int) (int, error) {
	query := `
        SELECT COUNT(*)
        FROM messages m
        JOIN room_members rm ON m.roomID = rm.roomID AND rm.userID = ?
        LEFT JOIN message_reads mr ON m.id = mr.messageID AND mr.userID = ?
        WHERE m.userID != ? AND mr.userID IS NULL
    `
	var count int
	err := db.QueryRow(query, userID, userID, userID).Scan(&count)
	return count, err
}

func GetMessageReaders(messageID int) ([]User, error) {
	query := `
        SELECT u.id, u.login, u.nickname, u.registerdate, u.lastTimeOnline
        FROM message_reads mr
        JOIN users u ON mr.userID = u.id
        WHERE mr.messageID = ?
        ORDER BY mr.read_at ASC
    `
	rows, err := db.Query(query, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		err := rows.Scan(&user.ID, &user.Login, &user.Nickname,
			&user.RegisterDate, &user.LastTimeOnline)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func GetMessageReadTime(messageID, userID int) (time.Time, error) {
	var readAt time.Time
	query := `SELECT read_at FROM message_reads WHERE messageID = ? AND userID = ?`
	err := db.QueryRow(query, messageID, userID).Scan(&readAt)
	return readAt, err
}

func IsMessageReadByAllMembers(messageID int) (bool, error) {
	query := `
        SELECT COUNT(DISTINCT rm.userID) = COUNT(DISTINCT mr.userID)
        FROM messages m
        JOIN room_members rm ON m.roomID = rm.roomID
        LEFT JOIN message_reads mr ON m.id = mr.messageID AND mr.userID = rm.userID
        WHERE m.id = ? AND rm.userID != m.userID
    `
	var isReadByAll bool
	err := db.QueryRow(query, messageID).Scan(&isReadByAll)
	return isReadByAll, err
}

func IsMessageReadByUser(messageID, userID int) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS(SELECT 1 FROM message_reads WHERE messageID = ? AND userID = ?)
	`
	err := db.QueryRow(query, messageID, userID).Scan(&exists)
	return exists, err
}

func GetMessageByID(messageID int) (*Message, error) {
	query := `SELECT id, roomID, userID, content, createdDateTime FROM messages WHERE id = ?`
	var msg Message
	err := db.QueryRow(query, messageID).Scan(&msg.ID, &msg.RoomID, &msg.UserID,
		&msg.Content, &msg.CreatedDateTime)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func DeleteMessage(messageID int) error {
	query := `DELETE FROM messages WHERE id = ?`
	_, err := db.Exec(query, messageID)
	return err
}
