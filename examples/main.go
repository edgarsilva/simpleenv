package main

import (
	"fmt"
	"log"

	"github.com/edgarsilva/simpleenv"
	"github.com/joho/godotenv"
)

type Env struct {
	Environment          string  `env:"ENVIRONMENT;oneof=development,test,staging,production"`
	EncryptionKey        string  `env:"ENCRYPTION_KEY"`
	APITokenSystems      string  `env:"FETCH_SYSTEM_TOKEN"`
	APITokenIntegrations string  `env:"POST_SYSTEM_TOKEN"`
	PubsubHostURL        string  `env:"PUBSUB_EMULATOR_HOST;regex='(http|https)://(localhost|127.0.0.1):[0-9]+'"`
	PubsubProjectID      string  `env:"PUBSUB_PROJECT_ID"`
	Version              float64 `env:"VERSION;optional"`
	Concurrency          int     `env:"CONCURRENCY;min=1"`
}

func main() {
	godotenv.Load()
	env := NewEnv()
	fmt.Println(env)
}

func NewEnv() *Env {
	e := Env{}
	err := simpleenv.Load(&e)
	if err != nil {
		log.Fatal("failed to load loading ENV variables:", err)
	}

	return &e
}
