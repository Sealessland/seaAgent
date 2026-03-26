package main

import (
	"log"

	"eino-vlm-agent-demo/internal/jetsonagent"
)

func main() {
	if err := jetsonagent.Run(); err != nil {
		log.Fatal(err)
	}
}
