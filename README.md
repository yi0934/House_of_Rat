<h1 align="center">House_of_Rat</h1>

**House_of_Rat** is a Remote Administration Tool (RAT) written in Go,just for RAT research

## Disclaimer

**THIS PROJECT, ITS SOURCE CODE, AND ITS RELEASES SHOULD ONLY BE USED FOR EDUCATIONAL PURPOSES.**
<br />
**ALL ILLEGAL USAGE IS PROHIBITED!**
<br />
**YOU SHALL USE THIS PROJECT AT YOUR OWN RISK.**
<br />
**THE AUTHORS AND DEVELOPERS ARE NOT RESPONSIBLE FOR ANY DAMAGE CAUSED BY YOUR MISUSE OF THIS PROJECT.**

**YOUR DATA IS PRICELESS. THINK TWICE BEFORE YOU CLICK ANY BUTTON OR ENTER ANY COMMAND.**

## Quick start

### tutorial

```bash
# Clone this repository.
$ git clone https://github.com/yi0934/House_of_Rat
$ cd ./House_of_Rat
$ go mod tidy
$ go mod download
$ go build main.go
$ ./main
Enter command: help
Available commands:
  http            Start HTTP server
  websocket       Start WebSocket server
  list websocket  List all active WebSocket connections
  use <UUID>      Interact with a specific WebSocket connection (by UUID)
  generate        Generate a client template with options: lang=<go|python> ip=<IP_ADDRESS> port=<PORT> protocol=<ws|wss>
  help            Show this help information
Enter command: generate lang=python ip=127.0.0.1  port=8081 protocol=ws
Client template generated: client_python.py
Enter command: websocket 
WebSocket server started on port 8081
# execute python3 client_python.py on client

New connection: 127.0.0.1:6606, UUID: 31f904ce-0e76-4659-8067-84ed644ecf0b
Enter command: use 31f904ce-0e76-4659-8067-84ed644ecf0b
Enter message for 31f904ce-0e76-4659-8067-84ed644ecf0b (or 'back' to return): help
Available:
  list_files
  get_clipboard
  download_file
  upload_file
  execute_command
  list_processes
  help
Enter message for 31f904ce-0e76-4659-8067-84ed644ecf0b (or 'back' to return):execute_command whoami

