package help

import (
	"fmt"
        "github.com/fatih/color"
)


var availableCommands = []string{
        "list_files", "get_clipboard", "download_file", "upload_file", "execute_command", "list_processes", "help",
}

func PrintUseCommandHelp() {
        color.Set(color.FgCyan)
        fmt.Println("Available commands:")
        for _, cmd := range availableCommands {
                fmt.Printf("  %s\n", cmd)
        }
        color.Unset()
}

func PrintHelp() {
        color.Set(color.FgCyan)
        fmt.Println("Available commands:")
        fmt.Println("  http            Start HTTP server")
        fmt.Println("  websocket       Start WebSocket server")
        fmt.Println("  list websocket  List all active WebSocket connections")
        fmt.Println("  use <UUID>      Interact with a specific WebSocket connection (by UUID)")
        fmt.Println("  generate        Generate a client template with options: lang=<go|python> ip=<IP_ADDRESS> port=<PORT> protocol=<ws|wss>")
        fmt.Println("  help            Show this help information")
        color.Unset()
}
