package cache

type NoopCache struct{}

func NewNoopCache() *NoopCache {
	return &NoopCache{}
}

func (nc *NoopCache) Save() error {
	return nil
}

func (nc *NoopCache) MarkFileProcessed(filePath, fileHash string) {
}

func (nc *NoopCache) IsFileProcessed(filePath, fileHash string) bool {
	return false
}
