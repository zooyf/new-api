package hwdramaproxy

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type SecretStore struct {
	values map[string]string
}

func LoadSecretStore(path string) (*SecretStore, error) {
	values := map[string]string{}
	path = strings.TrimSpace(path)
	if path != "" {
		fileValues, err := godotenv.Read(path)
		if err != nil {
			return nil, err
		}
		for key, value := range fileValues {
			values[key] = value
		}
	}
	return &SecretStore{values: values}, nil
}

func (store *SecretStore) Lookup(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if store != nil {
		if value, ok := store.values[key]; ok {
			return strings.TrimSpace(value)
		}
	}
	return strings.TrimSpace(os.Getenv(key))
}
