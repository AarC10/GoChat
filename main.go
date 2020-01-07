package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"

	uuid "github.com/satori/go.uuid"
	"net/http"
)

// Keeps track of connected clients, register attempts, destroyed clients awaiting removal and messages sent to all
type ClientManager struct {
	clients     map[*Client]bool
	broadcast	chan []byte
	register	chan *Client
	unregister	chan *Client
}


// Clients have unique id, sockets and messages waiting to be sent
type Client struct {
	id	    string
	socket  *websocket.Conn
	send	chan []byte
}

// Messaging
type Message struct {
	Sender		string `json:"sender,omitempty"`
	Recipient	string `json:"recipient,omitempty"`
	Content		string `json:"content,omitempty"`
}


// Client Manager
var manager = ClientManager {
	broadcast: make(chan []byte),
	register: make(chan *Client),
	unregister: make(chan *Client),
	clients: make(map[*Client]bool),
}

// Server GoRoutines
func (manager *ClientManager) start() {
	for {
		select {
		case conn := <-manager.register: // Client added to map of available clients managed by client manager
			manager.clients[conn] = true
			jsonMessage, _ := json.Marshal(&Message{Content: "/New socket connected"})
			manager.send(jsonMessage, conn)
		case conn := <-manager.unregister: // Channel data in disconnected client will be closed and client will be removed from manager
			if _, ok := manager.clients[conn]; ok {
				close(conn.send)
				delete(manager.clients, conn)
				jsonMessage, _ := json.Marshal(&Message{Content: "/A socket disconnected"})
				manager.send(jsonMessage, conn)
			}
		case message := <-manager.broadcast: // If manager.broadcast has data, messages are trying to be sent and recieved
			for conn := range manager.clients	{
				select {
				case conn.send <- message:
					default:
						close(conn.send)
						delete(manager.clients, conn)

				}
			}
		}
	}
}
// Read socket data and add it to manager.broadcast
func (manager *ClientManager) send(message []byte, ignore *Client)	{
	for conn := range manager.clients	{
		if conn != ignore	{
			conn.send <- message
		}
	}
}

func (c *Client) read()	{
	defer func()	{
		manager.unregister <- c
		c.socket.Close()
	}()

	for {	// If there is was an error, client probably disconnected and needs to be unregistered
		_, message, err := c.socket.ReadMessage()
		if err != nil	{
			manager.unregister <- c
			c.socket.Close()
			break
		}
		jsonMessage, _ := json.Marshal(&Message{Sender: c.id, Content: string(message)})
		manager.broadcast <- jsonMessage
	}
}
// If c.send has data, message attempts to send. If not ok, disconnect message sent to client
func (c *Client) write() {
	defer func()	{
		c.socket.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok{
				c.socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.socket.WriteMessage(websocket.TextMessage, message)
		}
	}
}
// Eliminate CORS errors by accepting requests from outside domains using checkOrigin
func wsPage(res http.ResponseWriter, req *http.Request) {
	conn, error := (&websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}).Upgrade(res, req, nil)
	if error != nil {
		http.NotFound(res, req)
		return
	}
	client := &Client{id: uuid.Must(uuid.NewV4()).String(), socket: conn, send: make(chan []byte)}

	manager.register <- client

	go client.read()
	go client.write()
}


func main()	{
	fmt.Println("Starting")
	go manager.start()
	http.HandleFunc("/ws", wsPage)
	http.ListenAndServe(":12345", nil) // Start server on port 12345 only accessible through websockets

}