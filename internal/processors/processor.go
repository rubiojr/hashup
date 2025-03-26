package processors

import "os"

type Processor interface {
	Process(path string, info os.FileInfo, hostname string) error
}
