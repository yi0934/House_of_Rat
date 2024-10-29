<h1 align="center">House_of_Rat Server</h1>

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
$ git clone https://github.com/yi0934/House_of_Rat-Server
$ cd ./House_of_Rat-Server
$ go mod tidy
$ go mod download
$ go build main.go
$ ./main
输入命令: help
可用命令:
  http            启动HTTP服务
  websocket       启动WebSocket服务
  list websocket  列出所有活动的WebSocket连接
  use <UUID>     与特定的WebSocket连接交互（通过UUID）
  help           显示此帮助信息
# 开启websocket,等待client连接
输入命令: websocket 
WebSocket服务已启动在8081端口
新连接: {ip}:{port}, UUID: {session_uuid}
输入命令: use dbd664fb-774d-4a57-b5cb-d16202ce1636
输入要给 {ip}:{55680} 发送的消息 (输入 'back' 或 'bk' 返回): help
可用命令:
  list_files
  get_clipboard
  download_file
  upload_file
  execute_command
  list_processes
  help

