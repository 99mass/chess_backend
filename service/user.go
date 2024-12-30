package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func NewUserStore() *UserStore {
	return &UserStore{
		Users: make(map[string]UserProfile),
		mutex: sync.RWMutex{},
	}
}

func (us *UserStore) Load() error {
	filename := filepath.Join("users", "users.json")

	// Vérifier si le dossier existe
	if err := os.MkdirAll("users", 0755); err != nil {
		return fmt.Errorf("failed to create users directory: %v", err)
	}

	// Vérifier si le fichier existe
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		// Si le fichier n'existe pas, créer un nouveau fichier vide
		us.Users = make(map[string]UserProfile)
		return us.Save()
	}

	// Le fichier existe, essayons de le lire
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read users file: %v", err)
	}

	// Si le fichier est vide, initialiser avec une map vide
	if len(data) == 0 {
		us.Users = make(map[string]UserProfile)
		return us.Save()
	}

	// Essayer de décoder le JSON
	var tempStore struct {
		Users map[string]UserProfile `json:"users"`
	}

	if err := json.Unmarshal(data, &tempStore); err != nil {
		// Si le fichier est corrompu, créer une nouvelle structure
		log.Printf("Warning: corrupted users.json file, creating new one: %v", err)
		us.Users = make(map[string]UserProfile)
		return us.Save()
	}

	us.Users = tempStore.Users
	return nil
}

func (us *UserStore) Save() error {
	filename := filepath.Join("users", "users.json")

	// Créer la structure à sauvegarder
	tempStore := struct {
		Users map[string]UserProfile `json:"users"`
	}{
		Users: us.Users,
	}

	// Encoder en JSON avec indentation
	data, err := json.MarshalIndent(tempStore, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal users: %v", err)
	}

	// Écrire dans le fichier
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write users file: %v", err)
	}

	return nil
}

func (us *UserStore) CreateUser(user UserProfile) error {
	us.mutex.Lock()
	defer us.mutex.Unlock()

	if _, exists := us.Users[user.UserName]; exists {

		us.Users[user.UserName] = user
	} else {

		us.Users[user.UserName] = user
	}

	return us.Save()
}

func (us *UserStore) GetUser(username string) (*UserProfile, error) {
	us.mutex.RLock()
	defer us.mutex.RUnlock()

	user, exists := us.Users[username]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	return &user, nil
}

func (us *UserStore) UpdateUserOnlineStatus(username string, isOnline bool, isInRoom bool) error {
	us.mutex.Lock()
	defer us.mutex.Unlock()

	user, exists := us.Users[username]
	if !exists {
		return fmt.Errorf("user not found")
	}

	user.IsOnline = isOnline
	user.IsInRoom = isInRoom
	us.Users[username] = user

	return us.Save()
}

func CreateUserHandler(userStore *UserStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var userInput struct {
			UserName string `json:"username"`
		}

		if err := json.NewDecoder(r.Body).Decode(&userInput); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		userInput.UserName = strings.TrimSpace(userInput.UserName)
		if userInput.UserName == "" {
			http.Error(w, "Username and password required", http.StatusBadRequest)
			return
		}

		_, err := userStore.GetUser(userInput.UserName)
		if err == nil {

			http.Error(w, "User already has an active session", http.StatusConflict)
			return

		}

		// Créer nouvel utilisateur...

		newUser := UserProfile{
			ID:       GenerateUniqueID(),
			UserName: userInput.UserName,
			IsOnline: false,
			IsInRoom: false,
		}

		if err := userStore.CreateUser(newUser); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(newUser)
	}
}

func GetUserHandler(userStore *UserStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		user, err := userStore.GetUser(username)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Créer une version de la réponse sans le mot de passe
		response := struct {
			ID       string `json:"id"`
			UserName string `json:"username"`
			IsOnline bool   `json:"isOnline"`
			IsInRoom bool   `json:"isInRoom"`
		}{
			ID:       user.ID,
			UserName: user.UserName,
			IsOnline: user.IsOnline,
			IsInRoom: user.IsInRoom,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func (us *UserStore) DeleteUser(username string) error {
	us.mutex.Lock()
	defer us.mutex.Unlock()

	if _, exists := us.Users[username]; !exists {
		return fmt.Errorf("user not found")
	}

	delete(us.Users, username)

	return us.Save()
}

// Créer un nouveau handler pour la déconnexion
func DisconnectUserHandler(userStore *UserStore, onlineUsersManager *OnlineUsersManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		username := r.URL.Query().Get("username")
		if username == "" {
			http.Error(w, "Username is required", http.StatusBadRequest)
			return
		}

		// Vérifier si l'utilisateur existe
		user, err := userStore.GetUser(username)
		if err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		// Fermer la connexion WebSocket si elle existe
		onlineUsersManager.mutex.Lock()
		if conn, exists := onlineUsersManager.connections[username]; exists {
			conn.conn.Close()
			delete(onlineUsersManager.connections, username)
		}
		onlineUsersManager.mutex.Unlock()

		// Mettre à jour le statut en ligne et dans la room
		userStore.UpdateUserOnlineStatus(username, false, false)

		// Supprimer l'utilisateur
		if err := userStore.DeleteUser(username); err != nil {
			http.Error(w, fmt.Sprintf("Failed to delete user: %v", err), http.StatusInternalServerError)
			return
		}

		// Notifier les autres utilisateurs que cet utilisateur est déconnecté
		onlineUsersManager.broadcastOnlineUsers()

		// Renvoyer une réponse de succès
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": fmt.Sprintf("User %s successfully disconnected and deleted", user.UserName),
		})
	}
}

func SetupUserStore() *UserStore {
	userStore := NewUserStore()
	if err := userStore.Load(); err != nil {
		log.Printf("Warning: Error loading user store: %v", err)
	}
	return userStore
}
