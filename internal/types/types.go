package types

import "time"

// ScannedFile represents the structure of the message sent to NATS
type ScannedFile struct {
	Path      string    `msgpack:"path"`
	Size      int64     `msgpack:"size"`
	ModTime   time.Time `msgpack:"mod_time"`
	Hash      string    `msgpack:"hash"`
	Extension string    `msgpack:"extension"`
	Hostname  string    `msgpack:"hostname"`
}
