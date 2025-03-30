package processors

import "github.com/rubiojr/hashup/internal/types"

type Processor interface {
	Process(path string, msg types.ScannedFile) error
}

type ChanProcessor struct {
	Ch chan types.ScannedFile
}

func NewChanProcessor() *ChanProcessor {
	return &ChanProcessor{
		Ch: make(chan types.ScannedFile),
	}
}

func (p *ChanProcessor) Process(path string, msg types.ScannedFile) error {
	p.Ch <- msg
	return nil
}
