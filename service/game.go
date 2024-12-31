package service

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type ChessGameRoom struct {
	RoomID      string     `json:"room_id"`
	WhitePlayer OnlineUser `json:"white_player"`
	BlackPlayer OnlineUser `json:"black_player"`
	CreatedAt   time.Time  `json:"created_at"`
	Connections map[string]*SafeConn `json:"-"`
	mutex       sync.RWMutex
	GameState   map[string]interface{} `json:"game_state,omitempty"`
	Status      RoomStatus             `json:"status"`


	GameCreatorUID string `json:"game_creator_uid"`
	PositionFEN    string `json:"position_fen"`
	WinnerID       string `json:"winner_id,omitempty"`
	WhitesTime     string `json:"whites_time"`
	BlacksTime     string `json:"blacks_time"`
	IsWhitesTurn   bool   `json:"is_whites_turn"`
	IsGameOver     bool   `json:"is_game_over"`
	Moves          []Move `json:"moves"`
	Timer          *ChessTimer
	InvitationTimeout *InvitationTimeout
}

type Move struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Piece string `json:"piece"`
}

type RoomStatus string

const (
	RoomStatusPending  RoomStatus = "pending"
	RoomStatusInGame   RoomStatus = "in_game"
	RoomStatusFinished RoomStatus = "finished"
)

type RoomManager struct {
	rooms map[string]*ChessGameRoom
	mutex sync.RWMutex
}

const (
	InvitationCancel InvitationMessageType = "invitation_cancel"
	RoomLeave        InvitationMessageType = "room_leave"
)

func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[string]*ChessGameRoom),
	}
}

func (rm *RoomManager) CreateRoom(invitation InvitationMessage) *ChessGameRoom {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	room := &ChessGameRoom{
		RoomID: invitation.RoomID,
		WhitePlayer: OnlineUser{
			ID:       invitation.FromUserID,
			Username: invitation.FromUsername,
		},
		BlackPlayer: OnlineUser{
			ID:       invitation.ToUserID,
			Username: invitation.ToUsername,
		},
		CreatedAt: time.Now(),
		Connections: make(map[string]*SafeConn),
		Status:      RoomStatusPending,
		GameState:   make(map[string]interface{}),

		// Initialize new fields
		GameCreatorUID: invitation.FromUserID,
		PositionFEN:    "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
		WhitesTime:     "",
		BlacksTime:     "",
		IsWhitesTurn:   true,
		IsGameOver:     false,
		Moves:          []Move{},
	}

	timer := NewChessTimer(room, 10)
	room.Timer = timer
	timer.Start()

	rm.rooms[invitation.RoomID] = room
	return room
}


func (rm *RoomManager) GetRoom(roomID string) (*ChessGameRoom, bool) {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	room, exists := rm.rooms[roomID]
	return room, exists
}


func (rm *RoomManager) RemoveRoom(roomID string) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	
	if room, exists := rm.rooms[roomID]; exists {
		
		if room.Timer != nil {
			room.Timer.Stop()
		}
		delete(rm.rooms, roomID)
	}
}

func (room *ChessGameRoom) AddConnection(username string, conn *SafeConn) {
	room.mutex.Lock()
	defer room.mutex.Unlock()
	room.Connections[username] = conn
}

func (room *ChessGameRoom) GetOtherPlayer(username string) (string, bool) {
	if room.WhitePlayer.Username == username {
		return room.BlackPlayer.Username, true
	} else if room.BlackPlayer.Username == username {
		return room.WhitePlayer.Username, true
	}
	return "", false // Aucun autre joueur trouvé
}

func (rm *RoomManager) GetActiveRooms() []*ChessGameRoom {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	activeRooms := make([]*ChessGameRoom, 0, len(rm.rooms))
	for _, room := range rm.rooms {
		// Vous pouvez ajouter des conditions supplémentaires si nécessaire
		if room.Status == RoomStatusInGame || room.Status == RoomStatusPending {
			activeRooms = append(activeRooms, room)
		}
	}

	return activeRooms
}

// Remove a connection from a room
func (room *ChessGameRoom) RemoveConnection(username string) {
	room.mutex.Lock()
	defer room.mutex.Unlock()

	delete(room.Connections, username)
}

func (m *OnlineUsersManager) RemoveUserFromRoom(username string) ([]OnlineUser, error) {
	// Find the room the user is in
	var roomToRemove *ChessGameRoom
	m.roomManager.mutex.RLock()
	for _, room := range m.roomManager.rooms {
		if room.WhitePlayer.Username == username || room.BlackPlayer.Username == username {
			roomToRemove = room
			break
		}
	}
	m.roomManager.mutex.RUnlock()

	// If no room found, return an error
	if roomToRemove == nil {
		return nil, fmt.Errorf("user %s not in any room", username)
	}

	// Find the other player
	otherUsername, found := roomToRemove.GetOtherPlayer(username)
	if !found {
		return nil, fmt.Errorf("could not find other player in room")
	}

	// Remove the room
	m.roomManager.RemoveRoom(roomToRemove.RoomID)

	// Update user statuses
	m.userStore.UpdateUserRoomStatus(username, false)
	if otherUsername != "" {
		m.userStore.UpdateUserRoomStatus(otherUsername, false)
	}

	// Broadcast and return online users
	return m.getCurrentOnlineUsers(), nil
}

func (room *ChessGameRoom) BroadcastMessage(message WebSocketMessage) {
    room.mutex.RLock()
    connections := make(map[string]*SafeConn)
    for username, conn := range room.Connections {
        connections[username] = conn
    }
    room.mutex.RUnlock()

    for username, conn := range connections {
        err := conn.WriteJSON(message)
        if err != nil {
            log.Printf("Error broadcasting message to %s: %v", username, err)
            // Si la connexion est fermée, supprimer l'utilisateur de la room
            if err.Error() == "write: broken pipe" || 
               err.Error() == "use of closed network connection" {
                room.mutex.Lock()
                delete(room.Connections, username)
                room.mutex.Unlock()
            }
        }
    }
}