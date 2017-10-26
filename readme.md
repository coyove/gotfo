# TCP Fast Open in Go
TCP Fast Open on Windows 10 (since version 1607) and Linux (since 3.7), 64bit with go1.8/1.9 only.

# Usage
```go
// dial an address with data, it returns a net.Conn
conn, err := gotfo.Dial(address, data)

// listen, it returns a net.Listener
listener, err := gotfo.Listen(address)
```
