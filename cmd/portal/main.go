package main

import (
	"github.com/student-learning-portal/backend/internal"
)

func main() {
	// Initializes structured logging, DB, router and starts the server.
	internal.Run()
}
