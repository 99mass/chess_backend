package main

import (
	service "chess_backend/service"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func main() {
	router := mux.NewRouter()
	userStore := service.SetupUserStore()
	onlineUsersManager := service.NewOnlineUsersManager(userStore)

	router.HandleFunc("/users/create", service.CreateUserHandler(userStore)).Methods("POST")
	router.HandleFunc("/users/get", service.GetUserHandler(userStore)).Methods("GET")
	router.HandleFunc("/users/disconnect", service.DisconnectUserHandler(userStore, onlineUsersManager)).Methods("DELETE")

	// Routes WebSocket
	router.HandleFunc("/ws", onlineUsersManager.HandleConnection)

	port := service.Getenv("PORT", "8081")
	log.Printf("Running user management server on port :%s...", port)

	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatal(err)
	}
}
