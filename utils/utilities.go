package utils

import (
        "net"
        "strconv"
)


func IsValidIP(ip string) bool {
        return net.ParseIP(ip) != nil
}

func IsValidPort(port string) bool {
        p, err := strconv.Atoi(port)
        return err == nil && p > 0 && p < 65536
}
