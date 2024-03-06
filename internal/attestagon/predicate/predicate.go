package predicate

import "time"

type Predicate struct {
	CreatedAt          time.Time
	Pod                Pod                        `json:"pod"`
	CommandsExecuted   map[string]CommandExecuted `json:"commandsExecuted"`
	ProcessesExecuted  map[string]int             `json:"processesExecuted"`
	FilesystemsMounted []FilesystemMounted        `json:"fileSystemsMounted"`
	TCPConnections     []TCPConnection            `json:"tcpConnections"`
	UIDSet             map[int]int                `json:"uidSet"`
	FilesWritten       map[string]int             `json:"filesWritten"`
	FilesRead          map[string]int             `json:"filesRead"`
	FilesOpened        map[string]int             `json:"filesOpened"`
}

type Pod struct {
	Name      string
	Namespace string
}

type CommandExecuted struct {
	Arguments map[string]int
}

type FilesystemMounted struct {
	Source      string
	Destination string
}

type TCPConnection struct {
	SocketAddress      string
	SocketPort         int
	DestinationAddress string
	DestinationPort    int
}
