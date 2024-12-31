package service

import (
	"log"
	"time"
)

const (
	PublicGameRequest string = "public_game_request"
	PublicGameTimeout string = "public_game_timeout"
	PublicGameMatched string = "game_start"
	PublicQueueLeave  string = "public_queue_leave"
)

func (m *OnlineUsersManager) handlePublicGameRequest(username string, userID string, conn *SafeConn) {
	// Vérifier si le joueur est déjà dans une partie
	if user, err := m.userStore.GetUser(username); err == nil && user.IsInRoom {
		conn.WriteJSON(WebSocketMessage{
			Type: "error",
			Content: string(mustJson(map[string]string{
				"message": "Vous êtes déjà dans une partie",
			})),
		})
		return
	}

	m.publicQueue.mutex.Lock()
	defer m.publicQueue.mutex.Unlock()

	// Vérifier si le joueur est déjà dans la file d'attente
	if _, exists := m.publicQueue.waitingPlayers[username]; exists {
		return
	}

	// Chercher l'adversaire qui attend depuis le plus longtemps
	var opponent *QueuedPlayer
	var longestWait time.Duration
	for _, player := range m.publicQueue.waitingPlayers {
		waitTime := time.Since(player.JoinedAt)
		if opponent == nil || waitTime > longestWait {
			opponent = player
			longestWait = waitTime
		}
	}

	if opponent == nil {
		// Aucun adversaire disponible, ajouter le joueur à la file d'attente
		timer := time.NewTimer(60 * time.Second)
		queuedPlayer := &QueuedPlayer{
			UserID:     userID,
			Username:   username,
			JoinedAt:   time.Now(),
			Connection: conn,
			Timer:      timer,
		}

		m.publicQueue.waitingPlayers[username] = queuedPlayer

		// Gérer le timeout
		go func() {
			<-timer.C
			m.handlePublicGameTimeout(username)
		}()

	} else {
		// Adversaire trouvé, créer la partie
		delete(m.publicQueue.waitingPlayers, opponent.Username)
		opponent.Timer.Stop()

		// Créer une invitation pour la partie
		invitation := InvitationMessage{
			Type:         InvitationAccept,
			FromUserID:   opponent.UserID,
			FromUsername: opponent.Username,
			ToUserID:     userID,
			ToUsername:   username,
			RoomID:       GenerateUniqueID(),
		}

		// Créer la room et démarrer la partie
		room := m.roomManager.CreateRoom(invitation)

		// Mettre à jour le statut des joueurs
		m.userStore.UpdateUserRoomStatus(opponent.Username, true)
		m.userStore.UpdateUserRoomStatus(username, true)

		// Préparation des états de jeu spécifiques pour chaque joueur
		baseGameState := map[string]interface{}{
			"gameId":         room.RoomID,
			"gameCreatorUid": opponent.UserID, // Premier joueur = créateur
			"positonFen":     room.PositionFEN,
			"whitesTime":     room.WhitesTime,
			"blacksTime":     room.BlacksTime,
			"isWhitesTurn":   true,
			"isGameOver":     false,
			"moves":          []Move{},
			"winnerId":       "",
		}

		// État pour le premier joueur (opponent - créateur)
		player1GameState := copyAndAddUserInfo(baseGameState, opponent.UserID, username)

		// État pour le second joueur
		player2GameState := copyAndAddUserInfo(baseGameState, userID, opponent.Username)

		room.AddConnection(opponent.Username, opponent.Connection)
		room.AddConnection(username, conn)

		// Créer un timer pour le délai de 2 secondes
		go func() {
			// Attendre 2 secondes
			time.Sleep(2 * time.Second)

			// Envoyer le message de début de partie aux deux joueurs
			if err := opponent.Connection.WriteJSON(WebSocketMessage{
				Type:    PublicGameMatched,
				Content: string(mustJson(player1GameState)),
			}); err != nil {
				log.Printf("Error sending game start message to creator: %v", err)
				return
			}

			if err := conn.WriteJSON(WebSocketMessage{
				Type:    PublicGameMatched,
				Content: string(mustJson(player2GameState)),
			}); err != nil {
				log.Printf("Error sending game start message to joiner: %v", err)
				return
			}

			// Mettre à jour la liste des utilisateurs en ligne
			m.broadcastOnlineUsers()
		}()
	}
}

// Fonction pour gérer le départ de la file d'attente
func (m *OnlineUsersManager) handlePublicQueueLeave(username string) {
	m.publicQueue.mutex.Lock()
	defer m.publicQueue.mutex.Unlock()

	// Vérifier si le joueur est dans la file d'attente
	player, exists := m.publicQueue.waitingPlayers[username]
	if !exists {
		return
	}

	// Arrêter le timer
	if player.Timer != nil {
		player.Timer.Stop()
	}

	// Supprimer le joueur de la file d'attente
	delete(m.publicQueue.waitingPlayers, username)

	// Notifier le joueur qu'il a quitté la file d'attente
	if player.Connection != nil {
		player.Connection.WriteJSON(WebSocketMessage{
			Type: PublicQueueLeave,
			Content: string(mustJson(map[string]string{
				"message": "Vous avez quitté le mode public.",
			})),
		})
	}
}

// Gérer le timeout d'une requête de partie publique
func (m *OnlineUsersManager) handlePublicGameTimeout(username string) {
	m.publicQueue.mutex.Lock()
	defer m.publicQueue.mutex.Unlock()

	player, exists := m.publicQueue.waitingPlayers[username]
	if !exists {
		return
	}

	// Supprimer le joueur de la file d'attente
	delete(m.publicQueue.waitingPlayers, username)

	// Notifier le joueur du timeout
	player.Connection.WriteJSON(WebSocketMessage{
		Type: PublicGameTimeout,
		Content: string(mustJson(map[string]string{
			"message": "Aucun adversaire trouvé. Veuillez réessayer.",
		})),
	})
}
