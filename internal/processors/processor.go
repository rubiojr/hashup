package processors

import "github.com/rubiojr/hashup/internal/types"

type Processor interface {
	Process(path string, msg types.ScannedFile) error
}
