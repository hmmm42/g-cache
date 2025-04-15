package cache

type Picker interface {
	Pick(key string) (Fetcher, bool)
}

type Fetcher interface {
	Fetch(group string, key string) ([]byte, error)
}

type Retriever interface {
	retrieve(key string) ([]byte, error)
}

type RetrieveFunc func(key string) ([]byte, error)

func (f RetrieveFunc) retrieve(key string) ([]byte, error) {
	return f(key)
}
