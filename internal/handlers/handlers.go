package handlers

import (
	"fmt"
	"log"
	"net/http"
	"sort"

	"github.com/CloudyKit/jet/v6"
	"github.com/gorilla/websocket"
)

// websocket channel
var wsChan = make(chan WsPayload)

var clients = make(map[WebSocketConnection]string)

var views = jet.NewSet(
	// loader
	jet.NewOSFileSystemLoader("./html"),
	// allows for live changes
	jet.InDevelopmentMode(),
)

// variables for websocket
var upgradeConnection = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func Home(w http.ResponseWriter, r *http.Request) {
	err := renderPage(w, "home.jet", nil)
	if err != nil {
		log.Println(err)
	}
}

// WebSocketConnection holds the connection to a websocket
type WebSocketConnection struct {
	*websocket.Conn
}

// WsJsonResponse defines the response sent back from websocket
type WsJsonResponse struct {
	Action         string   `json:"action"`
	Message        string   `json:"message"`
	MessageType    string   `json:"message_type"`
	ConnectedUsers []string `json:"connected_users"`
}

type WsPayload struct {
	// define what the server will do. eg join
	Action   string `json:"action"`
	Username string `json:"username"`
	Message  string `json:"message"`
	// dont show Conn '-'
	Conn WebSocketConnection `json:"-"`
}

// WsEndpoint upgrades standard connection to websocket connection
func WsEndpoint(w http.ResponseWriter, r *http.Request) {
	// upgrade connection
	ws, err := upgradeConnection.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("Client connected to endpoint")

	// response to send back to client
	var response WsJsonResponse

	response.Message = `<em><small>Connected to server</small></em>`

	conn := WebSocketConnection{Conn: ws}
	// add entry into map
	clients[conn] = ""

	// send the response back to client
	err = ws.WriteJSON(response)
	if err != nil {
		log.Println(err)
		return
	}

	// listen for websocket connection
	go ListenForWs(&conn)
}

func ListenForWs(conn *WebSocketConnection) {
	// start back up if connection dies
	defer func() {
		// recover if conn dies
		if r := recover(); r != nil {
			log.Println("Error", fmt.Sprintf("%v", r))
		}
	}()

	// create payload that will be used to send data betwen client and server
	var payload WsPayload

	// execute forever
	for {
		// everytime the backend gets a payload from js client
		err := conn.ReadJSON(&payload)
		if err != nil {
			// if no payload, do nothing
		} else {
			// if there is a payload, send it off to websocket channel
			payload.Conn = *conn
			wsChan <- payload
		}
	}
}

func ListenToWsChannel() {
	var response WsJsonResponse

	// everytime there is a payload from the channel,
	// store in 'e', populate some members in the response
	// variable then call broadcast to all.
	for {
		// read event from channel
		e := <-wsChan

		switch e.Action {
		case "username":
			// get list of all users and broadcast back to client.
			clients[e.Conn] = e.Username
			users := getUserList()
			response.Action = "list_users"
			response.ConnectedUsers = users
			broadcastToAll(response)

		case "left":
			// send back a response with the new list of users
			response.Action = "list_users"
			// delete the user conn of who left
			delete(clients, e.Conn)
			// get users
			users := getUserList()
			// add new list of users to response
			response.ConnectedUsers = users
			// send back new list of users
			broadcastToAll(response)

		case "broadcast":
			response.Action = "broadcast"
			response.Message = fmt.Sprintf("<strong>%s</strong>: %s", e.Username, e.Message)
			broadcastToAll(response)
		}

		// populate the response from the channel payload
		// response.Action = "Got here"
		// response.Message = fmt.Sprintf("Some message, and action was %s", e.Action)
		// broadcastToAll(response)
	}
}

func getUserList() []string {
	var userList []string
	for _, x := range clients {
		// only append user's with names
		if x != "" {
			userList = append(userList, x)
		}
	}
	// sort list
	sort.Strings(userList)
	return userList
}

// broad cast to all users
func broadcastToAll(response WsJsonResponse) {
	for client := range clients {
		err := client.WriteJSON(response)
		if err != nil {
			// someone likely left page
			log.Println("websocket error")
			_ = client.Close()
			// delete user from clients
			delete(clients, client)
		}
	}
}

func renderPage(w http.ResponseWriter, tmpl string, data jet.VarMap) error {
	view, err := views.GetTemplate(tmpl)
	if err != nil {
		log.Println(err)
		return err
	}

	err = view.Execute(w, data, nil)
	if err != nil {
		log.Println(err)
		return err
	}

	return nil

}
