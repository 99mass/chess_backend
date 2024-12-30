package service

import (
	"fmt"
	"sync"
	"time"
)

type ChessTimer struct {
	room         *ChessGameRoom
	ticker       *time.Ticker
	stopChan     chan struct{}
	mutex        sync.RWMutex
	isRunning    bool
	whiteSeconds int
	blackSeconds int
}

type TimerUpdate struct {
	RoomID       string `json:"roomId"`
	WhiteTime    int    `json:"whiteTime"`
	BlackTime    int    `json:"blackTime"`
	IsWhitesTurn bool   `json:"isWhitesTurn"`
}

func NewChessTimer(room *ChessGameRoom, initialTimeMinutes int) *ChessTimer {
	return &ChessTimer{
		room:         room,
		stopChan:     make(chan struct{}),
		whiteSeconds: initialTimeMinutes * 60,
		blackSeconds: initialTimeMinutes * 60,
	}
}

func (ct *ChessTimer) Start() {
	ct.mutex.Lock()
	if ct.isRunning {
		ct.mutex.Unlock()
		return
	}

	ct.isRunning = true
	ct.ticker = time.NewTicker(1 * time.Second)
	ct.mutex.Unlock()

	go ct.runTimer()
}

func (ct *ChessTimer) runTimer() {
	for {
		select {
		case <-ct.ticker.C:
			ct.mutex.Lock()
			timeoutOccurred := false
			var winner string

			// Vérifier le timeout avant de décrémenter
			if ct.room.IsWhitesTurn {
				if ct.whiteSeconds <= 0 {
					timeoutOccurred = true
					winner = "black"
				} else {
					ct.whiteSeconds--
					ct.room.WhitesTime = formatTime(ct.whiteSeconds)
				}
			} else {
				if ct.blackSeconds <= 0 {
					timeoutOccurred = true
					winner = "white"
				} else {
					ct.blackSeconds--
					ct.room.BlacksTime = formatTime(ct.blackSeconds)
				}
			}

			// Diffuser la mise à jour du temps
			ct.broadcastTimeUpdate()

			if timeoutOccurred {
				ct.mutex.Unlock() // Déverrouiller avant handleTimeOut
				ct.handleTimeOut(winner)
				return
			}

			ct.mutex.Unlock()

		case <-ct.stopChan:
			ct.ticker.Stop()
			return
		}
	}
}

func (ct *ChessTimer) handleTimeOut(winner string) {
	// Arrêter le timer
	ct.Stop()

	// Mettre à jour l'état du jeu
	ct.room.mutex.Lock()
	ct.room.IsGameOver = true
	ct.room.Status = RoomStatusFinished
	if winner == "white" {
		ct.room.WinnerID = ct.room.WhitePlayer.ID
	} else {
		ct.room.WinnerID = ct.room.BlackPlayer.ID
	}
	ct.room.mutex.Unlock()

	// S'assurer que le message de fin est envoyé avec une petite pause
	time.Sleep(100 * time.Millisecond)

	gameOver := map[string]interface{}{
		"gameId":     ct.room.RoomID,
		"winner":     winner,
		"reason":     "timeout",
		"whiteTime":  formatTime(ct.whiteSeconds),
		"blackTime":  formatTime(ct.blackSeconds),
		"winnerId":   ct.room.WinnerID,
		"isGameOver": true,
		"status":     string(RoomStatusFinished),
	}

	gameOverMsg := WebSocketMessage{
		Type:    "game_over",
		Content: string(mustJson(gameOver)),
	}

	// Vérifier que les connexions sont toujours actives avant d'envoyer
	ct.room.mutex.RLock()
	if len(ct.room.Connections) > 0 {
		ct.room.BroadcastMessage(gameOverMsg)
	}
	ct.room.mutex.RUnlock()
}
func (ct *ChessTimer) Stop() {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()

	if ct.isRunning {
		close(ct.stopChan)
		ct.ticker.Stop()
		ct.isRunning = false
	}
}

func (ct *ChessTimer) SwitchTurn() {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()

	ct.room.IsWhitesTurn = !ct.room.IsWhitesTurn
	ct.broadcastTimeUpdate()
}

func (ct *ChessTimer) broadcastTimeUpdate() {
	update := TimerUpdate{
		RoomID:       ct.room.RoomID,
		WhiteTime:    ct.whiteSeconds,
		BlackTime:    ct.blackSeconds,
		IsWhitesTurn: ct.room.IsWhitesTurn,
	}
	message := WebSocketMessage{
		Type:    "time_update",
		Content: string(mustJson(update)),
	}

	ct.room.BroadcastMessage(message)
}

// Fonction utilitaire pour formater le temps en string "MM:SS"
func formatTime(seconds int) string {
	minutes := seconds / 60
	remainingSeconds := seconds % 60
	return fmt.Sprintf("%02d:%02d", minutes, remainingSeconds)
}
