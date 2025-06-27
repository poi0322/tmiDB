package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 실제 프로덕션 환경에서는 특정 도메인만 허용해야 합니다.
		return true
	},
}

// broadcast 채널은 NATS 등 외부 메시지 큐로부터 메시지를 받아 모든 클라이언트에게 전송합니다.
var broadcast = make(chan []byte)
var clients = make(map[*websocket.Conn]bool)

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error upgrading connection: %v", err)
		return
	}
	defer ws.Close()
	clients[ws] = true
	log.Println("client connected")

	for {
		// 클라이언트로부터 메시지를 받을 필요가 없는 경우, 이 루프는 PING/PONG 처리 등을 위해 유지될 수 있습니다.
		// 현재는 아무 작업도 하지 않습니다.
		_, _, err := ws.ReadMessage()
		if err != nil {
			log.Printf("error reading message: %v", err)
			delete(clients, ws)
			break
		}
	}
}

func handleMessages() {
	for {
		msg := <-broadcast
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Printf("error writing message: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}

func main() {
	port := os.Getenv("REALTIME_PORT")
	if port == "" {
		port = "8081"
	}

	http.HandleFunc("/ws", handleConnections)
	go handleMessages()

	// NATS 구독 및 broadcast 채널로 메시지 전송 로직 추가 예정

	log.Printf("Realtime server starting on port %s", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalf("Failed to start realtime server: %v", err)
	}
}
