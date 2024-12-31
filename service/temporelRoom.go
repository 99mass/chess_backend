package service

import (
	"sync"
	"time"
)

type TempRoom struct {
	RoomID      string
	Timeout     *InvitationTimeout
	CreatedAt   time.Time
	WhitePlayer OnlineUser
	BlackPlayer OnlineUser
}

type TemporaryRoomManager struct {
	rooms map[string]*TempRoom
	mutex sync.RWMutex
}

func NewTemporaryRoomManager() *TemporaryRoomManager {
	return &TemporaryRoomManager{
		rooms: make(map[string]*TempRoom),
	}
}

func (trm *TemporaryRoomManager) CreateTempRoom(invitation InvitationMessage, timeout *InvitationTimeout) *TempRoom {
	trm.mutex.Lock()
	defer trm.mutex.Unlock()

	tempRoom := &TempRoom{
		RoomID:  invitation.RoomID,
		Timeout: timeout,
		WhitePlayer: OnlineUser{
			ID:       invitation.FromUserID,
			Username: invitation.FromUsername,
		},
		BlackPlayer: OnlineUser{
			ID:       invitation.ToUserID,
			Username: invitation.ToUsername,
		},
		CreatedAt: time.Now(),
	}

	trm.rooms[invitation.RoomID] = tempRoom
	return tempRoom
}

func (trm *TemporaryRoomManager) RemoveTempRoom(roomID string) {
	trm.mutex.Lock()
	defer trm.mutex.Unlock()

	if room, exists := trm.rooms[roomID]; exists {
		if room.Timeout != nil {
			room.Timeout.Stop()
		}

		delete(trm.rooms, roomID)
	}
}

func (trm *TemporaryRoomManager) GetTempRoom(roomID string) (*TempRoom, bool) {
	trm.mutex.RLock()
	defer trm.mutex.RUnlock()

	room, exists := trm.rooms[roomID]
	return room, exists
}
