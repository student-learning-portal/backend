package main

import (
	"log"

	"github.com/student-learning-portal/backend/internal"
)

func main() {
	log.Println("Starting portal backend...")
	// Initialized DB, router and starts the server
	internal.Run()
}
