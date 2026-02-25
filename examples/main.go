package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/edgarsilva/simpleenv"
	"github.com/joho/godotenv"
)

type Env struct {
	Environment     string `env:"ENVIRONMENT;oneof=development,test,staging,production"`
	EncryptionKey   string `env:"ENCRYPTION_KEY"`
	APITokenSystems string `env:"FETCH_SYSTEM_TOKEN"`
	APITestNoTag    string
	PubsubHostURL   string  `env:"PUBSUB_EMULATOR_HOST;regex='(http|https)://(localhost|127.0.0.1):[0-9]+'"`
	PubsubProjectID string  `env:"PUBSUB_PROJECT_ID"`
	Version         float64 `env:"VERSION;optional"`
	Concurrency     int     `env:"CONCURRENCY;min=1"`
}

func main() {
	_ = godotenv.Load()
	env := NewEnv()

	output, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		log.Fatal("failed to format env output:", err)
	}

	fmt.Println(string(output))
}

func NewEnv() Env {
	e := Env{}
	err := simpleenv.Load(&e)
	if err != nil {
		log.Fatal("failed to load ENV variables:", err)
	}

	return e
}
